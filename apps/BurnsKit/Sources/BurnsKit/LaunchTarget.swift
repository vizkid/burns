import Foundation

/// Where `burns://new?host=H&session=S` asks the panel to start: the session the New Agent Session
/// Rex action ran in. `host` is the Rex server's label, and empty for this Mac's own server.
public struct LaunchTarget: Equatable, Sendable {
    public var host: String
    public var session: String
    /// The focused block, from the beside key: the launch splits it.
    public var block: String?
    public init(host: String, session: String, block: String? = nil) { self.host = host; self.session = session; self.block = block }

    /// Nil without a session. A host that is this Mac, under any of its names, becomes empty, because
    /// the panel runs this Mac's launches without `-S`.
    public init?(url: URL, here: String) {
        let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems ?? []
        let value = { (name: String) in items.first { $0.name == name }?.value ?? "" }
        guard !value("session").isEmpty else { return nil }
        let host = value("host")
        self.init(host: RexHosts.short(host) == RexHosts.short(here) ? "" : host, session: value("session"),
                  block: value("block").isEmpty ? nil : value("block"))
    }

    func matches(_ h: LaunchHost) -> Bool {
        if let block { return h.block == block }
        return h.block == nil && h.create == nil && h.session == session
            && (h.rex.isEmpty ? host.isEmpty : RexHosts.short(h.rex) == RexHosts.short(host))
    }
}

/// A pane's folder, from `rex block call process`. Rex reports the process for every pane. `pwd`
/// answers only when the shell reports its folder, and a pane started as an agent's command does not.
public enum RexFolder {
    private struct Process: Decodable {
        struct Proc: Decodable { let cwd: String? }
        let child: Proc?
        let foreground: Proc?
    }

    /// The pane's own process's folder, else its foreground program's.
    public static func path(process: Data) -> String? {
        guard let p = try? JSONDecoder().decode(Process.self, from: process) else { return nil }
        return [p.child?.cwd, p.foreground?.cwd].compactMap { $0 }.first { !$0.isEmpty }
    }
}
