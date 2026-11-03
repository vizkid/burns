import Foundation
import SwiftUI

/// One hour of an account's work on one model, at list price (`GET /usage/cost`).
public struct CostHour: Codable, Equatable, Sendable {
    public var hour: Date
    public var provider: String
    public var account: String
    public var model: String
    public var requests: Int64
    public var tokens: Int64
    public var micros: Int64
    public var unpriced: Int64
}

/// What a provider says it charged. Never an estimate. 123 with exponent 2 is 1.23.
public struct BilledReading: Codable, Equatable, Sendable {
    public var provider: String
    public var account: String
    public var source: String
    public var currency: String
    public var usedMinor: Int64
    public var exponent: Int
    public var limitMinor: Int64?
    public var enabled: Bool
    public var reason: String?
    public var observedAt: Date
    /// The amount each time it changed, oldest first.
    public var history: [BilledPoint]? = nil
}

public struct BilledPoint: Codable, Equatable, Sendable {
    public var usedMinor: Int64
    public var observedAt: Date
}

public struct CostAnswer: Codable, Equatable, Sendable {
    public var host: String
    public var hours: [CostHour]
    public var billed: [BilledReading]
}

/// One host's answer. A nil answer means it did not answer.
public struct HostCost: Equatable, Sendable {
    public var host: String
    public var answer: CostAnswer?
    public init(host: String, answer: CostAnswer?) { self.host = host; self.answer = answer }
}

/// The selected account's money (docs/SPEC.md, "Proposal: billed spend and list price"). Billed
/// and list price are different numbers and never share a label: only billed is money spent.
public struct CostSummary: Equatable, Sendable {
    public struct Model: Equatable, Sendable, Identifiable {
        public var model: String
        public var micros: Int64
        public var tokens: Int64
        public var unpriced: Int64
        public var id: String { model }
        /// Dollars, or that none are known: part of a model's work can have no price.
        public var amountText: String {
            guard unpriced > 0 else { return CostSummary.dollars(micros) }
            return micros > 0 ? "at least " + CostSummary.plainDollars(micros) : "No price"
        }
    }

    public var billed: BilledReading?
    public var listMicros: Int64
    public var tokens: Int64
    /// List price from the moment the window reached 100%. Nil with no such moment.
    public var sinceLimitMicros: Int64?
    public var models: [Model]
    /// Requests with no known price: a total with any of these is a floor.
    public var unpriced: Int64
    /// Hosts that did not answer, so their work is left out.
    public var unread: [String]
    /// What the billed total rose by inside the window. Zero with no history.
    public var windowBilledMinor: Int64 = 0
    /// Whether any host answered. With none, every list price and token total is unknown.
    public var answered = true
    /// Requests since the limit with no known price.
    public var sinceLimitUnpriced: Int64 = 0

    /// Money was charged in this reset period, so the panel shows whatever the chart says.
    public var hasSpend: Bool { windowBilledMinor > 0 }

    /// The panel's one large number: money charged if there was any, else the list price.
    public func headline(period: String = "This window") -> (label: String, value: String) {
        guard hasSpend, let billed else { return ("\(period), list price", listText) }
        return ("Billed this window", Self.money(windowBilledMinor, billed))
    }

    public var headline: (label: String, value: String) { headline() }

    /// The rise in a month's running total from a window's start. The last amount before the start
    /// is the base. With none, the first amount is, so spend before the history began is left
    /// out. A fall is a new month, which counts from zero.
    static func risen(_ history: [BilledPoint], from start: Date) -> Int64 {
        guard var base = (history.last { $0.observedAt < start } ?? history.first)?.usedMinor else { return 0 }
        var total: Int64 = 0
        for p in history where p.observedAt >= start {
            total += p.usedMinor >= base ? p.usedMinor - base : p.usedMinor
            base = p.usedMinor
        }
        return total
    }

