import Foundation

/// Whether a quota window is being used faster than it refills (docs/SPEC.md, "Showing usage"; ported from Penfold's PenfoldKit). The bar shows how much is used; this says what that means for the time left.
///
/// With `elapsed` the fraction of the window gone, the average rate so far projects `used /
/// elapsed` at the reset, and reaches 100% when `elapsed * 100 / used` of the window has passed.
/// Nil when there is no reset to measure from: no reset instant, or one that has passed, in which
/// case the used percent belongs to a window that is gone.
public struct SpendPace: Equatable, Sendable {
    public enum Verdict: Int, Equatable, Sendable, Comparable {
        /// On or under pace: the window ends at or below 100%.
        case onPace
        /// 100% is projected before the reset, with a quarter of the window or less left by then.
        case over
        /// 100% is projected with more than a quarter of the window left, or is already reached.
        case critical
        public static func < (a: Verdict, b: Verdict) -> Bool { a.rawValue < b.rawValue }
    }

    public static let fiveHours: TimeInterval = 5 * 3600
    public static let sevenDays: TimeInterval = 7 * 86_400
    /// Under this much of the window the ratio is noise: one prompt reads as ten times over pace.
    public static let quietFraction = 0.03
    static let criticalFraction = 0.25
    static let projectionCap = 999.0

    /// Where the window is, 0 to 1: the bar's tick.
    public let elapsed: Double
    /// Nil in the window's first minutes, or with no used percent.
    public let verdict: Verdict?
    /// Use at the reset at the average rate so far, capped for display.
    public let projectedPercent: Double?
    /// When 100% is reached at that rate, if that is before the reset.
    public let runsOutAt: Date?
    public let resetsAt: Date
    /// The used percent read, nil with no reading.
    public let used: Double?

    public init?(usedPercent: Double?, resetsAt: Date?, length: TimeInterval, now: Date = Date()) {
        guard let resetsAt, resetsAt.timeIntervalSince1970 > 0, length > 0 else { return nil }
        let remaining = resetsAt.timeIntervalSince(now)
        guard remaining > 0 else { return nil }
        self.resetsAt = resetsAt
        elapsed = max(0, min(1, 1 - remaining / length))
        used = usedPercent.flatMap { $0.isFinite && $0 >= 0 ? $0 : nil }
        // A percent that is not a number, or is negative, is no reading: the tick, and no verdict.
        guard let used = usedPercent, used.isFinite, used >= 0 else { verdict = nil; projectedPercent = nil; runsOutAt = nil; return }
        if used >= 100 { verdict = .critical; projectedPercent = min(max(used, used / max(elapsed, Self.quietFraction)), Self.projectionCap); runsOutAt = nil; return }
        guard elapsed >= Self.quietFraction else { verdict = nil; projectedPercent = nil; runsOutAt = nil; return }
        let projected = used / elapsed
        projectedPercent = min(projected, Self.projectionCap)
        guard projected > 100 else { verdict = .onPace; runsOutAt = nil; return }
        // The part of the window gone when 100% is reached, and what is left of it then.
        let reached = elapsed * 100 / used
        runsOutAt = resetsAt.addingTimeInterval(-(1 - reached) * length)
        verdict = 1 - reached > Self.criticalFraction ? .critical : .over
    }

    /// Points above an even pace: used minus the window gone, rounded. Viz, 2026-10-05: "tell me how
    /// far ahead I'm running rather than '7d out'".
    public var aheadPoints: Int? {
        guard let used, used < 100 else { return nil }
        return Int((used - elapsed * 100).rounded())
    }

    /// The pace alone, only when there is something to act on: "6% ahead", or "out" once 100% is
    /// reached. For a place that already shows the window's reset beside it.
    public func words() -> String? {
        switch verdict {
        case .over?, .critical?:
            if runsOutAt == nil { return "out" }
            guard let aheadPoints, aheadPoints > 0 else { return "On the edge" }
            return "\(aheadPoints)% ahead"
        case .onPace?, nil: return nil
        }
    }

    /// The pace with the time to reset: "6% ahead · resets 2h", "out · resets 17h".
    public func short(calendar: Calendar = .current, now: Date = Date()) -> String? {
        words().map { $0 + (SpendReset.countdown(to: resetsAt, now: now).map { " · resets " + $0 } ?? "") }
    }

    /// The hover's full sentence, with the time to reset. Nil with no verdict.
    public func sentence(calendar: Calendar = .current, now: Date = Date()) -> String? {
        guard verdict != nil else { return nil }
        let resetIn = SpendReset.countdown(to: resetsAt, now: now) ?? "<1m"
        if let runsOutAt {
            return "At this pace you reach 100% at \(Self.clock(runsOutAt, calendar: calendar, now: now)), \(Self.span(resetsAt.timeIntervalSince(runsOutAt))) before the reset in \(resetIn)"
        }
        if verdict == .critical { return "You have reached 100%. The window resets at \(Self.clock(resetsAt, calendar: calendar, now: now)), in \(resetIn)" }
        return "At this pace you finish the window at \(String(format: "%.0f%%", projectedPercent ?? 0)) when it resets in \(resetIn)"
    }

    /// One line for an account: each window's pace, for the windows that have any. Each bar shows
    /// its own reset, so the line does not repeat it.
    public static func line(fiveHour: SpendPace?, sevenDay: SpendPace?, calendar: Calendar = .current, now: Date = Date()) -> String? {
        let parts = [("5h", fiveHour), ("7d", sevenDay)].compactMap { name, pace in
            pace?.words().map { "\(name) \($0)" }
        }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    /// The more urgent of two windows' verdicts: the line's colour.
    public static func worst(_ a: SpendPace?, _ b: SpendPace?) -> Verdict? {
        [a?.verdict, b?.verdict].compactMap { $0 }.max()
    }

    /// "14:20" today, "Thu 14:20" on another day.
    static func clock(_ date: Date, calendar: Calendar, now: Date) -> String {
        let f = DateFormatter()
        f.calendar = calendar; f.timeZone = calendar.timeZone; f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = calendar.isDate(date, inSameDayAs: now) ? "HH:mm" : "EEE HH:mm"
        return f.string(from: date)
    }

    /// "27m", "3h", "1h 50m", "3d 16h": the countdown's own shape.
    static func span(_ seconds: TimeInterval) -> String {
        SpendReset.countdown(to: Date(timeIntervalSince1970: max(seconds, 0)), now: Date(timeIntervalSince1970: 0)) ?? "<1m"
    }
}

extension SpendPace {
    /// Where the tick goes on a bar `width` wide, or nil when the bar is too narrow to hold one.
    public func tickOffset(width: Double, tick: Double = 1.5) -> Double? {
        guard width >= tick else { return nil }
        return min(max(width * elapsed - tick / 2, 0), width - tick)
    }
}
