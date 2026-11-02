import Foundation

/// The launch sentence (docs/SPEC.md, "Steps"): agent, model, effort where it can be set, billing
/// account, project, host. Everything is filled in from the start.
public struct LaunchSentence: Equatable, Sendable {
    public enum Word: Int, Sendable { case agent, model, effort, account, project, host }
    public struct Option: Equatable, Sendable, Identifiable {
        /// What this option is: a model's id, a project's folder, a host's Rex label. Titles can repeat.
        public var id: String
        public var mark = ""
        public var title: String
        public var note = ""
    }

    /// In the order Viz set. The last two have no accounts yet, so they cannot launch.
    public static let agents = [("codex", "Codex"), ("claude", "Claude"), ("antigravity", "Antigravity"), ("devin", "Devin")]
    public static let joins: [Word: String] = [.model: "with", .effort: "at", .account: "billed to", .project: "in", .host: "on"]

    public private(set) var accounts: [LaunchAccount]
    public private(set) var projects: [LaunchProject]
    public private(set) var slot = 0
    public private(set) var agent: Int
    public private(set) var account = 0
    public private(set) var model: CatalogModel?
    public private(set) var effort: String?
    public private(set) var project = 0
    public private(set) var host = 0
    public private(set) var query = ""
    /// Why the first account is the default.
    public private(set) var reason: String
    /// Hosts left out of the lists, each with why: "two: no Rex entry".
    public private(set) var unreachable: [String]
    /// Words the operator chose or confirmed. A newer read keeps these and moves all others.
    private var touched: Set<Word> = []

    /// `accounts` come best first, so the first is what the default rule picked.
    public init(accounts: [LaunchAccount], projects: [LaunchProject], reason: String = "", unreachable: [String] = []) {
        self.accounts = accounts
        self.projects = projects
        self.reason = reason
        self.unreachable = unreachable
        let first = accounts.first?.id.provider ?? "codex"
        agent = Self.agents.firstIndex { $0.0 == first } ?? 0
    }

    /// Takes a newer read while the panel is open. A choice the operator made is matched by identity,
    /// because a read can reorder the lists. Anything untouched takes the new read's default: the
    /// read the panel opened from can be hours old, and Return must launch the best account now.
    public mutating func adopt(_ fresh: LaunchSentence) {
        let shown = word, chosen = current?.id, at = place?.rex
        let folder = projects.indices.contains(project) ? projects[project].path : nil
        accounts = fresh.accounts
        projects = fresh.projects
        reason = fresh.reason
        unreachable = fresh.unreachable
        if !touched.contains(.agent) { agent = fresh.agent }
        account = touched.contains(.account) ? billing.firstIndex { $0.id == chosen } ?? 0 : 0
        project = touched.contains(.project) ? projects.firstIndex { $0.path == folder } ?? 0 : 0
        host = touched.contains(.host) && projects.indices.contains(project) ? projects[project].hosts.firstIndex { $0.rex == at } ?? 0 : 0
        revalidate()
        slot = words.firstIndex(of: shown) ?? min(slot, words.count - 1)
    }

    /// A choice also holds what it was chosen from: a model comes from an account's catalog, and a
    /// host from a project's list.
    private func holds(_ w: Word) -> Set<Word> {
        switch w {
        case .agent: [.agent]
        case .account: [.agent, .account]
        case .model, .effort: [.agent, .account, .model]
        case .project: [.project]
        case .host: [.project, .host]
        }
    }

