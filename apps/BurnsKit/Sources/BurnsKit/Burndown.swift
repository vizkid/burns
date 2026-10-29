import Foundation

/// A point on a burndown: x runs from the window's start (0) to its reset (1), left is percent left.
public struct Point: Sendable, Hashable {
    public var x: Double
    public var left: Double
    public init(x: Double, left: Double) { self.x = x; self.left = left }

    /// Equal within rounding: x comes from seconds divided by the window's length.
    public static func == (a: Point, b: Point) -> Bool { abs(a.x - b.x) < 1e-9 && abs(a.left - b.left) < 1e-9 }
    public func hash(into h: inout Hasher) { h.combine((x * 1e6).rounded()); h.combine((left * 1e6).rounded()) }
}

/// One account's combined burndown (docs/SPEC.md, "Showing usage"): each live window's line on
/// one axis from its start to its reset, the steady burn, the projection, and a time grid.
public struct Burndown: Equatable, Sendable {
    public struct Line: Equatable, Sendable {
        public var minutes: Int
        /// Solid: quota left from the readings. Ends at the last observation, so a gap draws nothing.
        public var actual: [Point]
        /// Dashed: 100% at the start to 0% at the reset.
        public var steady: [Point]
        /// Dotted: the current rate to the reset, or to empty. Empty when it is too early to say.
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
    /// Longest window first: it is drawn heavier.
    public var lines: [Line]
    /// Windows that are known to exist but have no live reading.
    public var unknown: [Int]
    /// Days for the week, hours for a five-hour window: the longest live window's grid.
    public var grid: [Slot]
    public var headline: String

    public static func build(account: AccountUsage, readings: [Reading], now: Date) -> Burndown {
        let live = account.liveWindows(at: now)
        var lines: [Line] = []
        for (m, w) in live.sorted(by: { $0.key > $1.key }) {
            let mine = readings.filter {
                $0.provider == account.id.provider && $0.account == account.id.name && $0.minutes == m
                    && abs($0.resetsAt.timeIntervalSince(w.resetsAt)) <= Merge.resetJitter
            }
            lines.append(line(minutes: m, window: w, readings: mine, now: now))
        }
        let unknown = account.windows.keys.filter { live[$0] == nil }.sorted(by: >)
        return Burndown(account: account.id, lines: lines, unknown: unknown, grid: grid(lines.first), headline: headline(live, now: now))
    }

    static func length(_ minutes: Int) -> TimeInterval { TimeInterval(minutes) * 60 }

    static func line(minutes: Int, window w: Window, readings: [Reading], now: Date) -> Line {
        let length = length(minutes), start = w.resetsAt.addingTimeInterval(-length)
        func x(_ d: Date) -> Double { min(max(d.timeIntervalSince(start) / length, 0), 1) }
        let used = w.percent ?? 0
        var actual = [Point(x: 0, left: 100)]
        var last: Double?
        for r in readings.sorted(by: { $0.observedAt < $1.observedAt }) where r.observedAt >= start {
            // A reading is a change: the previous value held until it.
            if let last { actual.append(Point(x: x(r.observedAt), left: 100 - last)) }
            actual.append(Point(x: x(r.observedAt), left: 100 - r.percent))
            last = r.percent
        }
        let observed = Point(x: x(w.observedAt), left: 100 - used)
        if let lastPoint = actual.last, observed.x > lastPoint.x {
            if last != nil, last != used { actual.append(Point(x: observed.x, left: lastPoint.left)) }
            actual.append(observed)
        }
        let pace = SpendPace(usedPercent: used, resetsAt: w.resetsAt, length: length, now: now)
        var projection: [Point] = []
        if let pace, pace.verdict != nil {
            let from = Point(x: x(now), left: 100 - used)
            if let out = pace.runsOutAt {
                projection = [from, Point(x: x(out), left: 0)]
            } else if used < 100 {
                projection = [from, Point(x: 1, left: 100 - used / max(pace.elapsed, SpendPace.quietFraction))]
            }
        }
        return Line(minutes: minutes, actual: actual, steady: [Point(x: 0, left: 100), Point(x: 1, left: 0)],
                    projection: projection, verdict: pace?.verdict, now: x(now))
    }

    static func grid(_ line: Line?) -> [Slot] {
        guard let line else { return [] }
        let count = line.minutes >= 1440 ? line.minutes / 1440 : max(1, line.minutes / 60)
        return (0..<count).map { i in
            let x0 = Double(i) / Double(count), x1 = Double(i + 1) / Double(count)
            return Slot(x0: x0, x1: x1, elapsed: x1 <= line.now + 1e-9)
        }
    }

    /// "34% left · 6% ahead · resets 2h", for the window with the least left.
    static func headline(_ live: [Int: Window], now: Date) -> String {
        guard let (m, w) = live.max(by: { ($0.value.percent ?? 0, $1.key) < ($1.value.percent ?? 0, $0.key) }), let used = w.percent else {
            return "Usage unknown"
        }
        let reset = SpendReset.countdown(to: w.resetsAt, now: now).map { "resets " + $0 }
        if used >= 100 { return (["Budget spent"] + [reset].compactMap { $0 }).joined(separator: " · ") }
        let words = SpendPace(usedPercent: used, resetsAt: w.resetsAt, length: length(m), now: now)?.words()
        return ([String(format: "%.0f%% left", 100 - used)] + [words, reset].compactMap { $0 }).joined(separator: " · ")
    }
}
