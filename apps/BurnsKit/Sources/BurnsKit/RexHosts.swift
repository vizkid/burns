import Foundation

/// `rex hosts list --json`: the one parser, and the one rule for matching a host to a Rex entry.
public struct RexHosts: Decodable, Equatable, Sendable {
    public struct Entry: Decodable, Equatable, Sendable {
        public var label: String
        public var icon: String?
        public init(label: String, icon: String? = nil) { self.label = label; self.icon = icon }
    }

    public var entries: [Entry]
    public init(entries: [Entry]) { self.entries = entries }

    private struct Row: Decodable { let host: Entry }
    private enum Keys: String, CodingKey { case hosts }

    public init(from decoder: Decoder) throws {
        entries = try decoder.container(keyedBy: Keys.self).decode([Row].self, forKey: .hosts).map(\.host)
    }

    /// `Host.tailnet.ts.net` and `host` are one machine.
    public static func short(_ name: String) -> String {
        (name.split(separator: ".").first.map(String.init) ?? name).lowercased()
    }

    /// One host can have several entries, such as a label and its tailnet name.
    private func matching(_ host: String) -> [Entry] {
        entries.filter { Self.short($0.label) == Self.short(host) }
    }

    /// The label for `rex -S`. The host's plain label comes before a tailnet name for it.
    public func server(for host: String) -> String? {
        let all = matching(host)
        return (all.first { $0.label.lowercased() == host.lowercased() } ?? all.first)?.label
    }

    /// The icon picked for a host. A pick lands on only the entry it was made on, so any counts.
    public func icon(for host: String) -> String? { matching(host).compactMap(\.icon).first }

    /// Where a launch on a host goes. Only this machine runs without `-S`: anything else that Rex
    /// does not list must not fall through to this Mac's server under another host's name.
    public enum Place: Equatable, Sendable {
        case here
        case server(String)
        case unreachable(String)
    }

    public static func place(host: String, here: String, rex: RexHosts?) -> Place {
        if short(host) == short(here) { return .here }
        guard let rex else { return .unreachable("Rex did not answer") }
        return rex.server(for: host).map(Place.server) ?? .unreachable("no Rex entry")
    }

    /// This machine's name, as `smithersd` reads it. A system call, so no name lookup can block it.
    public static var thisMachine: String {
        var name = [CChar](repeating: 0, count: 256)
        guard gethostname(&name, name.count) == 0 else { return "" }
        return short(String(decoding: name.prefix { $0 != 0 }.map { UInt8(bitPattern: $0) }, as: UTF8.self))
    }
}
