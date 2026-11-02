import Foundation
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Proposal: billed spend and list price".
struct CostTests {
    let now = Fixture.now, h = Fixture.h
    let id = AccountID(provider: "codex", name: "alpha")

    func hour(_ hoursAgo: Double, _ model: String = "gpt-6.1-sol", dollars: Double, tokens: Int64 = 1000, unpriced: Int64 = 0, account: String = "alpha") -> CostHour {
        CostHour(hour: now - hoursAgo * h, provider: "codex", account: account, model: model, requests: 1, tokens: tokens, micros: Int64(dollars * 1_000_000), unpriced: unpriced)
    }

    func billed(_ used: Int64, limit: Int64? = nil, at: Date, enabled: Bool = true, reason: String? = nil, currency: String = "SGD") -> BilledReading {
        BilledReading(provider: "codex", account: "alpha", source: "x", currency: currency, usedMinor: used, exponent: 2, limitMinor: limit, enabled: enabled, reason: reason, observedAt: at)
    }

    func summary(_ hosts: [HostCost], start: Date, limitAt: Date? = nil) -> CostSummary {
        CostSummary.build(hosts, account: id, windowStart: start, limitAt: limitAt)
    }

    /// Each host reports its own work, so hours add up across hosts. Other accounts stay out.
    @Test func hoursAddAcrossHosts() {
        let a = HostCost(host: "a", answer: CostAnswer(host: "a", hours: [hour(2, dollars: 10), hour(3, dollars: 5, account: "beta")], billed: []))
        let b = HostCost(host: "b", answer: CostAnswer(host: "b", hours: [hour(2, dollars: 2.5)], billed: []))
        let s = summary([a, b], start: now - 24 * h)
        #expect(s.listMicros == 12_500_000 && s.tokens == 2000 && s.unread.isEmpty)
    }

    /// Since limit counts only the hours from the moment the window reached 100%, by the hour it fell in.
    @Test func sinceLimitLeavesOutPlanUsage() {
        let hosts = [HostCost(host: "a", answer: CostAnswer(host: "a", hours: [hour(30, dollars: 100), hour(3, dollars: 7), hour(1, dollars: 3)], billed: []))]
        let s = summary(hosts, start: now - 48 * h, limitAt: now - 3 * h)
        #expect(s.listMicros == 110_000_000 && s.sinceLimitMicros == 10_000_000)
    }

    /// With no limit reached there is no such number, not a zero.
    @Test func noLimitNoSinceLimit() {
        let s = summary([HostCost(host: "a", answer: CostAnswer(host: "a", hours: [hour(1, dollars: 3)], billed: []))], start: now - h * 24)
        #expect(s.sinceLimitMicros == nil)
    }

    /// Hours before the window are not this window's.
    @Test func hoursBeforeTheWindowStayOut() {
        let s = summary([HostCost(host: "a", answer: CostAnswer(host: "a", hours: [hour(30, dollars: 100), hour(1, dollars: 3)], billed: []))], start: now - 24 * h)
        #expect(s.listMicros == 3_000_000)
    }

    /// Models read largest first, with their dollars and tokens.
    @Test func modelsRankByDollars() {
        let hosts = [HostCost(host: "a", answer: CostAnswer(host: "a", hours: [hour(1, "gpt-6.1-sol", dollars: 2), hour(1, "gpt-6-astra", dollars: 9), hour(2, "gpt-6.1-sol", dollars: 3)], billed: []))]
        let s = summary(hosts, start: now - 24 * h)
        #expect(s.models.map(\.model) == ["Astra 6", "Sol 6.1"])
        #expect(s.models.map(\.micros) == [9_000_000, 5_000_000])
    }

    /// Rule 9: a host that did not answer is named, and an unknown price makes the total a floor.
    @Test func unknownIsNeverZero() {
        let hosts = [HostCost(host: "a", answer: CostAnswer(host: "a", hours: [hour(1, "new-model", dollars: 0, unpriced: 4), hour(1, dollars: 2)], billed: [])),
                     HostCost(host: "b", answer: nil)]
        let s = summary(hosts, start: now - 24 * h)
        #expect(s.unpriced == 4 && s.unread == ["b"])
        #expect(s.listText == "at least $2.00")
    }

    /// A model with no price says so, and never reads as free.
    @Test func anUnpricedModelReadsAsUnknown() {
        let s = summary([HostCost(host: "a", answer: CostAnswer(host: "a", hours: [hour(1, "new-model", dollars: 0, unpriced: 4)], billed: []))], start: now - 24 * h)
        #expect(s.models.first?.amountText == "No price")
    }

    /// A row with no tokens is no work: it adds no line, and it does not make the total a floor.
    @Test func zeroTokenRowsAreNoWork() {
        let hosts = [HostCost(host: "a", answer: CostAnswer(host: "a", hours: [hour(1, "<synthetic>", dollars: 0, tokens: 0, unpriced: 5), hour(1, dollars: 2)], billed: []))]
        let s = summary(hosts, start: now - 24 * h)
        #expect(s.unpriced == 0 && s.models.count == 1 && s.listText == "~$2.00")
    }