    public static func build(_ hosts: [HostCost], account: AccountID, windowStart: Date, limitAt: Date?) -> CostSummary {
        let answers = hosts.compactMap(\.answer)
        let mine = answers.flatMap(\.hours).filter { $0.tokens > 0 && $0.provider == account.provider && $0.account == account.name && covers($0, from: windowStart) }
        let billed = answers.flatMap(\.billed).filter { $0.provider == account.provider && $0.account == account.name }
            .max { $0.observedAt < $1.observedAt }
        let models = Dictionary(grouping: mine, by: \.model).map { id, rows in
            Model(model: PaneShares.modelName(id), micros: rows.reduce(0) { $0 + $1.micros }, tokens: rows.reduce(0) { $0 + $1.tokens }, unpriced: rows.reduce(0) { $0 + $1.unpriced })
        }.sorted { ($1.micros, $0.model) < ($0.micros, $1.model) }
        let sinceLimit = limitAt.map { at in mine.filter { covers($0, from: at) } }
        return CostSummary(billed: billed, listMicros: mine.reduce(0) { $0 + $1.micros }, tokens: mine.reduce(0) { $0 + $1.tokens },
                           sinceLimitMicros: sinceLimit.map { $0.reduce(0) { $0 + $1.micros } },
                           models: models, unpriced: mine.reduce(0) { $0 + $1.unpriced }, unread: hosts.filter { $0.answer == nil }.map(\.host),
                           windowBilledMinor: risen(billed?.history ?? [], from: windowStart),
                           answered: !answers.isEmpty, sinceLimitUnpriced: sinceLimit?.reduce(0) { $0 + $1.unpriced } ?? 0)
    }

    /// Whether an hour's work falls from a moment on. The moment's own hour counts: totals are
    /// hourly, so a figure can reach back up to an hour before it.
    private static func covers(_ h: CostHour, from: Date) -> Bool { h.hour.addingTimeInterval(3600) > from }

    public var billedText: String {
        guard let b = billed else { return "Not available" }
        let used = Self.money(b.usedMinor, b)
        if let limit = b.limitMinor { return "\(used) of \(Self.money(limit, b))" }
        return b.enabled ? used : "\(used) · usage credits off"
    }

    /// "Billed this month", or the reading's own month: smithersd keeps its last reading when a
    /// poll fails, so a reading can belong to a month that has ended.
    public func billedLabel(now: Date = Date(), calendar: Calendar = .current) -> String {
        guard let at = billed?.observedAt, !calendar.isDate(at, equalTo: now, toGranularity: .month) else { return "Billed this month" }
        let f = DateFormatter()
        f.calendar = calendar; f.timeZone = calendar.timeZone; f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = calendar.isDate(at, equalTo: now, toGranularity: .year) ? "LLLL" : "LLLL yyyy"
        return "Billed in " + f.string(from: at)
    }

    /// How old the billed reading is, in the host line's words: "Read 2h ago". Nil with no reading.
    public func billedAge(now: Date = Date()) -> String? {
        billed.map { "Read " + ageText($0.observedAt, now: now) }
    }

    /// The window's list price.
    public var listText: String { estimate(listMicros, unpriced: unpriced) }

    /// List price since the window reached 100%. Nil with no such moment.
    public var sinceLimitText: String? { sinceLimitMicros.map { estimate($0, unpriced: sinceLimitUnpriced) } }

    /// The window's tokens, as a floor when a host did not answer.
    public var tokensText: String {
        guard answered else { return "Not available" }
        return unread.isEmpty ? Self.tokens(tokens) : "at least " + Self.tokens(tokens)
    }

    /// Rule 9: with no host answering the amount is unknown, never $0.00. A host that did not
    /// answer, or work with no price, makes it a floor.
    private func estimate(_ micros: Int64, unpriced: Int64) -> String {
        guard answered else { return "Not available" }
        return unpriced > 0 || !unread.isEmpty ? "at least " + Self.plainDollars(micros) : Self.dollars(micros)
    }

    static func money(_ minor: Int64, _ b: BilledReading) -> String {
        let scale = pow(10, Double(b.exponent))
        let value = Double(minor) / scale
        let text = number(value, places: b.exponent)
        return b.currency == "credits" ? "\(text) credits" : "\(b.currency) \(text)"
    }

    /// List price reads as an estimate wherever it appears.
    public static func dollars(_ micros: Int64) -> String { "~" + plainDollars(micros) }

    static func plainDollars(_ micros: Int64) -> String { "$" + number(Double(micros) / 1_000_000, places: 2) }

    private static func number(_ value: Double, places: Int) -> String {
        value.formatted(.number.precision(.fractionLength(places)).locale(Locale(identifier: "en_US")))
    }

    public static func tokens(_ n: Int64) -> String {
        switch n {
        case 1_000_000_000...: String(format: "%.1fB tokens", Double(n) / 1_000_000_000)
        case 1_000_000...: String(format: "%.1fM tokens", Double(n) / 1_000_000)
        case 10_000...: String(format: "%.0fK tokens", Double(n) / 1000)
        default: "\(n) tokens"
        }
    }
}
