import Foundation

/// A point on a burndown: x runs from the window's start (0) to its reset (1), left is percent left.
public struct Point: Sendable, Equatable {
    public var x: Double
    public var left: Double
    public init(x: Double, left: Double) { self.x = x; self.left = left }

    /// Equal within rounding: x comes from seconds divided by the window's length.
    public static func == (a: Point, b: Point) -> Bool { abs(a.x - b.x) < 1e-9 && abs(a.left - b.left) < 1e-9 }
}

/// One account's burndown (docs/SPEC.md, "Showing usage"): its longest live window, normally the
/// week, from its start to its reset, with the steady burn, the projection, and a time grid. The
/// five-hour window is the ring's alone (Viz, 2026-10-29).
public struct Burndown: Equatable, Sendable {
    public struct Line: Equatable, Sendable {
        public var minutes: Int
        /// Solid: quota left at each reading, which the chart joins with a monotone curve. Ends at the
        /// last observation.
        public var actual: [Point]
        /// Faint dashed: 100% at the start to 0% at the reset.
        public var steady: [Point]
        /// Heavy dashed: the last day's rate to the reset, or to empty. Empty when it is too early to say.
        public var projection: [Point]
        public var verdict: SpendPace.Verdict?
        /// Where now is on this window's axis.
        public var now: Double
    }

    public struct Slot: Equatable, Sendable {
        public var x0: Double
        public var x1: Double
        public var elapsed: Bool
    }

    public var account: AccountID
    /// The longest live window's line; nil with no live window.
    public var line: Line?
    /// Days for the week, hours for a five-hour window: the charted window's grid.
    public var grid: [Slot]
    public var headline: String
    /// Azure: Microsoft credits, so no quota window to draw.
    public var credits: Bool

    public init(account: AccountID, line: Line?, grid: [Slot], headline: String, credits: Bool = false) {
        self.account = account; self.line = line; self.grid = grid; self.headline = headline; self.credits = credits
    }

    public static func build(account: AccountUsage, readings: [Reading], now: Date) -> Burndown {
        guard let (m, w) = account.liveWindows(at: now).max(by: { $0.key < $1.key }) else {
            return Burndown(account: account.id, line: nil, grid: [], headline: "Usage unknown")
        }
        let mine = readings.filter {
            $0.provider == account.id.provider && $0.account == account.id.name && $0.minutes == m && w.holds($0)
        }
        let charted = line(minutes: m, window: w, readings: mine, now: now)
        return Burndown(account: account.id, line: charted, grid: grid(charted), headline: headline(minutes: m, window: w, now: now, projection: charted.projection))
    }

    static func length(_ minutes: Int) -> TimeInterval { TimeInterval(minutes) * 60 }

    static func line(minutes: Int, window w: Window, readings: [Reading], now: Date) -> Line {
        let length = length(minutes), start = w.resetsAt.addingTimeInterval(-length)
        func x(_ d: Date) -> Double { min(max(d.timeIntervalSince(start) / length, 0), 1) }
        let used = w.percent ?? 0
        // Readings sample usage that grew between them, so each one is a point on the curve: a flat
        // hold and a cliff would claim nothing was used until the reading. Rule 10: a reading below
        // an earlier one is stale and is skipped.
        var actual = [Point(x: 0, left: 100)]
        var high = 0.0
        // Readings are whole percents: keep the moment usage first reached each one. A later repeat
        // would add a flat shoulder, and the line would read as a staircase.
        for r in readings.sorted(by: { $0.observedAt < $1.observedAt }) where r.observedAt >= start && (r.percent > high || actual.count == 1) {
            actual.append(Point(x: x(r.observedAt), left: 100 - r.percent))
            high = r.percent
        }
        // The line ends at the current percent, the one the headline states, even when that
        // reading's time is no later than the last reading drawn. Readings record changes only, so
        // an unchanged percent observed later holds the line flat to that observation.
        if let lastPoint = actual.last, used > high || actual.count == 1 || x(w.observedAt) > lastPoint.x {
            actual.append(Point(x: max(x(w.observedAt), lastPoint.x), left: 100 - max(used, high)))
        }
        let pace = SpendPace(usedPercent: used, resetsAt: w.resetsAt, length: length, now: now)
        let projection = pace?.verdict != nil && used < 100 ? project(actual, now: x(now)) : []
        return Line(minutes: minutes, actual: actual, steady: [Point(x: 0, left: 100), Point(x: 1, left: 0)],
                    projection: projection, verdict: pace?.verdict, now: x(now))
    }

