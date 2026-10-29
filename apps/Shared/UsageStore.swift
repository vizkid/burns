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
}

/// Reads the hosts, keeps the cache, and builds what the views draw.
@MainActor
final class UsageStore: ObservableObject {
    @Published private(set) var model: UsageModel
    @Published private(set) var statuses: [HostStatus] = []
    @Published var hosts: [String] {
        didSet { HostList(defaults: AppGroup.defaults).hosts = hosts; Task { await refresh() } }
    }

    private let cache = HostCache(defaults: AppGroup.defaults)
    private let selection = Selection(defaults: AppGroup.defaults)

    init() {
        hosts = HostList(defaults: AppGroup.defaults).hosts
        model = UsageModel(snapshots: [], selected: nil)
        rebuild()
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
        model = UsageModel(snapshots: snaps, selected: selection.account)
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
