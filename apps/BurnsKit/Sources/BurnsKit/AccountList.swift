import CryptoKit
import Foundation

/// One entry of the central list, spelled as the accounts file spells it.
public struct CentralAccount: Codable, Equatable, Sendable, Identifiable {
    public var provider: String
    public var name: String
    public var home: String
    public var extraHomes: [String]
    public var kind: String
    public var colour: String?
    public var emoji: String?
    /// Every field the app does not edit, such as `launch`, kept as read. The app writes the whole
    /// file, so a field it dropped here would be deleted from the host.
    public var extra: [String: JSONValue]

    public var id: AccountID { AccountID(provider: provider, name: name) }

    public init(provider: String, name: String, home: String, extraHomes: [String] = [], kind: String, colour: String? = nil, emoji: String? = nil,
                extra: [String: JSONValue] = [:]) {
        self.provider = provider; self.name = name; self.home = home; self.extraHomes = extraHomes
        self.kind = kind; self.colour = colour; self.emoji = emoji; self.extra = extra
    }

    enum CodingKeys: String, CodingKey, CaseIterable {
        case provider, name, home, kind, colour, emoji
        case extraHomes = "extra_homes"
    }

    /// A missing list and an empty one are the same account.
    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        provider = try c.decode(String.self, forKey: .provider)
        name = try c.decode(String.self, forKey: .name)
        home = try c.decode(String.self, forKey: .home)
        extraHomes = try c.decodeIfPresent([String].self, forKey: .extraHomes) ?? []
        kind = try c.decode(String.self, forKey: .kind)
        colour = try c.decodeIfPresent(String.self, forKey: .colour)
        emoji = try c.decodeIfPresent(String.self, forKey: .emoji)
        let all = try decoder.container(keyedBy: FieldName.self)
        let known = Set(CodingKeys.allCases.map(\.rawValue))
        extra = try all.allKeys.filter { !known.contains($0.stringValue) }.reduce(into: [:]) { out, key in
            out[key.stringValue] = try all.decode(JSONValue.self, forKey: key)
        }
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(provider, forKey: .provider)
        try c.encode(name, forKey: .name)
        try c.encode(home, forKey: .home)
        if !extraHomes.isEmpty { try c.encode(extraHomes, forKey: .extraHomes) }
        try c.encode(kind, forKey: .kind)
        try c.encodeIfPresent(colour, forKey: .colour)
        try c.encodeIfPresent(emoji, forKey: .emoji)
        var rest = encoder.container(keyedBy: FieldName.self)
        for (name, value) in extra { try rest.encode(value, forKey: FieldName(name)) }
    }

    /// The account in a review: its home, and the names of its other fields. Never their values.
    public var detail: String { ([home] + extra.keys.sorted()).joined(separator: ", ") }
}

/// Any JSON value, kept exactly so that it can be written back.
public enum JSONValue: Codable, Equatable, Sendable {
    case null, bool(Bool), number(Double), string(String), array([JSONValue]), object([String: JSONValue])

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null }
        else if let v = try? c.decode(Bool.self) { self = .bool(v) }
        else if let v = try? c.decode(Double.self) { self = .number(v) }
        else if let v = try? c.decode(String.self) { self = .string(v) }
        else if let v = try? c.decode([JSONValue].self) { self = .array(v) }
        else { self = .object(try c.decode([String: JSONValue].self)) }
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .null: try c.encodeNil()
        case .bool(let v): try c.encode(v)
        case .number(let v): try c.encode(v)
        case .string(let v): try c.encode(v)
        case .array(let v): try c.encode(v)
        case .object(let v): try c.encode(v)
        }
    }
}

/// A key not known until the file is read.
private struct FieldName: CodingKey {
    let stringValue: String
    let intValue: Int? = nil
    init(_ name: String) { stringValue = name }
    init?(stringValue: String) { self.init(stringValue) }
    init?(intValue: Int) { nil }
}

/// The accounts file's contents.
public struct AccountsFile: Codable, Equatable, Sendable {
    public var accounts: [CentralAccount]
    public init(accounts: [CentralAccount]) { self.accounts = accounts }

