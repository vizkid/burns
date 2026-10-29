import Foundation

/// The time until a quota window resets, as the Spend panel shows it beside each bar
/// (COMMANDER_SPEC report section 6). A window without a future reset shows nothing: "0m" would
/// claim a reset nobody reported.
public enum SpendReset {
    /// Short time until `resetsAt`: "<1m", "14m", "2h 14m", "3h", "6d 19h", "6d". Nil when there is
    /// no reset instant or it has passed.
    public static func countdown(to resetsAt: Date?, now: Date = Date()) -> String? {
        guard let resetsAt else { return nil }
        let seconds = resetsAt.timeIntervalSince(now)
        guard seconds > 0 else { return nil }
        let minutes = Int(seconds / 60)
        if minutes < 1 { return "<1m" }
        let days = minutes / 1440, hours = (minutes % 1440) / 60, mins = minutes % 60
        if days > 0 { return hours > 0 ? "\(days)d \(hours)h" : "\(days)d" }
        if hours > 0 { return mins > 0 ? "\(hours)h \(mins)m" : "\(hours)h" }
        return "\(mins)m"
    }

    /// The hover text for one bar: the window, its use and the exact local reset time.
    public static func hover(window: String, percent: Double?, resetsAt: Date?) -> String {
        var text = "\(window): " + (percent.map { String(format: "%.0f%%", $0) } ?? "unknown")
        if let resetsAt {
            text += ", resets " + resetsAt.formatted(date: .abbreviated, time: .shortened)
        }
        return text
    }
}
