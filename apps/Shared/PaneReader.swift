import BurnsKit
import Observation
import SwiftUI

enum HostIcon {
    /// A host with no icon picked in Rex.
    static let fallback = "desktopcomputer"
}

/// The one reload both apps use: usage and panes together. It runs in its own task because
/// SwiftUI cancels the task of `.refreshable` and `.task`, and a cancelled read looks like every
/// host failing. Callers share a reload already running.
@MainActor
@Observable
final class PaneReader {
    /// Each host's answer, without its display name or icon: those can change between reads.
    private var hosts: [HostPanes]?
    @ObservationIgnored private var running: Task<Void, Never>?
    /// The addresses the last read covered.
    @ObservationIgnored private var read: [String] = []

    /// `icons` reads the host icons on a device that can ask Rex.
    func reload(_ store: UsageStore, icons: (@MainActor ([HostStatus]) async -> [String: String]?)? = nil) async {
        if let running { return await running.value }
        let task = Task {
            // Panes start from the cached host list, so a slow host cannot hold them back.
            async let panes: Void = readPanes(store.statuses)
            await store.refresh()
            if let found = await icons?(store.statuses) { store.addHostIcons(found) }
            _ = await panes
        }
        running = task
        await task.value
        running = nil
        catchUp(store)
    }

    /// Reads panes for hosts that a reload did not cover, such as one added since. Adding a host
    /// refreshes usage alone, so without this its panes would wait for the next window opening.
    func catchUp(_ store: UsageStore) {
        let addresses = store.statuses.map(\.address)
        guard running == nil, !addresses.allSatisfy(read.contains) else { return }
        running = Task {
            await readPanes(store.statuses)
            running = nil
        }
    }

    private func readPanes(_ statuses: [HostStatus]) async {
        read = statuses.map(\.address)
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
    let store: UsageStore
    let reader: PaneReader

    var body: some View {
        if !store.model.rings.isEmpty {
            PaneShareView(shares: reader.shares(store), account: store.model.selected,
                          tint: store.model.rings.first { $0.id == store.model.selected }?.swatch.color ?? .secondary)
                .onChange(of: store.statuses.map(\.address), initial: true) { reader.catchUp(store) }
        }
    }
}
