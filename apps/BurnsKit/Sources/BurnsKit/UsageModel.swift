import Foundation

/// The selected ring, shared by the app and its widgets through the app group.
public final class Selection: @unchecked Sendable {
    public static let key = "burns.selectedAccount"
    let defaults: UserDefaults
    public init(defaults: UserDefaults) { self.defaults = defaults }

    public var account: AccountID? {
        get { defaults.data(forKey: Self.key).flatMap { try? JSONDecoder().decode(AccountID.self, from: $0) } }
        set { defaults.set(newValue.flatMap { try? JSONEncoder().encode($0) }, forKey: Self.key) }
    }
}

/// The host list in settings, one tailnet name per entry.
public final class HostList: @unchecked Sendable {
    public static let key = "burns.hosts"
    let defaults: UserDefaults
    public init(defaults: UserDefaults) { self.defaults = defaults }

    public var hosts: [String] {
        get { defaults.stringArray(forKey: Self.key) ?? [] }
        set { defaults.set(Self.clean(newValue), forKey: Self.key) }
    }

    /// Trimmed, lowercased, without a scheme or port, each once.
    public static func clean(_ hosts: [String]) -> [String] {
        var out: [String] = []
        for h in hosts {
            var t = h.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            if let r = t.range(of: "://") { t = String(t[r.upperBound...]) }
            t = String(t.prefix { $0 != "/" && $0 != ":" })
            if !t.isEmpty, !out.contains(t) { out.append(t) }
        }
        return out
    }
}

/// Everything a usage view draws, from the hosts' snapshots: the rings, the selected account,
/// and its burndown. Pure, so the window and the widgets show the same facts.
public struct UsageModel: Equatable, Sendable {
    public struct Ring: Equatable, Sendable, Identifiable {
        public var id: AccountID
        public var tile: SpendTileModel
        /// Azure: Microsoft credits, no quota windows. It draws its mark without rings.
        public var credits: Bool
        /// The account name's colour, on its glyph.
        public var swatch: Swatch
    }

    public var rings: [Ring]
    public var selected: AccountID?
    public var burndown: Burndown?
    public var pick: Pick
    /// The oldest snapshot time among the hosts, so a view can say how old its data may be.
    public var oldestFetch: Date?

    public init(snapshots: [HostSnapshot], selected stored: AccountID?, now: Date = Date(), calendar: Calendar = .current) {
        let accounts = Merge.accounts(snapshots.map(\.usage), now: now)
        let ordered = RingOrder.ordered(accounts)
        let swatches = Swatch.assign(ordered.map(\.id.name))
        rings = ordered.map { a in
            let live = a.liveWindows(at: now)
            let five = live.filter { $0.key <= 600 }.max { $0.key < $1.key }?.value
            let seven = live.filter { $0.key > 600 }.max { $0.key < $1.key }?.value
            let tile = SpendTileModel(account: a.id.label, fiveHourPercent: five?.percent, fiveHourResetsAt: five?.resetsAt,
                                      sevenDayPercent: seven?.percent, sevenDayResetsAt: seven?.resetsAt,
                                      onCredits: false, now: now, calendar: calendar)
            return Ring(id: a.id, tile: tile, credits: !a.isQuota, swatch: swatches[a.id.name] ?? .grey)
        }
        pick = Pick.choose(accounts, now: now)
        let ids = rings.map(\.id)
        if let stored, ids.contains(stored) {
            selected = stored
        } else if let p = pick.account, ids.contains(p) {
            selected = p
        } else {
            selected = ids.first
        }
        let readings = Merge.readings(snapshots.map(\.readings), accounts: accounts, now: now)
        burndown = selected.flatMap { id in accounts.first { $0.id == id } }.map { a in
            a.isQuota ? Burndown.build(account: a, readings: readings, now: now)
                : Burndown(account: a.id, line: nil, unknown: [], grid: [], headline: "Microsoft credits")
        }
        oldestFetch = snapshots.map(\.fetchedAt).min()
    }

    /// The pace verdict of Enter's pick: the menu bar flame's colour.
    public var pickVerdict: SpendPace.Verdict? {
        guard let id = pick.account, let ring = rings.first(where: { $0.id == id }) else { return nil }
        return ring.tile.worst
    }
}
