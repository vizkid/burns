import Foundation

/// One account on the Spend tab, as a tile: its five-hour and seven-day windows as rings, the pace
/// in words when it matters, and when each window resets (docs/SPEC.md, "Showing usage"; ported from Penfold). Pure, so the
/// iPhone and the Mac draw the same facts.
public struct SpendTileModel: Equatable, Sendable, Identifiable {
    public struct Window: Equatable, Sendable {
        /// "5h" or "7d": the short name in the verdict words.
        public let name: String
        /// "five-hour" or "seven-day": the spoken name.
        public let spoken: String
        public let percent: Double?
        /// The ring's fill, 0 to 1.
        public var fill: Double { min(max(percent ?? 0, 0), 100) / 100 }
        public var percentText: String { percent.map { String(format: "%.0f%%", $0) } ?? "Unknown" }
        /// Where the clock is in the window, 0 to 1: the end of the ring's time arc. Nil with no future reset.
        public let elapsed: Double?
        public let verdict: SpendPace.Verdict?
        /// "6% ahead" or "out"; nil when on pace or too early to say. The reset is `resetsIn`.
        public let words: String?
        /// "2h 14m": the time until the window resets. Nil with no future reset.
        public let resetsIn: String?
    }

    public let account: String
    public let fiveHour: Window
    public let sevenDay: Window
    public let onCredits: Bool
    public var id: String { account }

    /// The words under the account, for the windows that have any: "5h 6% ahead · 7d out". The
    /// tile shows each window's reset on its own line.
    public var verdict: String? {
        let parts = [fiveHour, sevenDay].compactMap { w in w.words.map { "\(w.name) \($0)" } }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    /// The verdict with each window's reset, for a place that shows no reset of its own:
    /// "5h 6% ahead · resets 2h".
    public var verdictWithReset: String? {
        let parts = [fiveHour, sevenDay].compactMap { w in
            w.words.map { "\(w.name) \($0)" + (w.resetsIn.map { " · resets \($0)" } ?? "") }
        }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    /// The more urgent window's verdict: the tile's colour.
    public var worst: SpendPace.Verdict? { [fiveHour.verdict, sevenDay.verdict].compactMap { $0 }.max() }

    /// What VoiceOver reads: "alpha: five-hour 62%, over pace, resets in 1h 30m; seven-day 30%, resets in 3d".
    public var accessibilityLabel: String {
        func spoken(_ w: Window) -> String {
            "\(w.spoken) \(w.percent == nil ? "unknown" : w.percentText)" + (w.words.map { ", \($0)" } ?? "")
                + (w.resetsIn.map { ", resets in \($0)" } ?? "")
        }
        return "\(account): \(spoken(fiveHour)); \(spoken(sevenDay))" + (onCredits ? "; on credits" : "")
    }

    public init(account: String, fiveHourPercent: Double?, fiveHourResetsAt: Date?, sevenDayPercent: Double?, sevenDayResetsAt: Date?,
                onCredits: Bool, now: Date = Date(), calendar: Calendar = .current) {
        self.account = account
        self.onCredits = onCredits
        fiveHour = Self.window("5h", "five-hour", fiveHourPercent, fiveHourResetsAt, SpendPace.fiveHours, now, calendar)
        sevenDay = Self.window("7d", "seven-day", sevenDayPercent, sevenDayResetsAt, SpendPace.sevenDays, now, calendar)
    }

    private static func window(_ name: String, _ spoken: String, _ percent: Double?, _ resetsAt: Date?, _ length: TimeInterval,
                               _ now: Date, _ calendar: Calendar) -> Window {
        let reset = resetsAt.flatMap { $0.timeIntervalSince1970 > 0 ? $0 : nil }
        let pace = SpendPace(usedPercent: percent, resetsAt: reset, length: length, now: now)
        return Window(name: name, spoken: spoken, percent: percent, elapsed: pace?.elapsed, verdict: pace?.verdict,
                      words: pace?.words(), resetsIn: SpendReset.countdown(to: reset, now: now))
    }
}
