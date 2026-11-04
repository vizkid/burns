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
    public private(set) var slot = 0
    public private(set) var agent: Int
    public private(set) var account = 0
    public private(set) var model: CatalogModel?
    public private(set) var effort: String?
    public internal(set) var query = ""
    /// Why the first account is the default.
    public private(set) var reason: String
    /// Hosts left out of the lists, each with why: "two: no Rex entry".
    public private(set) var unreachable: [String]
    /// Words the operator chose or confirmed. A newer read keeps these and moves all others.
    var touched: Set<Word> = []
    /// The session the panel was opened for, until a read lists it.
    var target: LaunchTarget?
    // The places, kept apart so a read replaces only its own (LaunchPlaces.swift). Internal for
    // that file: the operator's choices go through `pick`.
    /// The reads' projects.
    var base: [LaunchProject]
    /// Typed folders the operator chose; they stay after the query clears.
    var kept: [LaunchProject] = []
    /// The hosts' folders for the query typed now.
    var found: [LaunchProject] = []
    /// Typed sessions the operator confirmed, by project path.
    var made: [String: [LaunchHost]] = [:]
    /// The typed session selected while its name is still being typed. It joins `made` only when
    /// the operator leaves the word on it, so Backspace never leaves a longer name behind.
    var pending: Pending?
    struct Pending: Equatable, Sendable {
        var path: String
        var row: LaunchHost
    }
    /// True once a read has arrived, so a target it did not list can be told from one not read yet.
    var hasRead = false
    /// The chosen project and host row, by identity. Nil is the first.
    var projectPath: String?
    var hostID: String?
    /// The pane the beside key was pressed in.
    public var pane: LaunchPane?

    /// `accounts` come best first, so the first is what the default rule picked.
    public init(accounts: [LaunchAccount], projects: [LaunchProject], reason: String = "", unreachable: [String] = []) {
        self.accounts = accounts
        base = projects
        self.reason = reason
        self.unreachable = unreachable
        let first = accounts.first?.id.provider ?? "codex"
        agent = Self.agents.firstIndex { $0.0 == first } ?? 0
    }

    /// Takes a newer read while the panel is open. A choice the operator made is matched by identity,
    /// because a read can reorder the lists. Anything untouched takes the new read's default: the
    /// read the panel opened from can be hours old, and Return must launch the best account now.
    public mutating func adopt(_ fresh: LaunchSentence) {
        let shown = word, chosen = current?.id
        accounts = fresh.accounts
        base = fresh.base
        pane = fresh.pane ?? pane
        hasRead = true
        reason = fresh.reason
        unreachable = fresh.unreachable
        if !touched.contains(.agent) { agent = fresh.agent }
        account = touched.contains(.account) ? billing.firstIndex { $0.id == chosen } ?? 0 : 0
        if !touched.contains(.project) { projectPath = nil }
        if !touched.contains(.host) { hostID = nil }
        land()
        revalidate()
        slot = words.firstIndex(of: shown) ?? min(slot, words.count - 1)
    }

    /// Starts on the session that `burns://new` named. The panel opens before the read that lists
    /// it, so the target waits for a read that does.
    public mutating func aim(_ t: LaunchTarget) {
        target = t
        land()
        revalidate()
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
        case .project: return chosenProject?.name ?? "no project"
        case .host: return place.map { $0.block == nil ? $0.shown : "\($0.shown) beside this pane" } ?? "no host"
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
        case .host:
            let rows = chosenProject.map { rows($0, typed: query) } ?? []
            all = rows.map { Option(id: $0.id, title: $0.shown, note: Self.note($0)) }
        }
        let q = (completing ?? query).lowercased()
        // A typed session's own row matches whatever was typed.
        return q.isEmpty ? all : all.filter { "\($0.title) \($0.note)".lowercased().contains(q) || $0.note == "new session \(query)" }
    }

    /// The selected option's `id`.
    public var selected: String {
        switch word {
        case .agent: return Self.agents[agent].1
        case .model: return shownModel?.id ?? ""
        case .effort: return shownEffort
        case .account: return current?.id.name ?? ""
        case .project: return chosenProject?.path ?? ""
        case .host: return place?.id ?? ""
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
        case .project, .host: choosePlace(id)
        }
        touched.formUnion(holds(word))
        pin()
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
        pin()
        step(1)
    }

    /// Esc steps back. False at the first word, where it closes the panel.
    public mutating func back() -> Bool {
        guard slot > 0 else { return false }
        step(-1)
        return true
    }

    /// Left and right stop at the ends.
    public mutating func step(_ by: Int) { move(to: max(0, min(words.count - 1, slot + by))) }
    /// Tab wraps.
    public mutating func cycle(_ by: Int) { move(to: (slot + by + words.count) % words.count) }
    public mutating func focus(_ w: Word) { if let i = words.firstIndex(of: w) { move(to: i) } }
    private mutating func move(to i: Int) {
        if let p = pending, hostID == p.row.id { made[p.path, default: []].append(p.row) }
        pending = nil
        slot = i; query = ""; found = []
    }

    /// Case stays: paths and session names keep what was typed.
    public mutating func type(_ text: String) {
        query += text
        if let first = options.first { pick(first.id) }
    }

    /// Backspace picks again, as typing does: the selection must be what the query now says.
    public mutating func erase() {
        guard !query.isEmpty else { return }
        query.removeLast()
        guard word == .project || word == .host else { return }
        if !query.isEmpty, let first = options.first {
            pick(first.id)
        } else if query.isEmpty, word == .host, let p = pending, hostID == p.row.id {
            hostID = nil
            pending = nil
        }
    }

    /// Why this sentence cannot launch, or nil when it can.
    public var problem: String? {
        if current == nil { return "\(Self.agents[agent].1) has no account in Burns yet." }
        // The panel opened for a session or pane. Until it lands, Return would launch somewhere else.
        if let t = target {
            let named = t.host.isEmpty ? t.session : "\(t.session) on \(t.host)"
            return hasRead ? "Cannot find \(named). Choose a project or host." : "Finding \(named)…"
        }
        // Typing that matches nothing must not launch the place chosen before it, out of sight.
        if word == .project || word == .host, !query.isEmpty, options.isEmpty { return "Nothing matches “\(query)”." }
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
        return LaunchRequest(host: place, project: chosenProject?.name ?? "", provider: provider, account: current.id.name,
                             model: model?.id, effort: efforts.isEmpty ? nil : effort, emoji: current.emoji)
    }
}
