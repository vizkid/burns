import Foundation

/// A provider and account name, such as Claude `alpha`.
public struct AccountID: Hashable, Codable, Sendable {
    public var provider: String
    public var name: String
    public init(provider: String, name: String) { self.provider = provider; self.name = name }

    /// "Claude alpha".
    public var label: String { "\(provider.prefix(1).uppercased())\(provider.dropFirst()) \(name)" }
}

/// One quota window. A nil percent was not reported.
public struct Window: Equatable, Sendable {
    public var percent: Double?
    public var resetsAt: Date
    public var observedAt: Date
    public init(percent: Double?, resetsAt: Date, observedAt: Date) { self.percent = percent; self.resetsAt = resetsAt; self.observedAt = observedAt }

    /// Rule 2: a window whose reset passed is unknown, never zero.
    public func isLive(at now: Date) -> Bool { percent != nil && resetsAt > now }
}

/// One account's merged windows, keyed by length in minutes.
public struct AccountUsage: Equatable, Sendable {
    public var id: AccountID
    public var kind: String
    public var windows: [Int: Window]
    public init(id: AccountID, kind: String, windows: [Int: Window]) { self.id = id; self.kind = kind; self.windows = windows }

    /// Azure runs on Microsoft credits and has no quota windows.
    public var isQuota: Bool { kind != "azure" }

    public func liveWindows(at now: Date) -> [Int: Window] { windows.filter { $0.value.isLive(at: now) } }
}

/// docs/SPEC.md, "Merging hosts". Quota belongs to the account, so the newest live reading per
/// window wins (rule 6); readings are never added. A port of Go `quota.Merge`.
public enum Merge {
    /// Two readings of one window whose resets differ by more than this are different windows.
    public static let resetJitter: TimeInterval = 60

    public static func windows(_ sets: [[Int: Window]], now: Date) -> [Int: Window] {
        var out: [Int: Window] = [:]
        for set in sets {
            for (m, w) in set where w.isLive(at: now) {
                if let prev = out[m] {
                    if abs(prev.resetsAt.timeIntervalSince(w.resetsAt)) <= resetJitter, let p = prev.percent, let q = w.percent {
                        // Rule 10: within one window usage never goes down, so the highest
                        // wins; another host's newer reading can be stale.
                        if q < p || (q == p && w.observedAt <= prev.observedAt) { continue }
                    } else if w.observedAt <= prev.observedAt { continue }
                }
                out[m] = w
            }
        }
        return out
    }

    /// Every account any host reports, in the order hosts list them, first host first.
    public static func accounts(_ answers: [UsageAnswer], now: Date) -> [AccountUsage] {
        var order: [AccountID] = [], kinds: [AccountID: String] = [:], sets: [AccountID: [[Int: Window]]] = [:]
        for answer in answers {
            for a in answer.accounts {
                let id = AccountID(provider: a.provider, name: a.account)
                if kinds[id] == nil { order.append(id); kinds[id] = a.kind }
                for limit in a.limits {
                    var set: [Int: Window] = [:]
                    for (key, w) in limit.windows {
                        guard let m = Int(key) else { continue }
                        set[m] = Window(percent: w.percent, resetsAt: w.resetsAt, observedAt: w.observedAt)
                    }
                    sets[id, default: []].append(set)
                }
            }
        }
        return order.map { AccountUsage(id: $0, kind: kinds[$0] ?? "", windows: windows(sets[$0] ?? [], now: now)) }
    }

    /// Every host's readings, each once, oldest first, keeping only readings of each account's
    /// current merged windows: a window that restarted early drops the old one's readings.
    public static func readings(_ sets: [[Reading]], accounts: [AccountUsage], now: Date) -> [Reading] {
        var current: [AccountID: [Int: Window]] = [:]
        for a in accounts { current[a.id] = a.liveWindows(at: now) }
        var seen = Set<Reading>()
        var out: [Reading] = []
        for r in sets.joined() {
            guard let w = current[AccountID(provider: r.provider, name: r.account)]?[r.minutes],
                  abs(w.resetsAt.timeIntervalSince(r.resetsAt)) <= resetJitter,
                  seen.insert(r).inserted else { continue }
            out.append(r)
        }
        return out.sorted { $0.observedAt < $1.observedAt }
    }
}
