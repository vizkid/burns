import Foundation

/// The account Enter would launch (docs/SPEC.md, "Default rule"). The widgets select it until a
/// ring is tapped.
public enum Pick: Equatable, Sendable {
    /// The quota account with the most left.
    case quota(AccountID, left: Double)
    /// Every quota account is known to be exhausted: Azure, on Microsoft credits.
    case azure(AccountID)
    /// Nothing has quota left and some accounts are unknown: the first quota account.
    case unknown(AccountID)
    case none

    public var account: AccountID? {
        switch self {
        case .quota(let id, _), .azure(let id), .unknown(let id): return id
        case .none: return nil
        }
    }

    public static func choose(_ accounts: [AccountUsage], now: Date) -> Pick {
        let quota = accounts.filter(\.isQuota)
        var best: (AccountID, Double)?
        var allExhausted = !quota.isEmpty
        for a in quota {
            // Rule 3: an account with no windows, or an unknown one, does not compete.
            guard !a.windows.isEmpty, a.windows.values.allSatisfy({ $0.isLive(at: now) }) else { allExhausted = false; continue }
            let used = a.windows.values.compactMap(\.percent).max() ?? 0
            if used < 100 { allExhausted = false }
            let left = max(0, 100 - used)
            if left > 0, best == nil || left > best!.1 { best = (a.id, left) }
        }
        if let best { return .quota(best.0, left: best.1) }
        if allExhausted, let azure = accounts.first(where: { !$0.isQuota }) { return .azure(azure.id) }
        return quota.first.map { .unknown($0.id) } ?? .none
    }
}

/// The ring row's order: each name's Claude then its Codex, names in the order settings list
/// them, then the Azure accounts (docs/SPEC.md, "Showing usage").
public enum RingOrder {
    public static func ordered(_ accounts: [AccountUsage]) -> [AccountUsage] {
        let quota = accounts.filter(\.isQuota)
        var names: [String] = []
        for a in quota where !names.contains(a.id.name) { names.append(a.id.name) }
        let rank = ["claude": 0, "codex": 1]
        return names.flatMap { name in
            quota.filter { $0.id.name == name }.sorted { (rank[$0.id.provider] ?? 2) < (rank[$1.id.provider] ?? 2) }
        } + accounts.filter { !$0.isQuota }
    }
}
