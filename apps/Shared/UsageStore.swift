import BurnsKit
import Foundation
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
final class UsageStore: ObservableObject {
    @Published private(set) var model: UsageModel
    @Published private(set) var statuses: [HostStatus] = []
    /// What the last pass did about each host's accounts file.
    @Published private(set) var accountSync: [AccountSync.HostSync] = []
    @Published var hosts: [String] {
        didSet { HostList(defaults: AppGroup.defaults).hosts = hosts; Task { await refresh() } }
    }

    /// Names to show for hosts. Commands keep the Rex label.
    @Published var hostNames: [String: String] = AppGroup.defaults.dictionary(forKey: AppGroup.hostNamesKey) as? [String: String] ?? [:] {
        didSet { AppGroup.defaults.set(hostNames, forKey: AppGroup.hostNamesKey) }
    }

    private let cache = HostCache(defaults: AppGroup.defaults)
    private let selection = Selection(defaults: AppGroup.defaults)
    private let accounts = CentralStore(values: NSUbiquitousKeyValueStore.default)
    private let launched = Date()
    /// How long a new device waits for iCloud before it imports from the hosts.
    private static let iCloudWait: TimeInterval = 60
    /// The central list, kept in step with iCloud.
    @Published private(set) var central: [CentralAccount] = AppGroup.accounts

    init() {
        hosts = HostList(defaults: AppGroup.defaults).hosts
        model = UsageModel(snapshots: [], selected: nil)
        rebuild()
        NSUbiquitousKeyValueStore.default.synchronize()
        NotificationCenter.default.addObserver(forName: NSUbiquitousKeyValueStore.didChangeExternallyNotification,
                                               object: NSUbiquitousKeyValueStore.default, queue: .main) { [weak self] _ in
            Task { @MainActor in self?.syncAccounts() ; await self?.pushAccounts() }
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

    /// Overwrites a held host: marks its current file as the last written one, so the next pass writes it.
    func overwrite(_ host: String) async {
        guard let file = try? await HostAccounts().get(host) else { return }
        accounts.setWritten(file.etag, host: host)
        await pushAccounts()
    }

    private func syncAccounts() {
        central = accounts.load() ?? central
        AppGroup.defaults.set(AccountsFile(accounts: central).encoded(), forKey: AppGroup.accountsKey)
        rebuild()
    }

    private func pushAccounts() async {
        accountSync = await AccountSync.run(hosts: hosts, store: accounts, client: HostAccounts(),
                                            mayImport: Date().timeIntervalSince(launched) > Self.iCloudWait)
        syncAccounts()
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
        await pushAccounts()
    }

    private var lastContent = ""

    func select(_ id: AccountID) {
        selection.account = id
        rebuild()
        WidgetCenter.shared.reloadAllTimelines()
    }

    /// Rebuilds from the cache: also how the time arcs and expiry move between fetches.
    func rebuild() {
        let snaps = hosts.compactMap { cache.load()[$0] }
        model = UsageModel(snapshots: snaps, selected: selection.account, accounts: central)
        if statuses.isEmpty { statuses = hosts.map { HostStatus(address: $0, snapshot: cache.load()[$0], error: nil) } }
    }
}

/// "now", "12m ago", "3h ago": how old a host's answer is.
func ageText(_ date: Date?, now: Date = Date()) -> String {
    guard let date else { return "never" }
    let s = now.timeIntervalSince(date)
    if s < 60 { return "now" }
    return (SpendReset.countdown(to: now.addingTimeInterval(s), now: now) ?? "") + " ago"
}
