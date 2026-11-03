import Foundation
import SwiftUI

/// Money under a spent window, or in place of Azure's chart. Dollars lead, and tokens sit below
/// them in a quieter style (Viz, 2026-11-02).
public struct CostView: View {
    let summary: CostSummary?
    let period: String
    let now: Date

    public init(summary: CostSummary?, period: String, now: Date = Date()) { self.summary = summary; self.period = period; self.now = now }

    public var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if let s = summary {
                let headline = s.headline(period: period)
                VStack(alignment: .leading, spacing: 0) {
                    Text(headline.label).font(.subheadline).foregroundStyle(.secondary)
                    Text(headline.value).font(.largeTitle.bold().monospacedDigit()).lineLimit(1).minimumScaleFactor(0.6)
                    if !s.hasSpend { tokens(s) }
                }
                line(s.billedLabel(now: now), s.billedText)
                if let note = billedNote(s) {
                    Text(note).font(.caption2).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                }
                if let since = s.sinceLimitText { line("Since limit", since) }
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
        Text(s.tokensText).font(.caption).foregroundStyle(.secondary)
    }

    /// The billed reading's age, since a failed poll keeps the last one, and Codex's lag.
    private func billedNote(_ s: CostSummary) -> String? {
        guard let age = s.billedAge(now: now) else { return nil }
        return s.billed?.source == "codex-usage" ? age + ". Reported daily, and can lag about two days." : age
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