    public func encoded() -> Data {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
        return ((try? encoder.encode(self)) ?? Data()) + Data("\n".utf8)
    }
}

/// What the central list needs of a store: `NSUbiquitousKeyValueStore` in the apps.
public protocol KeyValues: AnyObject, Sendable {
    func data(forKey key: String) -> Data?
    func set(_ value: Data?, forKey key: String)
    func removeObject(forKey key: String)
}

extension NSUbiquitousKeyValueStore: KeyValues, @unchecked @retroactive Sendable {}

/// The central account list (docs/SPEC.md, "Accounts in the apps"): one key per account plus one
/// for the order, so two devices editing different accounts do not overwrite each other. iCloud's
/// store takes keys of at most 64 bytes, so a host's key is a hash of its address.
///
/// iCloud delivers keys one at a time. `digest` hashes the whole list as saved, so a device that has
/// only some of a save can tell, and waits rather than copy a part or an older list to the hosts.
public final class CentralStore: Sendable {
    static let orderKey = "order"
    static let digestKey = "digest"
    let values: any KeyValues

    public init(values: any KeyValues) { self.values = values }

    static func key(_ id: AccountID) -> String { "account.\(id.provider).\(id.name)" }

    static func key(host: String) -> String {
        "written." + SHA256.hash(data: Data(host.utf8)).prefix(16).map { String(format: "%02x", $0) }.joined()
    }

    private var order: [AccountID]? {
        values.data(forKey: Self.orderKey).flatMap { try? JSONDecoder().decode([AccountID].self, from: $0) }
    }

    /// What iCloud holds now.
    public enum Stored: Equatable, Sendable {
        /// Nothing, which is what first use means.
        case never
        /// Part of a save: an account's key is missing, or the list does not match the digest.
        case arriving
        case ready([CentralAccount])
    }

    private func account(_ id: AccountID) -> CentralAccount? {
        values.data(forKey: Self.key(id)).flatMap { try? JSONDecoder().decode(CentralAccount.self, from: $0) }
    }

    /// Every listed account, or nil when one has not arrived.
    private var whole: [CentralAccount]? {
        guard let ids = order else { return nil }
        let list = ids.compactMap(account)
        return list.count == ids.count ? list : nil
    }

    private static func digest(_ accounts: [CentralAccount]) -> Data {
        Data(SHA256.hash(data: AccountsFile(accounts: accounts).encoded()))
    }

    public func stored() -> Stored {
        guard order != nil else { return .never }
        guard let list = whole else { return .arriving }
        // A store saved before the digest existed has none.
        if let digest = values.data(forKey: Self.digestKey), digest != Self.digest(list) { return .arriving }
        return .ready(list)
    }

    /// The list when iCloud holds all of it, and nil otherwise.
    public func load() -> [CentralAccount]? {
        if case .ready(let list) = stored() { return list }
        return nil
    }

    /// Takes the accounts as they are once iCloud has gone quiet: two devices that edit different
    /// accounts leave a digest that neither list matches. A missing key stays missing: that account
    /// could be real, so only an edit replaces it.
    public func settle() -> [CentralAccount]? {
        guard let list = whole else { return nil }
        values.set(Self.digest(list), forKey: Self.digestKey)
        return list
    }

    public func save(_ accounts: [CentralAccount]) {
        let ids = accounts.map(\.id)
        for gone in (order ?? []) where !ids.contains(gone) { values.removeObject(forKey: Self.key(gone)) }
        for account in accounts { values.set(try? JSONEncoder().encode(account), forKey: Self.key(account.id)) }
        values.set(try? JSONEncoder().encode(ids), forKey: Self.orderKey)
        values.set(Self.digest(accounts), forKey: Self.digestKey)
    }

    /// The hash of the file last seen in step with the list, shared so that one device's write does
    /// not read as a hand edit on another.
    public func written(host: String) -> String? {
        values.data(forKey: Self.key(host: host)).flatMap { String(data: $0, encoding: .utf8) }
    }

    public func setWritten(_ hash: String, host: String) {
        values.set(Data(hash.utf8), forKey: Self.key(host: host))
    }
}