    /// One account's login can sit on several hosts: the newest reading is the account's.
    @Test func newestBilledWins() {
        let hosts = [HostCost(host: "a", answer: CostAnswer(host: "a", hours: [], billed: [billed(100, at: now - h)])),
                     HostCost(host: "b", answer: CostAnswer(host: "b", hours: [], billed: [billed(250, limit: 200_000, at: now - 60)]))]
        let s = summary(hosts, start: now - 24 * h)
        #expect(s.billed?.usedMinor == 250)
        #expect(s.billedText == "SGD 2.50 of SGD 2,000.00")
    }

    @Test func billedWords() {
        func text(_ b: BilledReading?) -> String { summary([HostCost(host: "a", answer: CostAnswer(host: "a", hours: [], billed: b.map { [$0] } ?? []))], start: now - h).billedText }
        #expect(text(nil) == "Not available")
        #expect(text(billed(0, at: now, enabled: false, reason: "out_of_credits")) == "SGD 0.00 · usage credits off")
        #expect(text(billed(150_050, at: now, currency: "credits")) == "1,500.50 credits")
    }

    func history(_ points: (Int64, Double)...) -> BilledReading {
        var b = billed(points.last?.0 ?? 0, at: now)
        b.history = points.map { BilledPoint(usedMinor: $0.0, observedAt: now - $0.1 * h) }
        return b
    }

    func inWindow(_ b: BilledReading) -> CostSummary {
        summary([HostCost(host: "a", answer: CostAnswer(host: "a", hours: [], billed: [b]))], start: now - 24 * h)
    }

    /// A provider reports the month, so the window's spend is what the total rose by inside it.
    @Test func windowBilledIsTheRise() {
        let s = inWindow(history((500, 40), (700, 10), (900, 2)))
        #expect(s.windowBilledMinor == 400)
        #expect(s.hasSpend && s.headline.value == "SGD 4.00" && s.headline.label == "Billed this window")
    }

    /// Spending from before the window is not this window's.
    @Test func noRiseIsNoSpend() {
        #expect(inWindow(history((500, 40))).windowBilledMinor == 0)
        #expect(!inWindow(history((500, 40))).hasSpend)
    }

    /// A new month starts the total again: the fall is not negative spend.
    @Test func aNewMonthCountsFromZero() {
        #expect(inWindow(history((5000, 30), (5100, 12), (40, 2))).windowBilledMinor == 140)
    }

    /// With no spend the headline is the list price, labelled as that.
    @Test func headlineFallsBackToListPrice() {
        let s = summary([HostCost(host: "a", answer: CostAnswer(host: "a", hours: [hour(1, dollars: 2)], billed: []))], start: now - 24 * h)
        #expect(s.headline(period: "Last 7 days").value == "~$2.00" && s.headline(period: "Last 7 days").label == "Last 7 days, list price")
    }

    /// A five-hour limit reached under the week: the week is not spent, so the chart stays, and
    /// list price counts from that limit while it holds.
    @Test func aFiveHourLimitStartsSinceLimit() {
        let week = Window(percent: 40, resetsAt: now + 72 * h, observedAt: now), five = Window(percent: 100, resetsAt: now + h, observedAt: now)
        let u = AccountUsage(id: id, kind: "subscription", windows: [10080: week, 300: five])
        let rs = [Reading(provider: "codex", account: "alpha", limitId: "codex", minutes: 300, percent: 100, resetsAt: now + h, observedAt: now - 2 * h)]
        let b = Burndown.build(account: u, readings: rs, now: now)
        #expect(!b.spent && b.limitAt == now - 2 * h)
    }

    /// List price reads as an estimate, and tokens read in a compact form.
    @Test func amounts() {
        #expect(CostSummary.dollars(1_234_560_000) == "~$1,234.56")
        #expect(CostSummary.tokens(2_814_880_474) == "2.8B tokens")
        #expect(CostSummary.tokens(4_300_000) == "4.3M tokens")
        #expect(CostSummary.tokens(900) == "900 tokens")
    }

    /// The window's facts come with the burndown: a spent window knows when it reached 100%.
    @Test func burndownKnowsTheLimit() throws {
        let reset = now + 72 * h
        let u = AccountUsage(id: id, kind: "subscription", windows: [10080: Window(percent: 100, resetsAt: reset, observedAt: now)])
        let rs = [Reading(provider: "codex", account: "alpha", limitId: "codex", minutes: 10080, percent: 90, resetsAt: reset, observedAt: now - 8 * h),
                  Reading(provider: "codex", account: "alpha", limitId: "codex", minutes: 10080, percent: 100, resetsAt: reset, observedAt: now - 5 * h)]
        let b = Burndown.build(account: u, readings: rs, now: now)
        #expect(b.spent && b.limitAt == now - 5 * h)
        #expect(b.windowStart == reset - 168 * h)
    }

    /// A live window under its limit is not spent, and shows the full chart.
    @Test func liveWindowIsNotSpent() {
        let u = AccountUsage(id: id, kind: "subscription", windows: [10080: Window(percent: 40, resetsAt: now + 72 * h, observedAt: now)])
        let b = Burndown.build(account: u, readings: [], now: now)
        #expect(!b.spent && b.limitAt == nil)
    }
}
