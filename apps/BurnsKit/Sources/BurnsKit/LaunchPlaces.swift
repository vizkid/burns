import Foundation

/// The pane the beside key was pressed in (docs/SPEC.md, "Beside, typed folders and new sessions").
public struct LaunchPane: Equatable, Sendable {
    public var rex: String
    public var shown: String
    public var session: String
    public var block: String
    /// The pane's folder on its host, and relative to that host's home folder.
    public var path: String
    public var relative: String
    public init(rex: String, shown: String, session: String, block: String, path: String, relative: String) {
        self.rex = rex; self.shown = shown; self.session = session; self.block = block; self.path = path; self.relative = relative
    }
}

/// smithersd's `GET /dirs`.
public struct DirsAnswer: Codable, Equatable, Sendable {
    public var host: String
    public var home: String
    public var dirs: [String]
}

extension LaunchProject {
    /// `/Users/<name>/...` and `/home/<name>/...` are the same place on each host. Nil outside a
    /// home folder, and empty for the home folder itself.
    public static func relative(_ path: String) -> String? {
        let parts = path.split(separator: "/").map(String.init)
        guard path.hasPrefix("/"), parts.count >= 2, parts[0] == "Users" || parts[0] == "home" else { return nil }
        return parts.dropFirst(2).joined(separator: "/")
    }

    /// Each project once, in first-seen order, with every row any copy of it has.
    static func merged(_ lists: [LaunchProject]...) -> [LaunchProject] {
        var out: [LaunchProject] = []
        for p in lists.joined() {
            guard let i = out.firstIndex(where: { $0.path == p.path }) else { out.append(p); continue }
            for h in p.hosts where !out[i].hosts.contains(where: { $0.id == h.id }) { out[i].hosts.append(h) }
        }
        return out
    }
}

extension LaunchSentence {
    /// Every project: the reads', the pane's, folders the operator chose, and the folders the
    /// hosts found for what is typed now.
    public var projects: [LaunchProject] {
        let own = pane.map { [LaunchProject(name: Self.name($0.relative), path: $0.relative, hosts: [])] } ?? []
        return LaunchProject.merged(base, own, kept, found)
    }

    var chosenProject: LaunchProject? { projects.first { $0.path == projectPath } ?? projects.first }

    /// A project's rows: beside the pane first, when the pane's host has the folder, then its
    /// sessions, the sessions the operator named, and, while a name is typed, that new session.
    func rows(_ p: LaunchProject, typed: String = "") -> [LaunchHost] {
        var out: [LaunchHost] = []
        if let pane, let path = p.path == pane.relative ? pane.path : p.hosts.first(where: { $0.rex == pane.rex })?.path {
            var beside = LaunchHost(rex: pane.rex, shown: pane.shown, path: path, session: pane.session)
            beside.block = pane.block
            out.append(beside)
        }
        out += p.hosts + (made[p.path] ?? [])
        guard !typed.isEmpty else { return out }
        var hosts = Set<String>()
        for h in out where h.block == nil && hosts.insert(h.rex).inserted
            && !out.contains(where: { $0.rex == h.rex && ($0.session == typed || $0.create == typed) }) {
            var make = LaunchHost(rex: h.rex, shown: h.shown, path: h.path, session: nil, entries: h.entries)
            make.create = typed
            out.append(make)
        }
        return out
    }

    /// A typed session is kept in `made` when chosen, so the place needs no typed rows. It must not
    /// read `word`: the words depend on the place's efforts.
    var place: LaunchHost? {
        guard let p = chosenProject else { return nil }
        let all = rows(p)
        return all.first { $0.id == hostID } ?? all.first
    }

    /// The folder `GET /dirs` should complete, relative to the home folder, while one is typed in
    /// the project word. `~/` and a home prefix are stripped; other absolute paths are not offered.
    public var completing: String? {
        guard word == .project, !query.isEmpty else { return nil }
        if query.hasPrefix("~/") { return String(query.dropFirst(2)) }
        if query.hasPrefix("/") { return LaunchProject.relative(query) }
        return query
    }

    /// Takes the hosts' folders for `typed`. An answer for an older query is dropped.
    public mutating func found(_ folders: [LaunchProject], for typed: String) {
        guard typed == completing else { return }
        found = folders
        if !options.contains(where: { $0.id == selected }), let first = options.first { pick(first.id) }
    }

    /// Tab in the project word: the query becomes the selected folder and `/`. False elsewhere.
    public mutating func complete() -> Bool {
        guard word == .project, !query.isEmpty, let p = chosenProject else { return false }
        query = p.path + "/"
        return true
    }

    /// Chooses a project or a host row by `id`. A typed folder or session stays once chosen.
    mutating func choosePlace(_ id: String) {
        if word == .project, let p = projects.first(where: { $0.path == id }) {
            if found.contains(where: { $0.path == id }) { kept = LaunchProject.merged(kept, [p]) }
            projectPath = id
            hostID = nil
            touched.remove(.host)
        } else if word == .host, let p = chosenProject, let h = rows(p, typed: query).first(where: { $0.id == id }) {
            if h.create != nil, !(made[p.path] ?? []).contains(h) { made[p.path, default: []].append(h) }
            hostID = id
        }
    }

    /// The target counts as the operator's choice once it lands. A project or host the operator
    /// picked first wins, and drops the target.
    mutating func land() {
        guard let t = target else { return }
        guard touched.isDisjoint(with: [.project, .host]) else { target = nil; return }
        // The pane's own folder first: its other folders on that host can go beside it too.
        let own = projects.filter { $0.path == pane?.relative }
        for p in own + projects {
            guard let h = rows(p).first(where: t.matches) else { continue }
            projectPath = p.path
            hostID = h.id
            touched.formUnion([.project, .host])
            target = nil
            return
        }
    }

    /// A touched place is kept by identity, so a read that reorders the lists does not move it.
    mutating func pin() {
        if touched.contains(.project) { projectPath = chosenProject?.path }
        if touched.contains(.host) { hostID = place?.id }
    }

    static func name(_ path: String) -> String { path.split(separator: "/").last.map(String.init) ?? path }

    static func note(_ h: LaunchHost) -> String {
        if h.block != nil { return "beside this pane" }
        if let c = h.create { return "new session \(c)" }
        return h.session.map { "session \($0)" } ?? "new session"
    }
}
