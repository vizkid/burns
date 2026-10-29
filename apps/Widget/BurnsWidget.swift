import AppIntents
import BurnsKit
import SwiftUI
import WidgetKit

/// The usage widget for iPhone (StandBy small, Home Screen medium) and Mac (docs/SPEC.md, "Showing
/// usage"): a row of rings, one per account, over the selected account's combined burndown.
/// Tapping a ring selects it through `SelectAccountIntent`, without opening the app.
@main
struct BurnsWidgets: WidgetBundle {
    var body: some Widget { UsageWidget() }
}

struct UsageWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: "BurnsUsage", provider: UsageProvider()) { entry in
            UsageWidgetView(entry: entry).containerBackground(.fill.tertiary, for: .widget)
        }
        .configurationDisplayName("Burns")
        .description("Quota rings for every account, and the selected account's burndown.")
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}

struct UsageEntry: TimelineEntry {
    let date: Date
    let model: UsageModel
}

struct UsageProvider: TimelineProvider {
    /// A widget reads the hosts at most this often; between reads it rebuilds from the cache.
    static let fetchEvery: TimeInterval = 15 * 60
    static let entryStep: TimeInterval = 15 * 60
    static let entryCount = 8

    func placeholder(in context: Context) -> UsageEntry { UsageEntry(date: Date(), model: Self.model(at: Date())) }

    func getSnapshot(in context: Context, completion: @escaping (UsageEntry) -> Void) {
        completion(UsageEntry(date: Date(), model: Self.model(at: Date())))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<UsageEntry>) -> Void) {
        Task {
            let defaults = AppGroup.defaults
            let last = defaults.object(forKey: AppGroup.lastFetchKey) as? Date ?? .distantPast
            // A ring tap reloads the timeline: reuse a fresh cache so the tap answers at once.
            if Date().timeIntervalSince(last) >= Self.fetchEvery - 60 {
                let hosts = HostList(defaults: defaults).hosts
                _ = await HostCache(defaults: defaults).refresh(hosts: hosts) { try await HostFetcher().fetch($0) }
                defaults.set(Date(), forKey: AppGroup.lastFetchKey)
            }
            let now = Date()
            let entries = (0..<Self.entryCount).map { i -> UsageEntry in
                let at = now.addingTimeInterval(Double(i) * Self.entryStep)
                return UsageEntry(date: at, model: Self.model(at: at))
            }
            completion(Timeline(entries: entries, policy: .after(now.addingTimeInterval(Self.fetchEvery))))
        }
    }

    static func model(at date: Date) -> UsageModel {
        let defaults = AppGroup.defaults
        let cache = HostCache(defaults: defaults).load()
        let snaps = HostList(defaults: defaults).hosts.compactMap { cache[$0] }
        return UsageModel(snapshots: snaps, selected: Selection(defaults: defaults).account, now: date)
    }
}

/// Selects a ring for every widget on this device and the app (docs/SPEC.md, "Apps and widgets").
struct SelectAccountIntent: AppIntent {
    static var title: LocalizedStringResource = "Show account"
    static var description = IntentDescription("Show this account's burndown in the Burns widget.")
    static var openAppWhenRun = false
    static var isDiscoverable = false

    @Parameter(title: "Provider") var provider: String
    @Parameter(title: "Account") var name: String

    init() {}
    init(_ id: AccountID) { provider = id.provider; name = id.name }

    func perform() async throws -> some IntentResult {
        Selection(defaults: AppGroup.defaults).account = AccountID(provider: provider, name: name)
        return .result()
    }
}

struct UsageWidgetView: View {
    let entry: UsageEntry
    @Environment(\.widgetFamily) private var family

    var body: some View {
        GeometryReader { geo in
            let count = CGFloat(max(entry.model.rings.count, 1))
            let small = family == .systemSmall
            let diameter = min(small ? 30 : 34, (geo.size.width - (count - 1) * 2) / count - 2)
            UsageView(model: entry.model, diameter: diameter, compact: true) { ring, selected in
                Button(intent: SelectAccountIntent(ring.id)) {
                    RingCell(ring: ring, selected: selected, diameter: diameter, showsName: !small)
                }
                .buttonStyle(.plain)
            }
        }
    }
}
