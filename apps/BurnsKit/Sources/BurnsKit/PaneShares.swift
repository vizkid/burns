import Foundation

/// `GET /usage/panes`: tokens per open pane and model, without cached input.
public struct PanesAnswer: Codable, Equatable, Sendable {
    public var host: String
    /// Whether Rex confirmed the panes. Missing from an older `smithersd`, which counts as no.
    public var checked: Bool?
    public var panes: [WirePane]
}

public struct WirePane: Codable, Equatable, Sendable {
    public var provider: String
    public var account: String
    public var block: String
    public var model: String
    public var tokens: Int64
    public var last: Date
    /// Tokens from the session's own thread. Missing from an older `smithersd`.
    public var mainTokens: Int64?
    /// Distinct subagents of the whole pane, on every row. Missing from an older `smithersd`.
    public var agents: Int?
    /// The names Rex shows. Missing when Rex did not answer.
    public var session: String?
    public var label: String?
    public init(provider: String, account: String, block: String, model: String, tokens: Int64, last: Date, mainTokens: Int64? = nil, agents: Int? = nil,
                session: String? = nil, label: String? = nil) {
        self.provider = provider; self.account = account; self.block = block; self.model = model; self.tokens = tokens; self.last = last
        self.mainTokens = mainTokens; self.agents = agents; self.session = session; self.label = label
    }
}

/// What Rex says about a pane that exists now.
public struct LivePane: Equatable, Sendable {
    public var session: String
    public var label: String
    public init(session: String, label: String) { self.session = session; self.label = label }
}

/// One host's two answers. Nil means that source did not answer, which is unknown and not empty.
public struct HostPanes: Equatable, Sendable {
    public var host: String
    public var icon: String
    public var rows: [WirePane]?
    public var live: [String: LivePane]?
    public init(host: String, icon: String, rows: [WirePane]?, live: [String: LivePane]?) {
        self.host = host; self.icon = icon; self.rows = rows; self.live = live
    }

    /// A host's answer. Rows Rex did not confirm are unknown, so `live` stays nil.
    public init(host: String, icon: String, answer: PanesAnswer) {
        self.host = host; self.icon = icon; rows = answer.panes
        guard answer.checked == true else { live = nil; return }
        live = Dictionary(answer.panes.map { ($0.block, LivePane(session: $0.session ?? "", label: $0.label ?? "")) }) { first, _ in first }
    }
}

public struct PaneShare: Equatable, Sendable, Identifiable {
    public var id: String
    public var title: String
    public var host: String
    public var icon: String
    public var model: String
    public var tokens: Int64
    public var share: Double
    public var agents: Int
    /// What the subagents used, largest first, or nil when the pane has none.
    public var subagentModels: String?

    /// The model, then the subagents, so a pane on an expensive model that delegates reads as both.
    public var modelLine: String {
        agents > 0 ? "\(model) +\(agents) agent\(agents == 1 ? "" : "s")" : model
    }
}

/// The selected account's live panes (docs/SPEC.md, "Live pane shares"). `unread` names hosts
/// whose panes could not be read, so a view never shows their share as nothing.
public struct PaneShares: Equatable, Sendable {
    public var panes: [PaneShare]
    public var unread: [String]

    public static func build(_ hosts: [HostPanes], account: AccountID) -> PaneShares {
        var panes: [PaneShare] = []
        var unread: [String] = []
        for h in hosts {
            guard let rows = h.rows, let live = h.live else { unread.append(h.host); continue }
            let mine = rows.filter { $0.provider == account.provider && $0.account == account.name }
            for (block, models) in Dictionary(grouping: mine, by: \.block) {
                                // A pane appears once, so its share is whole, subagents included. The model its own
                // thread runs names it: subagents on a cheap model can outweigh that thread.
                let rank = { (m: WirePane) in (m.mainTokens ?? 0, m.tokens) }
                guard let pane = live[block], let main = models.max(by: { rank($0) < rank($1) }) else { continue }
                panes.append(PaneShare(id: h.host + block, title: title(pane), host: h.host, icon: h.icon, model: modelName(main.model),
                                       tokens: models.reduce(0) { $0 + $1.tokens }, share: 0,
                                       agents: models.compactMap(\.agents).max() ?? 0, subagentModels: subagents(models)))
            }
        }
        let total = panes.reduce(Int64(0)) { $0 + $1.tokens }
        for i in panes.indices where total > 0 { panes[i].share = Double(panes[i].tokens) / Double(total) }
        return PaneShares(panes: panes.sorted { ($1.tokens, $0.id) < ($0.tokens, $1.id) }, unread: unread)
    }

    /// A row's subagent tokens are what its own thread did not use. Nil on an older `smithersd`.
    static func subagents(_ models: [WirePane]) -> String? {
        let parts = models.compactMap { m -> (String, Int64)? in
            guard let main = m.mainTokens, m.tokens > main else { return nil }
            return (modelName(m.model), m.tokens - main)
        }.sorted { $0.1 > $1.1 }
        return parts.isEmpty ? nil : "Subagents: " + parts.map { "\($0.0) \(tokens($0.1))" }.joined(separator: ", ")
    }

    static func tokens(_ n: Int64) -> String {
        n >= 1_000_000 ? String(format: "%.1fM", Double(n) / 1e6) : n >= 1000 ? "\(n / 1000)K" : "\(n)"
    }

    static func title(_ p: LivePane) -> String {
        p.label.isEmpty || p.label == "shell" ? p.session : "\(p.session) · \(p.label)"
    }

    /// `claude-opus-5-5` reads Opus 5.5 and `gpt-6.1-sol` reads Sol 6.1, so the family leads as it does
    /// on both providers. Any other shape shows its id: a wrong guess is worse than a raw name.
    public static func modelName(_ id: String) -> String {
        var parts = id.split(separator: "-").map(String.init)
        if parts.count > 3, parts.last?.count == 8, Int(parts.last ?? "") != nil { parts.removeLast() } // A dated snapshot.
        if parts.count >= 3, parts[0] == "claude" { return parts[1].capitalized + " " + parts[2...].joined(separator: ".") }
        if parts.count == 3, parts[0] == "gpt" { return parts[2].capitalized + " " + parts[1] }
        return id
    }

    /// One hue stepped by rank: separate hues fail the colour-blind check on the window's dark
    /// surface. Nil after the fifth pane, which draws grey.
    public static func strength(rank: Int) -> Double? {
        let steps = [1, 0.74, 0.54, 0.38, 0.26]
        return rank < steps.count ? steps[rank] : nil
    }

    /// Segment widths that fill `room`. Each keeps `minimum`, and the rest divides by share, so a
    /// tiny pane stays visible and the bar never runs past its edge.
    public static func widths(_ shares: [Double], room: Double, minimum: Double) -> [Double] {
        // A first layout can have no width, and a frame refuses a negative one.
        let room = max(0, room), spare = room - minimum * Double(shares.count)
        guard spare > 0 else { return shares.map { _ in room / Double(max(shares.count, 1)) } }
        return shares.map { minimum + spare * $0 }
    }

    public static func percent(_ share: Double) -> String {
        share > 0 && share < 0.01 ? "<1%" : "\(Int((share * 100).rounded()))%"
    }
}
