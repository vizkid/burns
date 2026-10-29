import Foundation
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Apps and widgets": each app reads every host itself and keeps each host's last
/// answer, so a host that does not answer shows its cached readings marked with their time.
struct HostCacheTests {
    let now = Fixture.now, h = Fixture.h

    func defaults() -> UserDefaults {
        let name = "burnskit-test-\(UUID().uuidString)"
        let d = UserDefaults(suiteName: name)!
        d.removePersistentDomain(forName: name)
        return d
    }

    struct Down: Error {}

    @Test func aHostThatAnswersReplacesItsCache() async throws {
        let cache = HostCache(defaults: defaults())
        let fresh = try Fixture.snapshot("one.example.ts.net", usage: Fixture.usage(host: "one", alpha5: 10, alpha7: 20, beta7: nil))
        let status = await cache.refresh(hosts: ["one.example.ts.net"]) { _ in fresh }
        #expect(status == [HostStatus(address: "one.example.ts.net", snapshot: fresh, error: nil)])
        #expect(cache.load()["one.example.ts.net"] == fresh)
    }

    @Test func aHostThatFailsKeepsItsLastAnswer() async throws {
        let cache = HostCache(defaults: defaults())
        let old = try Fixture.snapshot("two.example.ts.net", usage: Fixture.usage(host: "two", alpha5: 10, alpha7: 20, beta7: nil), fetched: now - 3 * h)
        cache.save(["two.example.ts.net": old])
        let status = await cache.refresh(hosts: ["two.example.ts.net", "three.example.ts.net"]) { _ in throw Down() }
        #expect(status[0].snapshot == old && status[0].error != nil)
        #expect(status[1].snapshot == nil && status[1].error != nil)
        #expect(status.map(\.address) == ["two.example.ts.net", "three.example.ts.net"], "settings order, not finish order")
    }

    /// A host removed from settings leaves the cache.
    @Test func removedHostsLeaveTheCache() async throws {
        let cache = HostCache(defaults: defaults())
        let old = try Fixture.snapshot("gone.example.ts.net", usage: Fixture.usage(host: "gone", alpha5: 10, alpha7: nil, beta7: nil))
        cache.save(["gone.example.ts.net": old])
        _ = await cache.refresh(hosts: []) { _ in throw Down() }
        #expect(cache.load().isEmpty)
    }

    @Test func selectionFallsBackToEnterThenFirstRing() throws {
        let d = defaults()
        let snap = try Fixture.snapshot("one", usage: Fixture.usage(host: "one", alpha5: 90, alpha7: 30, beta7: 20))
        var model = UsageModel(snapshots: [snap], selected: nil, now: now)
        #expect(model.rings.map(\.id.label) == ["Claude alpha", "Codex alpha", "Claude beta", "Codex beta"])
        #expect(model.selected == AccountID(provider: "codex", name: "beta"), "Enter's pick: 80% left")
        #expect(model.burndown?.account == model.selected)

        let selection = Selection(defaults: d)
        selection.account = AccountID(provider: "claude", name: "alpha")
        model = UsageModel(snapshots: [snap], selected: selection.account, now: now)
        #expect(model.selected == AccountID(provider: "claude", name: "alpha"))

        // A stored selection that is no longer a ring falls back to Enter's pick.
        model = UsageModel(snapshots: [snap], selected: AccountID(provider: "claude", name: "gone"), now: now)
        #expect(model.selected == AccountID(provider: "codex", name: "beta"))

        // Nothing known: the first ring.
        let blank = try Fixture.snapshot("one", usage: Fixture.usage(host: "one", alpha5: nil, alpha7: nil, beta7: nil))
        #expect(UsageModel(snapshots: [blank], selected: nil, now: now).selected == AccountID(provider: "claude", name: "alpha"))
    }

    /// Rule 2 at display time: a cached window whose reset passed reads unknown.
    @Test func cachedWindowsExpireAtReset() throws {
        let snap = try Fixture.snapshot("one", usage: Fixture.usage(host: "one", alpha5: 90, alpha7: 30, beta7: nil))
        let later = UsageModel(snapshots: [snap], selected: nil, now: now + 3 * h)
        let alpha = try #require(later.rings.first)
        #expect(alpha.tile.fiveHour.percent == nil && alpha.tile.sevenDay.percent == 30)
    }
}
