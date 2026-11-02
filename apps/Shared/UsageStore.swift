import BurnsKit
import Foundation
import Observation
import WidgetKit

/// The app group both an app and its widget read: the host list, each host's last answer, and
/// the selected ring. Its identifier comes from Info.plist, set from the build's signing domain.
enum AppGroup {
    static var id: String { Bundle.main.object(forInfoDictionaryKey: "BurnsAppGroup") as? String ?? "" }
    static var defaults: UserDefaults { UserDefaults(suiteName: id) ?? .standard }
    /// When any process last fetched every host, so a widget tap reuses a fresh cache.
    static let lastFetchKey = "burns.lastFetch"
    /// The central list, copied here because widgets cannot read iCloud's store.
    static let accountsKey = "burns.centralAccounts"
    /// Each host's optional display name, keyed by the name its smithersd reports.
    static let hostNamesKey = "burns.hostNames"
    static var accounts: [CentralAccount] {
        defaults.data(forKey: accountsKey).flatMap { try? JSONDecoder().decode(AccountsFile.self, from: $0) }?.accounts ?? []
    }
}

/// Reads the hosts, keeps the cache, and builds what the views draw.
@MainActor
@Observable
final class UsageStore {
    private(set) var model: UsageModel
    private(set) var statuses: [HostStatus] = []
    /// What the last pass did about each host's accounts file.
    private(set) var accountSync: [AccountSync.HostSync] = []
    var hosts: [String] {
        didSet { HostList(defaults: AppGroup.defaults).hosts = hosts; Task { await refresh() } }
    }

    /// Each host's icon, keyed by the name its smithersd reports. The Mac reads them from Rex and
    /// shares them through iCloud, because a phone cannot ask Rex.
    private(set) var hostIcons: [String: String] = NSUbiquitousKeyValueStore.default.dictionary(forKey: UsageStore.hostIconsKey) as? [String: String] ?? [:]
    private static let hostIconsKey = "burns.hostIcons"

    /// Adds to what iCloud holds: each device knows only some hosts, and a host with no snapshot
    /// yet must keep its icon.
    func addHostIcons(_ icons: [String: String]) {
        let merged = hostIcons.merging(icons) { _, new in new }
        guard merged != hostIcons else { return }
        hostIcons = merged
        NSUbiquitousKeyValueStore.default.set(merged, forKey: Self.hostIconsKey)
    }

    /// Names to show for hosts. Commands keep the Rex label.
    var hostNames: [String: String] = AppGroup.defaults.dictionary(forKey: AppGroup.hostNamesKey) as? [String: String] ?? [:] {
        didSet { AppGroup.defaults.set(hostNames, forKey: AppGroup.hostNamesKey) }
    }

    @ObservationIgnored private let cache = HostCache(defaults: AppGroup.defaults)
    @ObservationIgnored private let selection = Selection(defaults: AppGroup.defaults)
    @ObservationIgnored private let accounts = CentralStore(values: NSUbiquitousKeyValueStore.default)
    /// When iCloud last delivered, or the app started.
    @ObservationIgnored private var lastDelivery = Date()
    /// How long iCloud must stay quiet before a device acts on what it has: a new device before it
    /// imports from the hosts, and any device before it accepts a list that does not match its digest.
    private static let iCloudWait: TimeInterval = 60
    /// The central list, kept in step with iCloud.
    private(set) var central: [CentralAccount] = AppGroup.accounts

    init() {
        hosts = HostList(defaults: AppGroup.defaults).hosts
        model = UsageModel(snapshots: [], selected: nil)
        rebuild()
        NSUbiquitousKeyValueStore.default.synchronize()
        NotificationCenter.default.addObserver(forName: NSUbiquitousKeyValueStore.didChangeExternallyNotification,
                                               object: NSUbiquitousKeyValueStore.default, queue: .main) { [weak self] _ in
            Task { @MainActor in
                self?.lastDelivery = Date()
                self?.syncAccounts()
                await self?.pushAccounts()
            }
        }
    }

    /// Hosts where each account's home exists.
    var setUp: [AccountID: [String]] {
        var out: [AccountID: [String]] = [:]
        for s in statuses {
            guard let usage = s.snapshot?.usage else { continue }
            for a in usage.accounts where a.setUp != false {
                out[AccountID(provider: a.provider, name: a.account), default: []].append(usage.host)
            }
        }
        return out
    }

    /// Saves an edit and copies it to the hosts.
    func edit(_ list: [CentralAccount]) {
        accounts.save(list)
        syncAccounts()
        Task { await pushAccounts() }
    }

    /// Takes a held host's accounts into the central list.
    func useHost(_ host: String, _ onHost: [CentralAccount]) {
        edit(AccountSync.importing([onHost, central]).accounts)
    }

    /// Overwrites a held host, after the operator chose that in the review.
    func overwrite(_ host: String) async {
        _ = await AccountSync.overwrite(host: host, store: accounts, client: HostAccounts())
        await pushAccounts()
    }

    private func syncAccounts() {
        central = accounts.load() ?? central
        hostIcons = NSUbiquitousKeyValueStore.default.dictionary(forKey: Self.hostIconsKey) as? [String: String] ?? hostIcons
        AppGroup.defaults.set(AccountsFile(accounts: central).encoded(), forKey: AppGroup.accountsKey)
        rebuild()
    }

    /// One pass at a time. An edit made during a pass gets a pass of its own, not the next refresh.
    private func pushAccounts() async {
        guard !pushing else { pushAgain = true; return }
        pushing = true
        defer { pushing = false }
        repeat {
            pushAgain = false
            accountSync = await AccountSync.run(hosts: hosts, store: accounts, client: HostAccounts(),
                                                settled: Date().timeIntervalSince(lastDelivery) > Self.iCloudWait)
            syncAccounts()
        } while pushAgain
    }

    func refresh() async {
        statuses = await cache.refresh(hosts: hosts) { try await HostFetcher().fetch($0) }
        AppGroup.defaults.set(Date(), forKey: AppGroup.lastFetchKey)
        rebuild()
        // Widgets reload only when the hosts' data changed: reloads count against their budget.
        let content = statuses.map { [$0.snapshot?.usage.accounts as Any, $0.snapshot?.readings as Any] }.description
        if content != lastContent {
            lastContent = content
            WidgetCenter.shared.reloadAllTimelines()
        }
        // Not awaited: a pull to refresh ends when the usage is in, not when the hosts' files are.
        Task { await pushAccounts() }
    }

    @ObservationIgnored private var lastContent = ""
    @ObservationIgnored private var pushing = false
    @ObservationIgnored private var pushAgain = false

    func select(_ id: AccountID) {
        selection.account = id
        rebuild()
        WidgetCenter.shared.reloadAllTimelines()
    }

    /// Rebuilds from the cache: also how the time arcs and expiry move between fetches.
    func rebuild() {
        let cached = cache.load()
        let snaps = hosts.compactMap { cached[$0] }
        model = UsageModel(snapshots: snaps, selected: selection.account, accounts: central)
        if statuses.isEmpty { statuses = hosts.map { HostStatus(address: $0, snapshot: cached[$0], error: nil) } }
    }
}

/// "now", "12m ago", "3h ago": how old a host's answer is.
func ageText(_ date: Date?, now: Date = Date()) -> String {
    guard let date else { return "never" }
    let s = now.timeIntervalSince(date)
    if s < 60 { return "now" }
    return (SpendReset.countdown(to: now.addingTimeInterval(s), now: now) ?? "") + " ago"
}
