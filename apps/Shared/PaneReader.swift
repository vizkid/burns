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
    /// Each host's money, by the same rules.
    private var costs: [HostCost]?
    @ObservationIgnored private var running: Task<Void, Never>?
    /// The addresses the last read covered.
    @ObservationIgnored private var read: [String] = []
    /// The name each address was last read under. A new host is named from its address until its
    /// usage answers with its own name, so today's name can differ from the stored answer's.
    @ObservationIgnored private var readNames: [String: String] = [:]

    /// `icons` reads the host icons on a device that can ask Rex.
    func reload(_ store: UsageStore, icons: (@MainActor ([HostStatus]) async -> [String: String]?)? = nil) async {
        if let running { return await running.value }
        let task = Task {
            // Panes start from the cached host list, so a slow host cannot hold them back.
            async let panes: Void = readPanes(store.statuses)
            async let cost: Void = readCost(store.statuses)
            await store.refresh()
            if let found = await icons?(store.statuses) { store.addHostIcons(found) }
            _ = await (panes, cost)
        }
        running = task
        await task.value
        running = nil
        catchUp(store)
    }

    /// Reads panes for hosts that a reload did not cover, such as one added since. Adding a host
    /// refreshes usage alone, so without this its panes would wait for the next window opening.
    /// A removed host needs no read: `shares` and `cost` drop it at once.
    func catchUp(_ store: UsageStore) {
        let addresses = store.statuses.map(\.address)
        guard running == nil, !addresses.allSatisfy(read.contains) else { return }
        running = Task {
            async let panes: Void = readPanes(store.statuses)
            async let cost: Void = readCost(store.statuses)
            _ = await (panes, cost)
            running = nil
            // A host added while this ran was turned away by the guard above, so look again.
            catchUp(store)
        }
    }

    private func readPanes(_ statuses: [HostStatus]) async {
        read = statuses.map(\.address)
        let names = statuses.map { (address: $0.address, name: Self.name($0)) }
        for h in names { readNames[h.address] = h.name }
        var answers = (hosts ?? []).reduce(into: [String: HostPanes]()) { $0[$1.host] = $1 }
        for h in names where answers[h.name] == nil { answers[h.name] = HostPanes(host: h.name, icon: "", rows: nil, live: nil) }
        // Assigned before any answer, so a removed host goes now and no hosts clears the list.
        hosts = names.compactMap { answers[$0.name] }
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

    private func readCost(_ statuses: [HostStatus]) async {
        let names = statuses.map { (address: $0.address, name: Self.name($0)) }
        var answers = (costs ?? []).reduce(into: [String: HostCost]()) { $0[$1.host] = $1 }
        costs = names.compactMap { answers[$0.name] }
        await withTaskGroup(of: HostCost.self) { group in
            for h in names {
                group.addTask { HostCost(host: h.name, answer: try? await HostFetcher().cost(h.address)) }
            }
            for await answer in group {
                answers[answer.host] = answer
                costs = names.compactMap { answers[$0.name] }
            }
        }
    }

    /// The selected account's money, with today's host names. Azure has no window, so its period is the last 7 days.
    func cost(_ store: UsageStore) -> CostSummary? {
        guard let costs, let b = store.model.burndown else { return nil }
        let shown = current(costs, \.host, store).map { HostCost(host: store.hostNames[$0.host] ?? $0.host, answer: $0.answer) }
        return CostSummary.build(shown, account: b.account, windowStart: b.windowStart ?? Date(timeIntervalSinceNow: -7 * 86_400), limitAt: b.limitAt)
    }

    /// Answers from hosts still in settings. A read keeps a removed host's answer until the next
    /// read, which removing a host does not start.
    private func current<T>(_ answers: [T], _ host: KeyPath<T, String>, _ store: UsageStore) -> [T] {
        let names = Set(store.statuses.flatMap { [Self.name($0), readNames[$0.address]].compactMap { $0 } })
        return answers.filter { names.contains($0[keyPath: host]) }
    }

    static func name(_ s: HostStatus) -> String {
        s.snapshot?.usage.host ?? s.address.split(separator: ".").first.map(String.init) ?? s.address
    }

    /// The selected account's shares, with today's host names and icons.
    func shares(_ store: UsageStore) -> PaneShares? {
        guard let hosts, let id = store.model.selected else { return nil }
        let shown = current(hosts, \.host, store).map { h in
            var named = h
            named.host = store.hostNames[h.host] ?? h.host
            named.icon = store.hostIcons[h.host] ?? HostIcon.fallback
            return named
        }
        return PaneShares.build(shown, account: id)
    }
}

/// Money on a spent window, and on Azure. Both apps draw it between the chart and the panes.
struct CostPanel: View {
    let store: UsageStore
    let reader: PaneReader

    var body: some View {
        // Money charged in this reset period always shows, even with room left in the week: the
        // five-hour limit can start paid usage (Viz, 2026-11-02). The chart squashes only when spent.
        if let b = store.model.burndown, b.showsCost || reader.cost(store)?.hasSpend == true {
            CostView(summary: reader.cost(store), period: b.windowStart == nil ? "Last 7 days" : "This window")
            Divider()
        }
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
