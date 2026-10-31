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

/// The launch sentence (docs/SPEC.md, "Steps"): agent, model, effort where it can be set, billing
/// account, project, host. Everything is filled in from the start.
public struct LaunchSentence: Equatable, Sendable {
    public enum Word: Int, Sendable { case agent, model, effort, account, project, host }
    public struct Option: Equatable, Sendable, Identifiable {
        public var mark = ""
        public var title: String
        public var note = ""
        public var id: String { title }
    }

    /// In the order Viz set. The last two have no accounts yet, so they cannot launch.
    public static let agents = [("codex", "Codex"), ("claude", "Claude"), ("antigravity", "Antigravity"), ("devin", "Devin")]
    public static let joins: [Word: String] = [.model: "with", .effort: "at", .account: "billed to", .project: "in", .host: "on"]

    public let accounts: [LaunchAccount]
    public let projects: [LaunchProject]
    public private(set) var slot = 0
    public private(set) var agent: Int
    public private(set) var account = 0
    public private(set) var model: CatalogModel?
    public private(set) var effort: String?
    public private(set) var project = 0
    public private(set) var host = 0
    public private(set) var query = ""

    /// `accounts` come best first, so the first is what the default rule picked.
    public init(accounts: [LaunchAccount], projects: [LaunchProject]) {
        self.accounts = accounts
        self.projects = projects
        let first = accounts.first?.id.provider ?? "codex"
        agent = Self.agents.firstIndex { $0.0 == first } ?? 0
    }

    var provider: String { Self.agents[agent].0 }
    var billing: [LaunchAccount] { accounts.filter { $0.id.provider == provider } }
    var current: LaunchAccount? { billing.indices.contains(account) ? billing[account] : nil }
    /// The chosen host's answer, or the account's from any host.
    var entry: ModelEntry? { current.flatMap { place?.entries[$0.id] ?? $0.entry } }
    var catalog: [CatalogModel] { entry?.models ?? [] }
    var defaultModel: CatalogModel? {
        guard let e = entry else { return nil }
        return catalog.first { $0.id == e.defaultId } ?? catalog.first { $0.display == e.default }
    }
    var shownModel: CatalogModel? { model ?? defaultModel }
    /// Efforts Burns can set: Codex's stay the home's, so its models list none.
    var efforts: [String] { shownModel?.efforts ?? [] }
    var shownEffort: String { effort ?? entry?.defaultEffort ?? "" }
    var place: LaunchHost? { projects.indices.contains(project) && projects[project].hosts.indices.contains(host) ? projects[project].hosts[host] : nil }

    public var words: [Word] { efforts.isEmpty ? [.agent, .model, .account, .project, .host] : [.agent, .model, .effort, .account, .project, .host] }
    public var word: Word { words[min(slot, words.count - 1)] }
    public var onLast: Bool { slot >= words.count - 1 }

    public func text(_ w: Word) -> String {
        switch w {
        case .agent: return Self.agents[agent].1
        case .model:
            let name = shownModel?.display ?? entry?.default ?? "Own default"
            let fixed = efforts.isEmpty ? shownEffort : ""
            return fixed.isEmpty ? (name.isEmpty ? "Own default" : name) : "\(name) \(fixed)"
        case .effort: return shownEffort.isEmpty ? "default" : shownEffort
        case .account: return current.map { "\($0.emoji) \($0.id.name)" } ?? "no account"
        case .project: return projects.indices.contains(project) ? projects[project].name : "no project"
        case .host: return place?.shown ?? "no host"
        }
    }

    public var options: [Option] {
        let all: [Option]
        switch word {
        case .agent: all = Self.agents.map { a in
            Option(title: a.1, note: accounts.first { $0.id.provider == a.0 }.map { "\($0.id.name) \($0.left)" } ?? "not in Burns yet") }
        case .model: all = catalog.map { Option(title: $0.display, note: $0 == defaultModel ? "Default" : "") }
        case .effort: all = efforts.map { Option(title: $0, note: $0 == entry?.defaultEffort ? "Default" : "") }
        case .account: all = billing.map { Option(mark: $0.emoji, title: $0.id.name, note: $0.left) }
        case .project: all = projects.map { Option(title: $0.name, note: $0.path) }
        case .host: all = projects.indices.contains(project) ? projects[project].hosts.map {
            Option(title: $0.shown, note: $0.session == nil ? "new session" : "open session") } : []
        }
        let q = query.lowercased()
        return q.isEmpty ? all : all.filter { "\($0.title) \($0.note)".lowercased().contains(q) }
    }

    public var selected: String {
        switch word {
        case .agent: return Self.agents[agent].1
        case .model: return shownModel?.display ?? ""
        case .effort: return shownEffort
        case .account: return current?.id.name ?? ""
        case .project: return projects.indices.contains(project) ? projects[project].name : ""
        case .host: return place?.shown ?? ""
        }
    }

    public mutating func pick(_ title: String) {
        switch word {
        case .agent:
            if let i = Self.agents.firstIndex(where: { $0.1 == title }), i != agent { agent = i; account = 0; model = nil; effort = nil }
        case .model:
            model = catalog.first { $0.display == title }
            if model == defaultModel { model = nil }
            if let e = effort, !efforts.contains(e) { effort = nil }
        case .effort: effort = title == entry?.defaultEffort ? nil : title
        case .account:
            if let i = billing.firstIndex(where: { $0.id.name == title }) {
                account = i
                if let m = model, !catalog.contains(m) { model = nil } // an untouched model follows the account
            }
        case .project:
            if let i = projects.firstIndex(where: { $0.name == title }) { project = i; host = 0 }
        case .host:
            if projects.indices.contains(project), let i = projects[project].hosts.firstIndex(where: { $0.shown == title }) {
                host = i
                if let m = model, !catalog.contains(m) { model = nil }
            }
        }
    }

    public mutating func move(_ by: Int) {
        let list = options
        guard !list.isEmpty else { return }
        let at = list.firstIndex { $0.title == selected } ?? -1
        pick(list[max(0, min(list.count - 1, at + by))].title)
    }

    /// Left and right stop at the ends.
    public mutating func step(_ by: Int) { slot = max(0, min(words.count - 1, slot + by)); query = "" }
    /// Tab wraps.
    public mutating func cycle(_ by: Int) { slot = (slot + by + words.count) % words.count; query = "" }
    public mutating func focus(_ w: Word) { if let i = words.firstIndex(of: w) { slot = i; query = "" } }

    public mutating func type(_ text: String) {
        query += text.lowercased()
        if let first = options.first { pick(first.title) }
    }

    public mutating func erase() { if !query.isEmpty { query.removeLast() } }

    /// Why this sentence cannot launch, or nil when it can.
    public var problem: String? {
        if current == nil { return "\(Self.agents[agent].1) has no account in Burns yet." }
        if place == nil { return "No project: no Rex session has a folder yet." }
        return nil
    }

    /// Only what differs from the account's defaults becomes a flag.
    public var request: LaunchRequest? {
        guard problem == nil, let current, let place else { return nil }
        return LaunchRequest(host: place, project: projects[project].name, provider: provider, account: current.id.name,
                             model: model?.id, effort: efforts.isEmpty ? nil : effort, emoji: current.emoji)
    }
}
