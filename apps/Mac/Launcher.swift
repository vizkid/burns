import AppKit
import BurnsKit
import SwiftUI

/// Runs `rex`. The app starts without the shell's PATH, so its path comes from a login shell.
enum Rex {
    static let path: String? = {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/bin/zsh")
        task.arguments = ["-ilc", "whence -p rex"]
        let out = Pipe()
        task.standardOutput = out
        task.standardError = Pipe()
        guard (try? task.run()) != nil else { return nil }
        task.waitUntilExit()
        let text = String(data: out.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
        return text.split(separator: "\n").last.map { String($0).trimmingCharacters(in: .whitespaces) }
    }()

    /// Output, or an error's text. Each argument passes as it is, with no shell.
    static func run(_ args: [String]) async -> Result<Data, RexFailure> {
        guard let path else { return .failure(RexFailure(text: "rex was not found")) }
        return await withCheckedContinuation { done in
            let task = Process()
            task.executableURL = URL(fileURLWithPath: path)
            task.arguments = args
            let out = Pipe(), err = Pipe()
            task.standardOutput = out
            task.standardError = err
            task.terminationHandler = { t in
                let data = out.fileHandleForReading.readDataToEndOfFile()
                let text = String(data: err.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
                done.resume(returning: t.terminationStatus == 0 ? .success(data) : .failure(RexFailure(text: text)))
            }
            do { try task.run() } catch { done.resume(returning: .failure(RexFailure(text: "\(error)"))) }
        }
    }

    static func json<T: Decodable>(_ type: T.Type, _ args: [String]) async -> T? {
        guard case .success(let data) = await run(args) else { return nil }
        return try? JSONDecoder().decode(type, from: data)
    }
}

struct RexFailure: Error { let text: String }

/// What the panel needs from Rex and smithersd, read when it opens.
enum LaunchData {
    private struct HostList: Decodable {
        struct Row: Decodable { struct Host: Decodable { let label: String }; let host: Host }
        let hosts: [Row]
    }
    private struct Sessions: Decodable {
        struct Row: Decodable { let label: String }
        let sessions: [Row]
    }
    private struct Blocks: Decodable {
        struct Row: Decodable { let block_id: String }
        let blocks: [Row]
    }
    private struct Pwd: Decodable { let pwd: String }

    /// One host's folders. Rex reports a pane's folder as a file URL.
    private static func folders(rex: String, shown: String) async -> [(host: LaunchHost, relative: String)] {
        let server = rex.isEmpty ? [] : ["-S", rex]
        guard let sessions = await Rex.json(Sessions.self, server + ["session", "ls", "--json"]) else { return [] }
        var out: [(host: LaunchHost, relative: String)] = []
        for s in sessions.sessions {
            guard let block = await Rex.json(Blocks.self, server + ["block", "ls", "-s", s.label, "--json"])?.blocks.first,
                  let pwd = await Rex.json(Pwd.self, server + ["block", "call", "--block", block.block_id, "pwd"]),
                  let path = URL(string: pwd.pwd)?.path else { continue }
            let parts = path.split(separator: "/").map(String.init)
            // /Users/<name>/... and /home/<name>/... are the same place on each host.
            guard parts.count > 2, parts[0] == "Users" || parts[0] == "home" else { continue }
            out.append((LaunchHost(rex: rex, shown: shown, path: path, session: s.label), parts[2...].joined(separator: "/")))
        }
        return out
    }

    private static func models(_ host: String) async -> [ModelEntry] {
        guard let url = URL(string: "http://\(host):\(HostFetcher.port)/models"),
              let (data, _) = try? await URLSession(configuration: .ephemeral).data(from: url) else { return [] }
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return (try? decoder.decode(ModelsAnswer.self, from: data))?.accounts ?? []
    }

    @MainActor
    static func sentence(store: UsageStore) async -> LaunchSentence {
        let names = store.hostNames
        let rexHosts = await Rex.json(HostList.self, ["hosts", "list", "--json"])?.hosts.map(\.host.label) ?? []
        var found: [(host: LaunchHost, relative: String)] = []
        var entries: [AccountID: ModelEntry] = [:]
        for status in store.statuses {
            guard let host = status.snapshot?.usage.host else { continue }
            let rex = rexHosts.first { $0.lowercased() == host.lowercased() } ?? ""
            found += await folders(rex: rex, shown: names[host] ?? host)
            for e in await models(status.address) where entries[AccountID(provider: e.provider, name: e.account)] == nil {
                entries[AccountID(provider: e.provider, name: e.account)] = e
            }
        }
        var projects = LaunchProject.group(found)
        // A host with no session on a project can start one, when it has any session to show it runs Rex.
        let all = found.reduce(into: [LaunchHost]()) { list, f in if !list.contains(where: { $0.rex == f.host.rex }) { list.append(f.host) } }
        for i in projects.indices {
            for h in all where !projects[i].hosts.contains(where: { $0.rex == h.rex }) {
                let parts = h.path.split(separator: "/").map(String.init)
                let path = "/" + parts.prefix(2).joined(separator: "/") + "/" + projects[i].path
                projects[i].hosts.append(LaunchHost(rex: h.rex, shown: h.shown, path: path, session: nil))
            }
        }
        return LaunchSentence(accounts: store.launchAccounts(entries), projects: projects)
    }
}

extension UsageStore {
    /// The accounts in the order the default rule ranks them, best first.
    func launchAccounts(_ entries: [AccountID: ModelEntry]) -> [LaunchAccount] {
        let groups = AccountGroup.build(central)
        let now = Date()
        func left(_ a: AccountUsage) -> Double? {
            guard a.isQuota else { return nil }
            let live = a.liveWindows(at: now).values.compactMap(\.percent)
            return live.max().map { 100 - $0 }
        }
        let usage = Merge.accounts(statuses.compactMap(\.snapshot?.usage), now: now)
        var ranked = usage.sorted { (left($0) ?? ($0.isQuota ? -1 : -2)) > (left($1) ?? ($1.isQuota ? -1 : -2)) }
        // The default rule decides the top: with every quota exhausted, that is Azure.
        if let pick = model.pick.account, let i = ranked.firstIndex(where: { $0.id == pick }) { ranked.insert(ranked.remove(at: i), at: 0) }
        return ranked.map { a in
            let words = !a.isQuota ? "credits" : left(a).map { "\(Int($0.rounded()))% left" } ?? "usage unknown"
            return LaunchAccount(id: a.id, emoji: groups.first { $0.name == a.id.name }?.emoji ?? "⚪", left: words, entry: entries[a.id])
        }
    }
}

/// The floating panel. Borderless panels refuse key status unless they say otherwise.
private final class KeyPanel: NSPanel {
    override var canBecomeKey: Bool { true }
}

@MainActor
enum LaunchWindow {
    private static var panel: NSPanel?
    static weak var store: UsageStore?

    static func show() {
        guard let store, panel == nil else { panel?.makeKeyAndOrderFront(nil); return }
        Task {
            let sentence = await LaunchData.sentence(store: store)
            guard panel == nil else { return }
            let w = KeyPanel(contentRect: NSRect(x: 0, y: 0, width: 820, height: 400), styleMask: [.borderless, .nonactivatingPanel],
                             backing: .buffered, defer: false)
            w.isOpaque = false
            w.backgroundColor = .clear
            w.hasShadow = true
            w.level = .floating
            w.contentView = NSHostingView(rootView: LaunchPanel(sentence: sentence, launch: start, close: close))
            w.center()
            panel = w
            NSApp.activate()
            w.makeKeyAndOrderFront(nil)
        }
    }

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
