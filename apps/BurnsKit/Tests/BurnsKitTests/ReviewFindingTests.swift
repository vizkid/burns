import Foundation
import Testing
@testable import BurnsKit

/// Findings from the 2026-11 Swift review, each pinned where the logic can be reached.
struct WrongNumberTests {
    let now = Fixture.now, h = Fixture.h
    let id = AccountID(provider: "codex", name: "beta")

    func usage(_ limits: [(String, Int, Double, Date, Date)]) throws -> UsageAnswer {
        let rows = limits.map { l in
            #"{"limit_id":"\#(l.0)","windows":{"\#(l.1)":\#(Fixture.window(l.2, resets: l.3, observed: l.4))}}"#
        }.joined(separator: ",")
        return try Wire.decoder.decode(UsageAnswer.self, from: Data(#"""
        {"host":"one","observed_at":"\#(Fixture.iso(now))","accounts":[
         {"provider":"codex","account":"beta","kind":"subscription","limits":[\#(rows)]}]}
        """#.utf8))
    }

    /// F15: two pools of one length are not one window. The newer read must not hide a spent pool.
    @Test func aSpentPoolIsNotHiddenByANewerReadOfAnother() throws {
        let u = try usage([("codex", 10080, 100, now + 72 * h, now - 3600), ("codex_other", 10080, 5, now + 20 * h, now - 60)])
        let merged = Merge.accounts([u], now: now)
        #expect(merged[0].windows[10080]?.percent == 100)
        #expect(merged[0].quotaLeft(at: now) == 0)
    }

    @Test func eachPoolMergesAcrossHostsOnItsOwn() throws {
        let one = try usage([("codex", 10080, 30, now + 72 * h, now - 60), ("codex_other", 10080, 80, now + 20 * h, now - 3600)])
        let two = try usage([("codex", 10080, 40, now + 72 * h, now - 3600)])
        #expect(Merge.accounts([one, two], now: now)[0].windows[10080]?.percent == 80)
    }

    /// F15: the burndown draws the pool the ring reads, not the other pool's readings.
    @Test func readingsFollowTheChartedPool() throws {
        let reset = now + 72 * h
        let u = try usage([("codex", 10080, 50, reset, now - 60), ("codex_other", 10080, 5, reset, now - 60)])
        let account = Merge.accounts([u], now: now)
        let rows = ["codex", "codex_other"].map {
            Reading(provider: "codex", account: "beta", limitId: $0, minutes: 10080, percent: 7, resetsAt: reset, observedAt: now - h)
        }
        #expect(Merge.readings([rows], accounts: account, now: now).map(\.limitId) == ["codex"])
        #expect(Burndown.build(account: account[0], readings: rows, now: now).line?.actual.count == 3)
    }

    /// F32: a ring and a burndown of one window give one verdict, whatever its length.
    @Test func ringAndBurndownAgreeOnADayLongWindow() throws {
        let u = try usage([("codex", 1440, 70, now + 12 * h, now - 60)])
        let snapshot = HostSnapshot(address: "a", fetchedAt: now, usage: u, readings: [])
        let model = UsageModel(snapshots: [snapshot], selected: id, now: now)
        // 70% used with half the day gone runs out early. Against a week it would read on pace.
        #expect(model.burndown?.line?.verdict == .critical)
        #expect(model.rings[0].tile.sevenDay.verdict == .critical)
        #expect(abs((model.rings[0].tile.sevenDay.elapsed ?? 0) - 0.5) < 0.001)
    }

    /// F50: no quota account means none has any left, so the rule lands on Azure.
    @Test func anAzureOnlySetupPicksAzure() {
        let azure = AccountUsage(id: id, kind: "azure", windows: [:])
        #expect(Pick.choose([azure], now: now) == .azure(id))
    }

    @Test func noAccountsPicksNothing() { #expect(Pick.choose([], now: now) == .none) }

    /// F45: "Exhausted means a live window at 100 percent", so a lapsed window beside one is not
    /// an unknown. A lapsed window never reached Pick through Merge, and now the rule says so.
    @Test func aLapsedWindowBesideASpentOneIsStillExhausted() {
        let spent = AccountUsage(id: id, kind: "subscription", windows: [
            300: Window(percent: 100, resetsAt: now + h, observedAt: now), 10080: Window(percent: 50, resetsAt: now - 1, observedAt: now - h)])
        let azure = AccountUsage(id: AccountID(provider: "codex", name: "delta"), kind: "azure", windows: [:])
        #expect(Pick.choose([spent, azure], now: now) == .azure(azure.id))
    }

    /// F46: the view asks a flag, not the headline's words.
    @Test func creditsAreAFlag() {
        let azure = HostSnapshot(address: "a", fetchedAt: now, usage: UsageAnswer(host: "h", observedAt: now, accounts: [
            WireAccount(provider: "codex", account: "beta", kind: "azure", limits: [])]), readings: [])
        #expect(UsageModel(snapshots: [azure], selected: id, now: now).burndown?.credits == true)
    }

    /// F51: a first layout of width zero asks for no negative width.
    @Test func noWidthIsNeverNegative() {
        #expect(PaneShares.widths([0.5, 0.5], room: -2, minimum: 3) == [0, 0])
    }

    /// F52: a point is Equatable only, so tolerance cannot break a hash.
    @Test func pointsAreEqualWithinRounding() {
        #expect(Point(x: 0.1, left: 5) == Point(x: 0.1 + 1e-10, left: 5))
    }
}

struct LaunchChoiceTests {
    let h = 3600.0

    func entry(_ name: String, _ models: [CatalogModel]) -> ModelEntry {
        ModelEntry(provider: "codex", account: name, defaultId: models[0].id, default: models[0].display, defaultEffort: "", models: models)
    }

    /// F6: two checkouts share a last folder name.
    func twins() -> LaunchSentence {
        let a = LaunchAccount(id: AccountID(provider: "codex", name: "beta"), emoji: "x", left: "1% left", entry: entry("beta", [CatalogModel(id: "m1", display: "M1")]))
        let one = LaunchHost(rex: "", shown: "One", path: "/home/a/dev/a/api", session: "s1")
        let two = LaunchHost(rex: "", shown: "One", path: "/home/a/wt/x/api", session: "s2")
        return LaunchSentence(accounts: [a], projects: [LaunchProject(name: "api", path: "dev/a/api", hosts: [one]), LaunchProject(name: "api", path: "wt/x/api", hosts: [two])])
    }

    @Test func projectsOfOneNameAreBothReachable() {
        var s = twins()
        s.focus(.project)
        #expect(Set(s.options.map(\.id)).count == 2)
        s.move(1)
        #expect(s.request?.host.path == "/home/a/wt/x/api")
        s.pick(s.options[0].id)
        #expect(s.request?.host.path == "/home/a/dev/a/api")
    }

    /// F11: a project on another host can leave a picked model out of that host's catalog.
    @Test func aModelTheNewHostLacksIsDropped() {
        let id = AccountID(provider: "codex", name: "beta")
        let shared = CatalogModel(id: "m1", display: "M1"), only = CatalogModel(id: "m2", display: "M2")
        let a = LaunchHost(rex: "A", shown: "A", path: "/p", session: "s", entries: [id: entry("beta", [shared, only])])
        let b = LaunchHost(rex: "B", shown: "B", path: "/q", session: "s", entries: [id: entry("beta", [shared])])
        var s = LaunchSentence(accounts: [LaunchAccount(id: id, emoji: "x", left: "", entry: nil)],
                               projects: [LaunchProject(name: "p", path: "p", hosts: [a]), LaunchProject(name: "q", path: "q", hosts: [b])])
        s.focus(.model)
        s.pick(s.options[1].id)
        #expect(s.request?.model == "m2")
        s.focus(.project)
        s.pick(s.options[1].id)
        #expect(s.request?.host.rex == "B" && s.request?.model == nil)
    }
}

/// F2, F47: one parser and one rule for Rex's host list. Output shaped as `rex hosts list --json`.
struct RexHostsTests {
    let list = Data(#"""
    {"home":{"label":"","this_machine":true},"warnings":[],"hosts":[
     {"from":"home","held":false,"host":{"id":"host:1","label":"Alpha","endpoints":["https://a"],"icon":"macstudio"}},
     {"from":"home","held":false,"host":{"id":"host:2","label":"Delta"}},
     {"from":"home","held":false,"host":{"id":"host:3","label":"delta.tail.ts.net","icon":"server.rack"}},
     {"from":"home","held":false,"host":{"id":"host:4","label":"gamma.tail.ts.net"}}]}
    """#.utf8)

    func hosts() throws -> RexHosts { try JSONDecoder().decode(RexHosts.self, from: list) }

    @Test func aPlainLabelComesBeforeATailnetName() throws {
        #expect(try hosts().server(for: "delta") == "Delta")
    }

    @Test func aTailnetEntryMatchesAShortName() throws {
        #expect(try hosts().server(for: "gamma") == "gamma.tail.ts.net")
    }

    @Test func anIconOnAnyEntryCounts() throws {
        #expect(try hosts().icon(for: "delta") == "server.rack" && hosts().icon(for: "gamma") == nil)
    }

    @Test func thisMachineGetsNoServer() throws {
        #expect(RexHosts.place(host: "Mine", here: "mine", rex: nil) == .here)
        #expect(RexHosts.place(host: "mine", here: "Mine.local", rex: try hosts()) == .here)
    }

    /// The case that launched on the Mac under another host's name.
    @Test func noOtherHostFallsThroughToThisMac() throws {
        #expect(RexHosts.place(host: "elsewhere", here: "mine", rex: try hosts()) == .unreachable("no Rex entry"))
        #expect(RexHosts.place(host: "alpha", here: "mine", rex: nil) == .unreachable("Rex did not answer"))
        #expect(RexHosts.place(host: "alpha", here: "mine", rex: try hosts()) == .server("Alpha"))
    }

    @Test func thisMachineHasAName() { #expect(!RexHosts.thisMachine.isEmpty && !RexHosts.thisMachine.contains(".")) }
}

struct LaunchRankTests {
    let now = Fixture.now, h = Fixture.h

    func quota(_ provider: String, _ name: String, _ percent: Double?) -> AccountUsage {
        AccountUsage(id: AccountID(provider: provider, name: name), kind: "subscription",
                     windows: percent.map { [10080: Window(percent: $0, resetsAt: now + h, observedAt: now)] } ?? [:])
    }

    @Test func accountsRankByQuotaThenUnknownThenCredits() {
        let azure = AccountUsage(id: AccountID(provider: "codex", name: "delta"), kind: "azure", windows: [:])
        let list = [azure, quota("claude", "gamma", nil), quota("claude", "alpha", 90), quota("codex", "beta", 40)]
        let out = LaunchAccount.ranked(list, pick: nil, central: [], entries: [:], now: now)
        #expect(out.map(\.id.name) == ["beta", "alpha", "gamma", "delta"])
        #expect(out.map(\.left) == ["60% left", "10% left", "usage unknown", "credits"])
    }

    @Test func thePickGoesFirst() {
        let list = [quota("codex", "beta", 40), quota("claude", "alpha", 100)]
        #expect(LaunchAccount.ranked(list, pick: AccountID(provider: "claude", name: "alpha"), central: [], entries: [:], now: now)[0].id.name == "alpha")
    }

    @Test func theModelSaysWhyEachPickWon() {
        let beta = AccountID(provider: "codex", name: "beta"), azure = AccountUsage(id: beta, kind: "azure", windows: [:])
        #expect(Pick.quota(beta, left: 62).reason(among: []) == "Codex beta: 62% left")
        #expect(Pick.unknown(beta).reason(among: []) == "Usage unknown: Codex beta")
        #expect(Pick.azure(beta).reason(among: [quota("codex", "alpha", 100), azure]) == "All quotas exhausted: Azure")
        #expect(Pick.azure(beta).reason(among: [azure]) == "No quota accounts: Azure")
    }

    /// F36: this Mac can look, so only a folder that exists is offered. Nothing else is guessed.
    @Test func aNewSessionNeedsAFolderThatExists() {
        let here = LaunchHost(rex: "", shown: "Mine", path: "/home/me/dev/a", session: "a")
        let other = LaunchHost(rex: "Two", shown: "Two", path: "/home/you/dev/b", session: "b")
        let projects = [LaunchProject(name: "a", path: "dev/a", hosts: [here]), LaunchProject(name: "b", path: "dev/b", hosts: [other]),
                        LaunchProject(name: "c", path: "dev/c", hosts: [other])]
        let out = LaunchProject.offer(here, home: "/home/me", to: projects) { $0 == "/home/me/dev/b" }
        #expect(out[0].hosts.count == 1)
        #expect(out[1].hosts.map(\.rex) == ["Two", ""] && out[1].hosts[1].session == nil && out[1].hosts[1].path == "/home/me/dev/b")
        #expect(out[2].hosts.map(\.rex) == ["Two"])
    }

    /// F39: nothing on a host expands a tilde.
    @Test func aTypedTildeIsTheUsersHome() {
        #expect(AccountGroup.cleanHome(" ~/.claude-work ") == ".claude-work")
        #expect(AccountGroup.cleanHome(".claude-work") == ".claude-work")
        #expect(AccountGroup.cleanHome("/opt/home") == "/opt/home")
        #expect(AccountGroup.cleanHome("") == "")
        #expect(AccountGroup.cleanHome("~") == nil && AccountGroup.cleanHome("~/") == nil && AccountGroup.cleanHome("~other/x") == nil)
    }

    /// The Go side is making `models` an empty list. Older hosts still answer null.
    @Test func aNullCatalogIsEmpty() throws {
        let data = Data(#"{"host":"h","accounts":[{"provider":"codex","account":"a","default_id":"","default":"","default_effort":"","models":null}]}"#.utf8)
        #expect(try Wire.decoder.decode(ModelsAnswer.self, from: data).accounts[0].models.isEmpty)
    }
}

struct HostCacheOrderTests {
    let usage = Fixture.usage(host: "h", alpha5: 1, alpha7: 1, beta7: 1)

    let scratch = ScratchDefaults()

    func cache() -> HostCache { HostCache(defaults: scratch.defaults) }

    /// F38: a host removed while a slow pass runs stays removed.
    @Test func aSlowOlderRefreshCannotOverwriteANewerOne() async throws {
        let cache = cache()
        let usage = usage
        let older = Task { await cache.refresh(hosts: ["a", "gone"]) { host in
            try await Task.sleep(for: .milliseconds(150))
            return try Fixture.snapshot(host, usage: usage)
        } }
        try await Task.sleep(for: .milliseconds(30))
        let newer = await cache.refresh(hosts: ["a"]) { host in try Fixture.snapshot(host, usage: usage) }
        _ = await older.value
        #expect(newer.map(\.address) == ["a"])
        #expect(Set(cache.load().keys) == ["a"])
    }

    /// The widget stores to the same key from its own process.
    @Test func aNewerAnswerFromAnotherProcessStays() async throws {
        let cache = cache(), usage = usage
        let later = Fixture.now + 600
        _ = await cache.refresh(hosts: ["a"]) { host in
            cache.save([host: try Fixture.snapshot(host, usage: usage, fetched: later)])
            return try Fixture.snapshot(host, usage: usage, fetched: Fixture.now)
        }
        #expect(cache.load()["a"]?.fetchedAt == later)
    }
}

#if os(macOS)
/// F21, F22.
struct CommandTests {
    let sh = "/bin/sh"

    @Test func moreThanAPipeHoldsDoesNotStall() async throws {
        let bytes = 300_000
        let out = await Command.run(sh, ["-c", "head -c \(bytes) /dev/zero; head -c \(bytes) /dev/zero >&2"], limit: 10)
        #expect(try out.get().count == bytes)
    }

    @Test func aFailureCarriesItsErrorOutput() async {
        let out = await Command.run(sh, ["-c", "echo nope >&2; exit 3"])
        #expect(out == .failure(CommandFailure(text: "nope\n")))
    }

    @Test func aLimitStopsTheProgram() async {
        let start = Date()
        guard case .failure = await Command.run(sh, ["-c", "exec sleep 30"], limit: 0.1) else { Issue.record("no failure"); return }
        #expect(Date().timeIntervalSince(start) < 5)
    }

    /// A child that inherits the pipes must not hold the answer back.
    @Test func aChildHoldingThePipesDoesNotHang() async {
        let start = Date()
        _ = await Command.run(sh, ["-c", "sleep 8 & exit 0"])
        #expect(Date().timeIntervalSince(start) < 5)
    }

    @Test func aProgramThatCannotStartFails() async {
        guard case .failure = await Command.run("/nonexistent/burns", []) else { Issue.record("no failure"); return }
    }

    @Test func aProgramSeesNoInput() async throws {
        let out = await Command.run(sh, ["-c", "cat"], limit: 5)
        #expect(try out.get().isEmpty)
    }

    /// Start files print around the answer, before it and after it.
    @Test func theMarkerPicksTheAnswerOutOfShellNoise() {
        #expect(Command.located(Data("welcome\nburns-found:/opt/bin/rex\nbye\n".utf8)) == "/opt/bin/rex")
        #expect(Command.located(Data("burns-found:\n".utf8)) == nil)
        #expect(Command.located(Data("/opt/bin/rex\n".utf8)) == nil)
    }
}
#endif
