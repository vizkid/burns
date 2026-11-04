import AppKit
import BurnsKit
import SwiftUI

/// What the panel needs from Rex and smithersd, read behind the open panel.
enum LaunchData {
    private struct Sessions: Decodable {
        struct Row: Decodable { let label: String }
        let sessions: [Row]
    }
    private struct Blocks: Decodable {
        struct Row: Decodable { let block_id: String }
        let blocks: [Row]
    }
    private struct Folder: Sendable {
        let session: String
        let path: String
    }

    /// One host's last answers, so a host that stops answering keeps its place in the lists.
    private struct HostRead: Sendable {
        var entries: [AccountID: ModelEntry] = [:]
        var folders: [Folder] = []
    }

    @MainActor private static var known: [String: HostRead] = [:]
    /// The hosts of the last read, for completing a typed folder.
    @MainActor private static var lastHosts: [Place] = []
    /// Rex's last answer, so that one timeout does not drop every remote host.
    @MainActor private static var rexHosts: RexHosts?

    /// One host's sessions and their folders, or nil when Rex did not answer.
    private static func folders(rex: String) async -> [Folder]? {
        let server = rex.isEmpty ? [] : ["-S", rex]
        guard let sessions = await Rex.json(Sessions.self, server + ["session", "ls", "--json"])?.sessions else { return nil }
        return await withTaskGroup(of: (Int, Folder?).self) { group in
            for (i, s) in sessions.enumerated() {
                group.addTask {
                    guard let blocks = await Rex.json(Blocks.self, server + ["block", "ls", "-s", s.label, "--json"])?.blocks else { return (i, nil) }
                    // The first pane that answers names the session's folder.
                    for b in blocks {
                        guard case .success(let data) = await Rex.run(server + ["block", "call", "--block", b.block_id, "process"], limit: 5),
                              let path = RexFolder.path(process: data) else { continue }
                        return (i, Folder(session: s.label, path: path))
                    }
                    return (i, nil)
                }
            }
            var out: [(Int, Folder?)] = []
            for await row in group { out.append(row) }
            return out.sorted { $0.0 < $1.0 }.compactMap(\.1)
        }
    }

    /// Each host's parts, at the same time, with an empty or nil part where the host did not answer.
    private static func read(_ hosts: [(address: String, rex: String)]) async -> [(entries: [AccountID: ModelEntry], folders: [Folder]?)] {
        await withTaskGroup(of: (Int, [AccountID: ModelEntry], [Folder]?).self) { group in
            for (i, h) in hosts.enumerated() {
                group.addTask {
                    async let entries = HostFetcher().models(h.address)
                    async let found = folders(rex: h.rex)
                    return await (i, entries, found)
                }
            }
            var out: [(Int, [AccountID: ModelEntry], [Folder]?)] = []
            for await row in group { out.append(row) }
            return out.sorted { $0.0 < $1.0 }.map { ($0.1, $0.2) }
        }
    }

    /// Each host and where a launch on it goes. A host that Rex cannot reach is left out, with why.
    @MainActor
    private static func places(store: UsageStore) async -> (hosts: [Place], unreachable: [String]) {
        if let answer = await Rex.hosts() { rexHosts = answer }
        var hosts: [Place] = [], unreachable: [String] = []
        for status in store.statuses {
            guard let name = status.snapshot?.usage.host else { unreachable.append("\(status.address): no answer yet"); continue }
            switch RexHosts.place(host: name, here: RexHosts.thisMachine, rex: rexHosts) {
            case .here: hosts.append(Place(address: status.address, name: name, rex: ""))
            case .server(let rex): hosts.append(Place(address: status.address, name: name, rex: rex))
            case .unreachable(let why): unreachable.append("\(store.hostNames[name] ?? name): \(why)")
            }
        }
        return (hosts, unreachable)
    }

    @MainActor
    static func sentence(store: UsageStore) async -> LaunchSentence {
        let names = store.hostNames
        let (hosts, unreachable) = await places(store: store)
        lastHosts = hosts
        let answers = await read(hosts.map { (address: $0.address, rex: $0.rex) })
        var found: [(host: LaunchHost, relative: String)] = []
        var entries: [AccountID: ModelEntry] = [:]
        for (h, answer) in zip(hosts, answers) {
            let last = known[h.address]
            let here = HostRead(entries: answer.entries.isEmpty ? last?.entries ?? [:] : answer.entries, folders: answer.folders ?? last?.folders ?? [])
            known[h.address] = here
            entries.merge(here.entries) { first, _ in first }
            for f in here.folders {
                guard let relative = LaunchProject.relative(f.path) else { continue }
                let host = LaunchHost(rex: h.rex, shown: names[h.name] ?? h.name, path: f.path, session: f.session, entries: here.entries)
                found.append((host, relative))
            }
        }
        var projects = LaunchProject.group(found)
        if let h = hosts.first(where: { $0.rex.isEmpty }) {
            let local = LaunchHost(rex: "", shown: names[h.name] ?? h.name, path: "", session: nil, entries: known[h.address]?.entries ?? [:])
            projects = LaunchProject.offer(local, home: NSHomeDirectory(), to: projects) { FileManager.default.fileExists(atPath: $0) }
        }
        return store.launchSentence(entries, projects: projects, unreachable: unreachable)
    }

