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
        // True with no quota accounts: none has any left, so the rule falls to Azure.
        var allExhausted = true
        for a in quota {
            // Rule 3: an account with no live window does not compete. A window whose reset passed
            // with no newer reading does not block it, or an idle account could never win.
            guard let left = a.quotaLeft(at: now) else { allExhausted = false; continue }
            guard left > 0 else { continue }
            allExhausted = false
            if best == nil || left > best!.1 { best = (a.id, left) }
        }
        if let best { return .quota(best.0, left: best.1) }
        if allExhausted, let azure = accounts.first(where: { !$0.isQuota }) { return .azure(azure.id) }
        return quota.first.map { .unknown($0.id) } ?? .none
    }

    /// Why the panel picked the account (docs/SPEC.md, "Default rule").
    public func reason(among accounts: [AccountUsage]) -> String {
        switch self {
        case .quota(let id, let left): return "\(id.label): \(Int(left.rounded()))% left"
        case .azure: return accounts.contains(where: \.isQuota) ? "All quotas exhausted: Azure" : "No quota accounts: Azure"
        case .unknown(let id): return "Usage unknown: \(id.label)"
        case .none: return ""
        }
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