    /// How far back the current burn is measured: a seventh of the window, so a day of a week. A
    /// whole day holds a night's idle and a day's work, so neither alone sets the slope.
    static let recentFraction = 1.0 / 7

    /// The current burn carried to the reset, or to 0% left. It starts where the line ends, the last
    /// observation, which can be hours before now: starting at now would leave a gap. The burn is
    /// measured to now, not to that observation, because usage only moves when a reading arrives,
    /// so the quiet hours since it burned nothing.
    static func project(_ actual: [Point], now: Double) -> [Point] {
        guard let last = actual.last else { return [] }
        let end = max(now, last.x), start = max(end - recentFraction, 0)
        let rate = (left(actual, at: start) - last.left) / max(end - start, SpendPace.quietFraction)
        if rate > 0, last.x + last.left / rate < 1 { return [last, Point(x: last.x + last.left / rate, left: 0)] }
        return [last, Point(x: 1, left: last.left - rate * (1 - last.x))]
    }

    /// Percent left at `x`, straight between readings. The actual line starts at (0, 100).
    static func left(_ actual: [Point], at x: Double) -> Double {
        guard let i = actual.firstIndex(where: { $0.x >= x }) else { return actual.last?.left ?? 100 }
        guard i > 0, actual[i].x > actual[i - 1].x else { return actual[i].left }
        let a = actual[i - 1], b = actual[i]
        return a.left + (b.left - a.left) * (x - a.x) / (b.x - a.x)
    }

    static func grid(_ line: Line) -> [Slot] {
        let count = line.minutes >= 1440 ? line.minutes / 1440 : max(1, line.minutes / 60)
        return (0..<count).map { i in
            let x0 = Double(i) / Double(count), x1 = Double(i + 1) / Double(count)
            return Slot(x0: x0, x1: x1, elapsed: x1 <= line.now + 1e-9)
        }
    }

    /// "34% left · 15% under · 4h ahead of pace · resets 1d 7h", for the charted window.
    static func headline(minutes m: Int, window w: Window, now: Date, projection: [Point] = []) -> String {
        guard let used = w.percent else { return "Usage unknown" }
        let reset = SpendReset.countdown(to: w.resetsAt, now: now).map { "resets " + $0 }
        if used >= 100 { return (["Budget spent"] + [reset].compactMap { $0 }).joined(separator: " · ") }
        let pace = SpendPace(usedPercent: used, resetsAt: w.resetsAt, length: length(m), now: now)
        let parts = [pace.flatMap { current($0) }, outlook(projection, length: length(m), now: now), reset]
        return ([String(format: "%.0f%% left", 100 - used)] + parts.compactMap { $0 }).joined(separator: " · ")
    }

    /// The current reading against the steady line: "15% under". Nil on the line, or while it is
    /// too early to say.
    static func current(_ pace: SpendPace) -> String? {
        guard pace.verdict != nil, let points = pace.aheadPoints, points != 0 else { return nil }
        return "\(abs(points))% \(points > 0 ? "over" : "under")"
    }

    /// Where the current burn ends up (Viz, 2026-11-02): out early is the time short of the reset,
    /// "4h ahead of pace"; quota left at the reset is that percent, "12% behind pace". A reading
    /// can be under the steady line and ahead of pace, so the header shows both.
    static func outlook(_ projection: [Point], length: TimeInterval, now: Date) -> String? {
        guard let end = projection.last else { return nil }
        if end.left >= 0.5 { return "\(Int(end.left.rounded()))% behind pace" }
        let early = (1 - end.x) * length
        return early / length < 0.005 ? "at pace" : "\(span(early, now: now)) ahead of pace"
    }

    /// A duration in the countdown's format. The half minute keeps 47h 59m 59.9s from reading short.
    static func span(_ seconds: TimeInterval, now: Date) -> String {
        SpendReset.countdown(to: now.addingTimeInterval(seconds + 30), now: now) ?? "<1m"
    }
}