    /// Each host's folders under a typed path, from its smithersd, as projects with a new-session
    /// row on each host that has the folder.
    @MainActor
    static func folders(under prefix: String, store: UsageStore) async -> [LaunchProject] {
        let hosts = lastHosts, names = store.hostNames
        let answers = await withTaskGroup(of: (Int, DirsAnswer?).self) { group in
            for (i, h) in hosts.enumerated() { group.addTask { (i, await HostFetcher().dirs(h.address, prefix: prefix)) } }
            var out: [(Int, DirsAnswer?)] = []
            for await row in group { out.append(row) }
            return out.sorted { $0.0 < $1.0 }.map(\.1)
        }
        var found: [(host: LaunchHost, relative: String)] = []
        for (h, answer) in zip(hosts, answers) {
            for dir in answer?.dirs ?? [] {
                let host = LaunchHost(rex: h.rex, shown: names[h.name] ?? h.name, path: answer!.home + "/" + dir, session: nil,
                                      entries: known[h.address]?.entries ?? [:])
                found.append((host, dir))
            }
        }
        return LaunchProject.group(found)
    }

    /// The pane the beside key was pressed in, with its folder from `process`. Nil without a block,
    /// or when its host is not in the last read.
    @MainActor
    static func pane(_ target: LaunchTarget?, store: UsageStore) async -> LaunchPane? {
        guard let target, let block = target.block,
              let h = lastHosts.first(where: { $0.rex.isEmpty ? target.host.isEmpty : RexHosts.short($0.rex) == RexHosts.short(target.host) })
        else { return nil }
        let server = h.rex.isEmpty ? [] : ["-S", h.rex]
        guard case .success(let data) = await Rex.run(server + ["block", "call", "--block", block, "process"], limit: 5),
              let path = RexFolder.path(process: data), let relative = LaunchProject.relative(path), !relative.isEmpty else { return nil }
        return LaunchPane(rex: h.rex, shown: store.hostNames[h.name] ?? h.name, session: target.session, block: block, path: path, relative: relative)
    }

    private struct Place: Sendable {
        let address: String
        let name: String
        let rex: String
    }
}

extension UsageStore {
    /// One store for the app: `burns://new` can arrive before any view exists.
    static let shared = UsageStore()

    /// The sentence from the store's own usage, ranked by the default rule.
    func launchSentence(_ entries: [AccountID: ModelEntry], projects: [LaunchProject], unreachable: [String]) -> LaunchSentence {
        LaunchSentence(accounts: LaunchAccount.ranked(model.accounts, pick: model.pick.account, central: central, entries: entries, now: Date()),
                       projects: projects, reason: model.pick.reason(among: model.accounts), unreachable: unreachable)
    }
}

/// The floating panel. Borderless panels refuse key status unless they say otherwise.
private final class KeyPanel: NSPanel {
    override var canBecomeKey: Bool { true }
}

@MainActor
enum LaunchWindow {
    private static var panel: NSPanel?
    private static var last: LaunchSentence?
    private static var reading: Task<LaunchSentence, Never>?

    /// Opens at once: a read takes seconds, and longer with a host asleep. The panel starts from the
    /// last read's projects and models, ranked by what the store knows now, and takes the new read
    /// when it arrives.
    /// A target starts the panel on that session. A panel already open for another target opens anew.
    static func show(at target: LaunchTarget? = nil) {
        if target != nil { close() }
        guard panel == nil else { panel?.makeKeyAndOrderFront(nil); return }
        let entries = (last?.accounts ?? []).reduce(into: [AccountID: ModelEntry]()) { $0[$1.id] = $1.entry }
        var sentence = UsageStore.shared.launchSentence(entries, projects: last?.projects ?? [], unreachable: last?.unreachable ?? [])
        if let target { sentence.aim(target) }
        let w = KeyPanel(contentRect: NSRect(x: 0, y: 0, width: 820, height: 400), styleMask: [.borderless, .nonactivatingPanel],
                         backing: .buffered, defer: false)
        w.isOpaque = false
        w.backgroundColor = .clear
        w.hasShadow = true
        w.level = .floating
        let refresh = {
            var fresh = await read()
            fresh.pane = await LaunchData.pane(target, store: .shared)
            return fresh
        }
        let complete = { (prefix: String) in await LaunchData.folders(under: prefix, store: .shared) }
        w.contentView = NSHostingView(rootView: LaunchPanel(sentence: sentence, refresh: refresh, complete: complete, launch: start, close: close))
        w.center()
        panel = w
        NSApp.activate()
        w.makeKeyAndOrderFront(nil)
    }

    /// One read at a time, shared. It is its own task: closing the panel cancels the panel's wait,
    /// and a cancelled request would look like a host that did not answer.
    private static func read() async -> LaunchSentence {
        let task = reading ?? Task {
            let sentence = await LaunchData.sentence(store: .shared)
            last = sentence
            reading = nil
            return sentence
        }
        reading = task
        return await task.value
    }

    /// Reads ahead of the plus button, so that the panel opens with the lists already filled.
    static func prepare() { Task { _ = await read() } }

    static func close() {
        panel?.orderOut(nil)
        panel = nil
    }

    private static func start(_ request: LaunchRequest) async -> String? {
        if case .failure(let f) = await Rex.run(request.arguments) { return f.text.trimmingCharacters(in: .whitespacesAndNewlines) }
        return nil
    }
}

/// `burns://new` opens the panel, from a Rex action or a script. `?host=H&session=S` starts it on
/// that session.
final class BurnsDelegate: NSObject, NSApplicationDelegate {
    func application(_ application: NSApplication, open urls: [URL]) {
        guard let url = urls.last(where: { $0.scheme == "burns" }) else { return }
        LaunchWindow.show(at: LaunchTarget(url: url, here: RexHosts.thisMachine))
    }
}
