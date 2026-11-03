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
    /// Under this much of the window the ratio is noise. It is 50 minutes of a week: a longer hold hid
    /// a real burn just after a reset (Viz, 2026-11-02).
    public static let quietFraction = 0.005
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
}
