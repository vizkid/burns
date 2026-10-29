import Foundation
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Merging hosts" and rules 1, 2 and 6. The window cases mirror
/// internal/quota/quota_test.go, so the Go and Swift merges agree.
struct MergeTests {
    let now = Fixture.now, h = Fixture.h

    @Test func decodesRealShapes() throws {
        let u = try Wire.decoder.decode(UsageAnswer.self, from: Fixture.usage(host: "one", alpha5: 12, alpha7: 30, beta7: nil))
        #expect(u.host == "one" && u.accounts.count == 5)
        #expect(abs(u.observedAt.timeIntervalSince1970 - 1_793_253_600.347688) < 0.001)
        let w = try #require(u.accounts[0].limits.first?.windows["300"])
        #expect(w.percent == 12 && w.resetsAt == now + 2 * h)
        let unknown = try Wire.decoder.decode(UsageAnswer.self, from: Fixture.usage(host: "one", alpha5: nil, alpha7: nil, beta7: nil))
        #expect(unknown.accounts[0].limits.isEmpty)
    }

    @Test func newestLiveReadingWinsPerWindow() {
        let older = [300: Window(percent: 40, resetsAt: now + h, observedAt: now - 60)]
        let newer = [300: Window(percent: 10, resetsAt: now + h, observedAt: now - 10),
                     10080: Window(percent: 5, resetsAt: now + 50 * h, observedAt: now - 3600)]
        let m = Merge.windows([older, newer], now: now)
        #expect(m[300]?.percent == 10 && m[10080]?.percent == 5)
        #expect(Merge.windows([newer, older], now: now) == m, "host order does not matter")
    }

    /// Rule 2: a reading whose reset passed is unknown, never zero, and loses to nothing.
    @Test func expiredReadingsDropOut() {
        let expired = [300: Window(percent: 90, resetsAt: now - 1, observedAt: now - 10)]
        #expect(Merge.windows([expired], now: now).isEmpty)
        let live = [300: Window(percent: 20, resetsAt: now + h, observedAt: now - 7200)]
        #expect(Merge.windows([expired, live], now: now)[300]?.percent == 20)
    }

    /// Quota is shared, so hosts are not added: the newest reading per window wins.
    @Test func accountsMergeAcrossHosts() throws {
        let a = try Wire.decoder.decode(UsageAnswer.self, from: Fixture.usage(host: "a", alpha5: 50, alpha7: 30, beta7: 9, observed: now - 3600))
        let b = try Wire.decoder.decode(UsageAnswer.self, from: Fixture.usage(host: "b", alpha5: nil, alpha7: nil, beta7: 100))
        let merged = Merge.accounts([a, b], now: now)
        #expect(merged.map(\.id.label) == ["Claude alpha", "Claude beta", "Codex alpha", "Codex beta", "Codex beta-azure"])
        let beta = try #require(merged.first { $0.id == AccountID(provider: "codex", name: "beta") })
        #expect(beta.windows[10080]?.percent == 100)
        #expect(merged[0].windows[300]?.percent == 50)
        #expect(merged.last?.isQuota == false)
    }

    /// The burndown's readings: the union of every host's, each once, oldest first, without
    /// readings of a window the merged window has replaced.
    @Test func readingsUnionDropsDuplicatesAndSupersededWindows() throws {
        let reset = now + 72 * h, oldReset = now + 10 * h
        let one = try Wire.decoder.decode(ReadingsAnswer.self, from: Fixture.readings(host: "a", [
            ("claude", "alpha", 10080, 20, reset, now - 5 * h), ("claude", "alpha", 10080, 25, reset, now - 3 * h),
            ("claude", "alpha", 10080, 80, oldReset, now - 50 * h)]))
        let two = try Wire.decoder.decode(ReadingsAnswer.self, from: Fixture.readings(host: "b", [
            ("claude", "alpha", 10080, 25, reset, now - 3 * h), ("claude", "alpha", 10080, 30, reset, now - h)]))
        let usage = try Wire.decoder.decode(UsageAnswer.self, from: Fixture.usage(host: "a", alpha5: nil, alpha7: 30, beta7: nil))
        let rs = Merge.readings([one.readings, two.readings], accounts: Merge.accounts([usage], now: now), now: now)
        #expect(rs.map(\.percent) == [20, 25, 30])
    }
}
