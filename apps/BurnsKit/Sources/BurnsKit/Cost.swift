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
        return CostSummary(billed: billed, listMicros: mine.reduce(0) { $0 + $1.micros }, tokens: mine.reduce(0) { $0 + $1.tokens },
                           sinceLimitMicros: limitAt.map { at in mine.filter { covers($0, from: at) }.reduce(0) { $0 + $1.micros } },
                           models: models, unpriced: mine.reduce(0) { $0 + $1.unpriced }, unread: hosts.filter { $0.answer == nil }.map(\.host),
                           windowBilledMinor: risen(billed?.history ?? [], from: windowStart))
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

    /// The window's list price, as a floor when some of it has no price.
    public var listText: String {
        unpriced > 0 ? "at least " + Self.plainDollars(listMicros) : Self.dollars(listMicros)
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

/// Money under a spent window, or in place of Azure's chart. Dollars lead, and tokens sit below
/// them in a quieter style (Viz, 2026-11-02).
public struct CostView: View {
    let summary: CostSummary?
    let period: String

    public init(summary: CostSummary?, period: String) { self.summary = summary; self.period = period }

    public var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let s = summary {
                let headline = s.headline(period: period)
                VStack(alignment: .leading, spacing: 0) {
                    Text(headline.label).font(.subheadline).foregroundStyle(.secondary)
                    Text(headline.value).font(.largeTitle.bold().monospacedDigit()).lineLimit(1).minimumScaleFactor(0.6)
                    if !s.hasSpend { tokens(s) }
                }
                line("Billed this month", s.billedText)
                if let since = s.sinceLimitMicros { line("Since limit", CostSummary.dollars(since)) }
                // Tokens are the list price's: billed money can cover far fewer of them.
                if s.hasSpend {
                    VStack(alignment: .leading, spacing: 1) {
                        line("\(period), list price", s.listText)
                        tokens(s)
                    }
                }
                ForEach(s.models.prefix(4)) { m in
                    HStack(alignment: .firstTextBaseline) {
                        Text(m.model)
                        Spacer(minLength: 8)
                        Text(CostSummary.tokens(m.tokens)).font(.caption2)
                        Text(m.amountText).monospacedDigit().foregroundStyle(.primary)
                    }
                    .font(.caption).foregroundStyle(.secondary)
                }
                if !s.unread.isEmpty {
                    Text("Not read: \(s.unread.joined(separator: ", ")). List price leaves those hosts out.")
                        .font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                }
                Text("List price is an estimate of the work at published rates. Only Billed is money charged.")
                    .font(.caption2).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            } else {
                Text("Reading cost").font(.footnote).foregroundStyle(.secondary)
            }
        }
    }

    private func tokens(_ s: CostSummary) -> some View {
        Text(CostSummary.tokens(s.tokens)).font(.caption).foregroundStyle(.secondary)
    }

    private func line(_ label: String, _ value: String) -> some View {
        HStack(alignment: .firstTextBaseline) {
            Text(label).foregroundStyle(.secondary)
            Spacer(minLength: 8)
            Text(value).monospacedDigit()
        }
        .font(.subheadline)
    }
}
