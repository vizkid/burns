import BurnsKit
import SwiftUI

enum HostIcon {
    /// A host with no icon picked in Rex.
    static let fallback = "desktopcomputer"
}

/// The one reload both apps use: usage and panes together. It runs in its own task because
/// SwiftUI cancels the task of `.refreshable` and `.task`, and a cancelled read looks like every
/// host failing. Callers share a reload already running.
@MainActor
final class PaneReader: ObservableObject {
    /// Each host's answer, without its display name or icon: those can change between reads.
    @Published private var hosts: [HostPanes]?
    private var running: Task<Void, Never>?

    /// `icons` reads the host icons on a device that can ask Rex.
    func reload(_ store: UsageStore, icons: (@MainActor ([HostStatus]) async -> [String: String]?)? = nil) async {
        if let running { return await running.value }
        let task = Task {
            // Panes start from the cached host list, so a slow host cannot hold them back.
            async let panes: Void = read(store.statuses)
            await store.refresh()
            if let found = await icons?(store.statuses) { store.addHostIcons(found) }
            _ = await panes
        }
        running = task
        await task.value
        running = nil
    }

    private func read(_ statuses: [HostStatus]) async {
        let names = statuses.map { (address: $0.address, name: Self.name($0)) }
        var answers = (hosts ?? []).reduce(into: [String: HostPanes]()) { $0[$1.host] = $1 }
        for h in names where answers[h.name] == nil { answers[h.name] = HostPanes(host: h.name, icon: "", rows: nil, live: nil) }
        await withTaskGroup(of: HostPanes.self) { group in
            for h in names {
                group.addTask {
                    guard let answer = try? await HostFetcher().panes(h.address) else { return HostPanes(host: h.name, icon: "", rows: nil, live: nil) }
                    return HostPanes(host: h.name, icon: "", answer: answer)
                }
            }
            // Each host shows as it answers, so one offline host holds back only its own panes.
            for await answer in group {
                answers[answer.host] = answer
                hosts = names.compactMap { answers[$0.name] }
            }
        }
    }

    static func name(_ s: HostStatus) -> String {
        s.snapshot?.usage.host ?? s.address.split(separator: ".").first.map(String.init) ?? s.address
    }

    /// The selected account's shares, with today's host names and icons.
    func shares(_ store: UsageStore) -> PaneShares? {
        guard let hosts, let id = store.model.selected else { return nil }
        let shown = hosts.map { h in
            var named = h
            named.host = store.hostNames[h.host] ?? h.host
            named.icon = store.hostIcons[h.host] ?? HostIcon.fallback
            return named
        }
        return PaneShares.build(shown, account: id)
    }
}

/// The share view both apps draw, in the selected ring's colour.
struct LivePanes: View {
    @ObservedObject var store: UsageStore
    @ObservedObject var reader: PaneReader

    var body: some View {
        if !store.model.rings.isEmpty {
            PaneShareView(shares: reader.shares(store), account: store.model.selected,
                          tint: store.model.rings.first { $0.id == store.model.selected }?.swatch.color ?? .secondary)
        }
    }
}
