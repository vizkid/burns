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
    private struct Pwd: Decodable { let pwd: String }

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
    /// Rex's last answer, so that one timeout does not drop every remote host.
    @MainActor private static var rexHosts: RexHosts?

    /// One host's sessions and their folders, or nil when Rex did not answer. Rex reports a pane's
    /// folder as a file URL.
    private static func folders(rex: String) async -> [Folder]? {
        let server = rex.isEmpty ? [] : ["-S", rex]
        guard let sessions = await Rex.json(Sessions.self, server + ["session", "ls", "--json"])?.sessions else { return nil }
        return await withTaskGroup(of: (Int, Folder?).self) { group in
            for (i, s) in sessions.enumerated() {
                group.addTask {
                    guard let block = await Rex.json(Blocks.self, server + ["block", "ls", "-s", s.label, "--json"])?.blocks.first,
                          let pwd = await Rex.json(Pwd.self, server + ["block", "call", "--block", block.block_id, "pwd"]),
                          let path = URL(string: pwd.pwd)?.path else { return (i, nil) }
                    return (i, Folder(session: s.label, path: path))
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
        let answers = await read(hosts.map { (address: $0.address, rex: $0.rex) })
        var found: [(host: LaunchHost, relative: String)] = []
        var entries: [AccountID: ModelEntry] = [:]
        for (h, answer) in zip(hosts, answers) {
            let last = known[h.address]
            let here = HostRead(entries: answer.entries.isEmpty ? last?.entries ?? [:] : answer.entries, folders: answer.folders ?? last?.folders ?? [])
            known[h.address] = here
            entries.merge(here.entries) { first, _ in first }
            for f in here.folders {
                let parts = f.path.split(separator: "/").map(String.init)
                // /Users/<name>/... and /home/<name>/... are the same place on each host.
                guard parts.count > 2, parts[0] == "Users" || parts[0] == "home" else { continue }
                let host = LaunchHost(rex: h.rex, shown: names[h.name] ?? h.name, path: f.path, session: f.session, entries: here.entries)
                found.append((host, parts[2...].joined(separator: "/")))
            }
        }
        var projects = LaunchProject.group(found)
        if let h = hosts.first(where: { $0.rex.isEmpty }) {
            let local = LaunchHost(rex: "", shown: names[h.name] ?? h.name, path: "", session: nil, entries: known[h.address]?.entries ?? [:])
            projects = LaunchProject.offer(local, home: NSHomeDirectory(), to: projects) { FileManager.default.fileExists(atPath: $0) }
        }
        return store.launchSentence(entries, projects: projects, unreachable: unreachable)
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
    static func show() {
        guard panel == nil else { panel?.makeKeyAndOrderFront(nil); return }
        let entries = (last?.accounts ?? []).reduce(into: [AccountID: ModelEntry]()) { $0[$1.id] = $1.entry }
        let sentence = UsageStore.shared.launchSentence(entries, projects: last?.projects ?? [], unreachable: last?.unreachable ?? [])
        let w = KeyPanel(contentRect: NSRect(x: 0, y: 0, width: 820, height: 400), styleMask: [.borderless, .nonactivatingPanel],
                         backing: .buffered, defer: false)
        w.isOpaque = false
        w.backgroundColor = .clear
        w.hasShadow = true
        w.level = .floating
        w.contentView = NSHostingView(rootView: LaunchPanel(sentence: sentence, refresh: read, launch: start, close: close))
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

/// `burns://new` opens the panel, from a Rex action or a script.
final class BurnsDelegate: NSObject, NSApplicationDelegate {
    func application(_ application: NSApplication, open urls: [URL]) {
        if urls.contains(where: { $0.scheme == "burns" }) { LaunchWindow.show() }
    }
}
