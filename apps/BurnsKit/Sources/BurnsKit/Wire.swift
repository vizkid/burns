import Foundation

/// smithersd's answers (docs/SPEC.md, "Wire protocol"). Field names follow the JSON with
/// snake_case converted.
public struct UsageAnswer: Codable, Equatable, Sendable {
    public var host: String
    public var observedAt: Date
    public var accounts: [WireAccount]
}

public struct WireAccount: Codable, Equatable, Sendable {
    public var provider: String
    public var account: String
    public var kind: String
    public var limits: [WireLimit]
    /// Whether the account's home is on this host. Missing from an older `smithersd`.
    public var setUp: Bool?
}

public struct WireLimit: Codable, Equatable, Sendable {
    public var limitId: String
    /// Keyed by window length in minutes, as text.
    public var windows: [String: WireWindow]
}

public struct WireWindow: Codable, Equatable, Sendable {
    /// Nil when smithersd already knows the window is unknown.
    public var percent: Double?
    public var resetsAt: Date
    public var observedAt: Date
}

public struct ReadingsAnswer: Codable, Equatable, Sendable {
    public var host: String
    public var readings: [Reading]
}

/// One change of a quota window (docs/SPEC.md, "Reading history").
public struct Reading: Codable, Equatable, Hashable, Sendable {
    public var provider: String
    public var account: String
    public var limitId: String
    public var minutes: Int
    public var percent: Double
    public var resetsAt: Date
    public var observedAt: Date

    public init(provider: String, account: String, limitId: String, minutes: Int, percent: Double, resetsAt: Date, observedAt: Date) {
        self.provider = provider; self.account = account; self.limitId = limitId; self.minutes = minutes
        self.percent = percent; self.resetsAt = resetsAt; self.observedAt = observedAt
    }
}

public enum Wire {
    /// Decodes smithersd's JSON: snake_case keys, RFC 3339 times with any number of fractional
    /// digits and any offset (Go writes up to nine digits; ISO8601DateFormatter takes three).
    public static var decoder: JSONDecoder {
        let d = JSONDecoder()
        d.keyDecodingStrategy = .convertFromSnakeCase
        d.dateDecodingStrategy = .custom { decoder in
            let text = try decoder.singleValueContainer().decode(String.self)
            guard let date = parse(text) else {
                throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: "not an RFC 3339 time: \(text)"))
            }
            return date
        }
        return d
    }

    static func parse(_ text: String) -> Date? {
        // Split off the fraction, parse the whole seconds, then add the fraction back.
        var whole = text, fraction = 0.0
        if let dot = text.firstIndex(of: ".") {
            let rest = text[text.index(after: dot)...]
            let digits = rest.prefix { $0.isNumber }
            fraction = Double("0." + digits) ?? 0
            whole = String(text[..<dot]) + rest.dropFirst(digits.count)
        }
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f.date(from: whole).map { $0.addingTimeInterval(fraction) }
    }
}
