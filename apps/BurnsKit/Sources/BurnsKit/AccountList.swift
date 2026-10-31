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

    public var id: AccountID { AccountID(provider: provider, name: name) }

    public init(provider: String, name: String, home: String, extraHomes: [String] = [], kind: String, colour: String? = nil, emoji: String? = nil) {
        self.provider = provider; self.name = name; self.home = home; self.extraHomes = extraHomes
        self.kind = kind; self.colour = colour; self.emoji = emoji
    }

    enum CodingKeys: String, CodingKey {
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
    }
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
public final class CentralStore: Sendable {
    static let orderKey = "order"
    let values: any KeyValues

    public init(values: any KeyValues) { self.values = values }

    static func key(_ id: AccountID) -> String { "account.\(id.provider).\(id.name)" }

    static func key(host: String) -> String {
        "written." + SHA256.hash(data: Data(host.utf8)).prefix(16).map { String(format: "%02x", $0) }.joined()
    }

    private var order: [AccountID]? {
        values.data(forKey: Self.orderKey).flatMap { try? JSONDecoder().decode([AccountID].self, from: $0) }
    }

    /// Nil until the list has been stored once, which is what first use means.
    public func load() -> [CentralAccount]? {
        order?.compactMap { id in
            values.data(forKey: Self.key(id)).flatMap { try? JSONDecoder().decode(CentralAccount.self, from: $0) }
        }
    }

    public func save(_ accounts: [CentralAccount]) {
        let ids = accounts.map(\.id)
        for gone in (order ?? []) where !ids.contains(gone) { values.removeObject(forKey: Self.key(gone)) }
        for account in accounts { values.set(try? JSONEncoder().encode(account), forKey: Self.key(account.id)) }
        values.set(try? JSONEncoder().encode(ids), forKey: Self.orderKey)
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