    /// A model or effort the current account and host do not list would fail in the new tab, so
    /// it never reaches a request.
    private mutating func revalidate() {
        if let m = model { model = catalog.first { $0.id == m.id } }
        if let e = effort, !efforts.contains(e) { effort = nil }
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
            Option(id: a.1, title: a.1, note: accounts.first { $0.id.provider == a.0 }.map { "\($0.id.name) \($0.left)" } ?? "not in Burns yet") }
        case .model: all = catalog.map { Option(id: $0.id, title: $0.display, note: $0 == defaultModel ? "Default" : "") }
        case .effort: all = efforts.map { Option(id: $0, title: $0, note: $0 == entry?.defaultEffort ? "Default" : "") }
        case .account: all = billing.map { Option(id: $0.id.name, mark: $0.emoji, title: $0.id.name, note: $0.left) }
        case .project: all = projects.map { Option(id: $0.path, title: $0.name, note: $0.path) }
        case .host: all = projects.indices.contains(project) ? projects[project].hosts.map {
            Option(id: $0.rex, title: $0.shown, note: $0.session == nil ? "new session" : "open session") } : []
        }
        let q = query.lowercased()
        return q.isEmpty ? all : all.filter { "\($0.title) \($0.note)".lowercased().contains(q) }
    }

    /// The selected option's `id`.
    public var selected: String {
        switch word {
        case .agent: return Self.agents[agent].1
        case .model: return shownModel?.id ?? ""
        case .effort: return shownEffort
        case .account: return current?.id.name ?? ""
        case .project: return projects.indices.contains(project) ? projects[project].path : ""
        case .host: return place?.rex ?? ""
        }
    }

    /// Chooses the option with this `id`.
    public mutating func pick(_ id: String) {
        switch word {
        case .agent:
            if let i = Self.agents.firstIndex(where: { $0.1 == id }), i != agent {
                agent = i; account = 0; model = nil; effort = nil
                touched.subtract([.account, .model])
            }
        case .model:
            model = catalog.first { $0.id == id }
            if model == defaultModel { model = nil }
        case .effort: effort = id == entry?.defaultEffort ? nil : id
        case .account:
            if let i = billing.firstIndex(where: { $0.id.name == id }) { account = i } // an untouched model follows the account
        case .project:
            if let i = projects.firstIndex(where: { $0.path == id }) { project = i; host = 0; touched.remove(.host) }
        case .host:
            if projects.indices.contains(project), let i = projects[project].hosts.firstIndex(where: { $0.rex == id }) { host = i }
        }
        touched.formUnion(holds(word))
        revalidate()
    }

    public mutating func move(_ by: Int) {
        let list = options
        guard !list.isEmpty else { return }
        let at = list.firstIndex { $0.id == selected } ?? -1
        pick(list[max(0, min(list.count - 1, at + by))].id)
    }

    /// Return on a word: the operator saw it and took it, so a newer read must not change it.
    public mutating func confirm() {
        touched.formUnion(holds(word))
        step(1)
    }

    /// Esc steps back. False at the first word, where it closes the panel.
    public mutating func back() -> Bool {
        guard slot > 0 else { return false }
        step(-1)
        return true
    }

    /// Left and right stop at the ends.
    public mutating func step(_ by: Int) { slot = max(0, min(words.count - 1, slot + by)); query = "" }
    /// Tab wraps.
    public mutating func cycle(_ by: Int) { slot = (slot + by + words.count) % words.count; query = "" }
    public mutating func focus(_ w: Word) { if let i = words.firstIndex(of: w) { slot = i; query = "" } }

    public mutating func type(_ text: String) {
        query += text.lowercased()
        if let first = options.first { pick(first.id) }
    }

    public mutating func erase() { if !query.isEmpty { query.removeLast() } }

    /// Why this sentence cannot launch, or nil when it can.
    public var problem: String? {
        if current == nil { return "\(Self.agents[agent].1) has no account in Burns yet." }
        if place == nil { return "No project: no Rex session has a folder yet." }
        return nil
    }

    /// The panel's line: why the default won, while it is still the default, and who is missing.
    public var note: String {
        let why = current?.id == accounts.first?.id ? reason : ""
        let missing = unreachable.isEmpty ? "" : "Not offered: " + unreachable.joined(separator: ", ")
        return [why, missing].filter { !$0.isEmpty }.joined(separator: " · ")
    }

    /// Only what differs from the account's defaults becomes a flag.
    public var request: LaunchRequest? {
        guard problem == nil, let current, let place else { return nil }
        return LaunchRequest(host: place, project: projects[project].name, provider: provider, account: current.id.name,
                             model: model?.id, effort: efforts.isEmpty ? nil : effort, emoji: current.emoji)
    }
}
