import Foundation

/// smithersd's `GET /models`: what each account can run on one host.
public struct ModelsAnswer: Codable, Equatable, Sendable {
    public var host: String
    public var accounts: [ModelEntry]
}

public struct CatalogModel: Codable, Equatable, Sendable, Hashable {
    public var id: String
    public var display: String
    public var efforts: [String]?
    public init(id: String, display: String, efforts: [String]? = nil) { self.id = id; self.display = display; self.efforts = efforts }
}

public struct ModelEntry: Codable, Equatable, Sendable {
    public var provider: String
    public var account: String
    public var defaultId: String
    public var `default`: String
    public var defaultEffort: String
    public var models: [CatalogModel]

    private enum CodingKeys: String, CodingKey { case provider, account, defaultId, `default`, defaultEffort, models }

    /// An account with no catalog can arrive as `null`, which is an empty list.
    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        provider = try c.decode(String.self, forKey: .provider)
        account = try c.decode(String.self, forKey: .account)
        defaultId = try c.decode(String.self, forKey: .defaultId)
        self.default = try c.decode(String.self, forKey: .default)
        defaultEffort = try c.decode(String.self, forKey: .defaultEffort)
        models = try c.decodeIfPresent([CatalogModel].self, forKey: .models) ?? []
    }

    public init(provider: String, account: String, defaultId: String, default d: String, defaultEffort: String, models: [CatalogModel]) {
        self.provider = provider; self.account = account; self.defaultId = defaultId
        self.default = d; self.defaultEffort = defaultEffort; self.models = models
    }
}

/// One account the launch sentence can bill.
public struct LaunchAccount: Equatable, Sendable {
    public var id: AccountID
    public var emoji: String
    /// In words: "96% left", "usage unknown", "credits".
    public var left: String
    public var entry: ModelEntry?
    public init(id: AccountID, emoji: String, left: String, entry: ModelEntry?) { self.id = id; self.emoji = emoji; self.left = left; self.entry = entry }

    /// The accounts best first, with the default rule's pick on top: with every quota exhausted,
    /// that is Azure. Quota comes before unknown usage, and unknown before credits. A tie keeps the
    /// merged order.
    public static func ranked(_ accounts: [AccountUsage], pick: AccountID?, central: [CentralAccount], entries: [AccountID: ModelEntry], now: Date) -> [LaunchAccount] {
        let groups = AccountGroup.build(central)
        let rank = { (a: AccountUsage) in a.quotaLeft(at: now) ?? (a.isQuota ? -1 : -2) }
        var ordered = accounts.enumerated().sorted { (rank($1.element), $0.offset) < (rank($0.element), $1.offset) }.map(\.element)
        if let i = ordered.firstIndex(where: { $0.id == pick }) { ordered.insert(ordered.remove(at: i), at: 0) }
        return ordered.map { a in
            let words = !a.isQuota ? "credits" : a.quotaLeft(at: now).map { "\(Int($0.rounded()))% left" } ?? "usage unknown"
            return LaunchAccount(id: a.id, emoji: groups.first { $0.name == a.id.name }?.emoji ?? "⚪", left: words, entry: entries[a.id])
        }
    }
}

/// A place a project can run. An empty `rex` is this Mac's own server. A nil session starts one.
public struct LaunchHost: Equatable, Sendable {
    public var rex: String
    public var shown: String
    public var path: String
    public var session: String?
    /// What each account runs on this host: a default is a fact of the host's home.
    public var entries: [AccountID: ModelEntry]
    public init(rex: String, shown: String, path: String, session: String?, entries: [AccountID: ModelEntry] = [:]) {
        self.rex = rex; self.shown = shown; self.path = path; self.session = session; self.entries = entries
    }
}

public struct LaunchProject: Equatable, Sendable {
    public var name: String
    public var path: String
    public var hosts: [LaunchHost]
    public init(name: String, path: String, hosts: [LaunchHost]) { self.name = name; self.path = path; self.hosts = hosts }

    /// Folders from each host's sessions, one project per folder relative to the user's home, so
    /// the same checkout on two hosts is one project. A home folder is no project.
    public static func group(_ found: [(host: LaunchHost, relative: String)]) -> [LaunchProject] {
        var out: [LaunchProject] = []
        for item in found where !item.relative.isEmpty {
            if let i = out.firstIndex(where: { $0.path == item.relative }) {
                if !out[i].hosts.contains(where: { $0.rex == item.host.rex }) { out[i].hosts.append(item.host) }
            } else {
                let name = item.relative.split(separator: "/").last.map(String.init) ?? item.relative
                out.append(LaunchProject(name: name, path: item.relative, hosts: [item.host]))
            }
        }
        return out
    }

    /// Offers `host` for the projects it has no session on, where `exists` finds the folder there. A
    /// path built from another host's prefix would be a guess, and only this Mac can look.
    public static func offer(_ host: LaunchHost, home: String, to projects: [LaunchProject], exists: (String) -> Bool) -> [LaunchProject] {
        projects.map { project in
            let path = home + "/" + project.path
            guard !project.hosts.contains(where: { $0.rex == host.rex }), exists(path) else { return project }
            var offered = project
            offered.hosts.append(LaunchHost(rex: host.rex, shown: host.shown, path: path, session: nil, entries: host.entries))
            return offered
        }
    }
}

/// The launch the sentence describes, ready for Rex.
public struct LaunchRequest: Equatable, Sendable {
    public var host: LaunchHost
    public var project: String
    public var provider: String
    public var account: String
    public var model: String?
    public var effort: String?
    public var emoji: String

    /// Arguments for `rex`. Each value is one argument, never shell code.
    public var arguments: [String] {
        var smithers = ["smithers", "launch", "--provider", provider, "--account", account]
        if let model { smithers += ["--model", model] }
        if let effort { smithers += ["--effort", effort] }
        let server = host.rex.isEmpty ? [] : ["-S", host.rex]
        if let session = host.session {
            return server + ["window", "new", "-s", session, "-c", host.path, emoji, "--"] + smithers
        }
        return server + ["new", project, "-c", host.path, "--window", emoji, "--"] + smithers
    }
}
