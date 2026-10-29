import Foundation
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Default rule": the account Enter would pick is the widget's default ring.
struct PickTests {
    let now = Fixture.now, h = Fixture.h

    func account(_ provider: String, _ name: String, kind: String = "subscription", _ windows: [Int: Double?]) -> AccountUsage {
        AccountUsage(id: AccountID(provider: provider, name: name), kind: kind,
                     windows: windows.mapValues { Window(percent: $0, resetsAt: $0 == nil ? now - 1 : now + h, observedAt: now - 60) })
    }

    @Test func mostQuotaLeftWins() {
        let accounts = [account("claude", "alpha", [300: 50, 10080: 70]), account("codex", "beta", [10080: 40])]
        #expect(Pick.choose(accounts, now: now) == .quota(AccountID(provider: "codex", name: "beta"), left: 60))
    }

    @Test func aTieGoesToTheEarlierAccount() {
        let accounts = [account("claude", "alpha", [10080: 40]), account("codex", "beta", [10080: 40])]
        #expect(Pick.choose(accounts, now: now).account == AccountID(provider: "claude", name: "alpha"))
    }

    /// Rule 3: an unknown window, or no windows at all, does not compete.
    @Test func unknownAccountsDoNotCompete() {
        let accounts = [account("claude", "alpha", [300: nil, 10080: 1]), account("claude", "beta", [:]),
                        account("codex", "beta", [10080: 90])]
        #expect(Pick.choose(accounts, now: now).account == AccountID(provider: "codex", name: "beta"))
    }

    /// Rule 5: every quota account known to be exhausted chooses Azure.
    @Test func allExhaustedChoosesAzure() {
        let accounts = [account("claude", "alpha", [10080: 100]), account("codex", "beta", [10080: 100]),
                        account("codex", "beta-azure", kind: "azure", [:])]
        #expect(Pick.choose(accounts, now: now) == .azure(AccountID(provider: "codex", name: "beta-azure")))
    }

    /// Rule 6: nothing left but some unknown chooses the first quota account, marked unknown.
    @Test func exhaustedOrUnknownFallsBack() {
        let accounts = [account("claude", "alpha", [:]), account("codex", "beta", [10080: 100]),
                        account("codex", "beta-azure", kind: "azure", [:])]
        #expect(Pick.choose(accounts, now: now) == .unknown(AccountID(provider: "claude", name: "alpha")))
    }

    /// Rings: quota accounts only, each name's Claude then its Codex, names in settings order.
    @Test func ringOrder() {
        let accounts = [account("codex", "beta", [:]), account("claude", "alpha", [:]), account("codex", "alpha", [:]),
                        account("claude", "beta", [:]), account("codex", "beta-azure", kind: "azure", [:])]
        #expect(RingOrder.ordered(accounts).map(\.id.label) == ["Claude beta", "Codex beta", "Claude alpha", "Codex alpha"])
    }
}
