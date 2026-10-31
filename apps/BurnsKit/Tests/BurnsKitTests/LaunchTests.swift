import Foundation
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "The trigger modal", with synthetic accounts.
struct LaunchTests {
    func entry(_ provider: String, _ name: String, _ d: String, _ effort: String, _ efforts: [String]?) -> ModelEntry {
        ModelEntry(provider: provider, account: name, defaultId: d, default: d.uppercased(), defaultEffort: effort,
                   models: [CatalogModel(id: "m1", display: "M1", efforts: efforts), CatalogModel(id: "m2", display: "M2", efforts: efforts)])
    }

    func sentence() -> LaunchSentence {
        let h1 = LaunchHost(rex: "", shown: "One", path: "/home/a/dev/x", session: "x")
        let h2 = LaunchHost(rex: "Two", shown: "Two", path: "/home/b/dev/x", session: nil)
        let accounts = [
            LaunchAccount(id: AccountID(provider: "codex", name: "beta"), emoji: "💎", left: "96% left", entry: entry("codex", "beta", "m1", "medium", nil)),
            LaunchAccount(id: AccountID(provider: "codex", name: "alpha"), emoji: "🎱", left: "72% left", entry: entry("codex", "alpha", "m2", "medium", nil)),
            LaunchAccount(id: AccountID(provider: "claude", name: "alpha"), emoji: "🎱", left: "60% left", entry: entry("claude", "alpha", "m1", "high", ["low", "high"])),
        ]
        return LaunchSentence(accounts: accounts, projects: [LaunchProject(name: "x", path: "dev/x", hosts: [h1, h2])])
    }

    @Test func startsFullyFilledFromTheBestAccount() {
        let s = sentence()
        #expect(s.words == [.agent, .model, .account, .project, .host])
        #expect(s.text(.agent) == "Codex" && s.text(.model) == "M1 medium" && s.text(.account) == "💎 beta" && s.text(.host) == "One")
    }

    @Test func defaultsPassNoFlags() {
        #expect(sentence().request?.arguments == ["window", "new", "-s", "x", "-c", "/home/a/dev/x", "💎", "--",
                                                  "smithers", "launch", "--provider", "codex", "--account", "beta"])
    }

    /// Claude's effort is a word; Codex's is fixed in the model word.
    @Test func effortIsAWordOnlyWhereItCanBeSet() {
        var s = sentence()
        s.pick("Claude")
        #expect(s.words.contains(.effort) && s.text(.model) == "M1" && s.text(.effort) == "high")
        s.focus(.effort)
        s.pick("low")
        #expect(s.request?.effort == "low")
    }

    @Test func anUntouchedModelFollowsTheAccount() {
        var s = sentence()
        s.focus(.account)
        s.pick("alpha")
        #expect(s.text(.model) == "M2 medium" && s.request?.model == nil)
    }

    @Test func aPickedModelStaysAndBecomesAFlag() {
        var s = sentence()
        s.focus(.model)
        s.pick("M2")
        s.focus(.account)
        s.pick("alpha")
        #expect(s.request?.model == "m2")
    }

    @Test func aHostWithNoSessionStartsOne() {
        var s = sentence()
        s.focus(.host)
        s.pick("Two")
        #expect(s.request?.arguments.prefix(8) == ["-S", "Two", "new", "x", "-c", "/home/b/dev/x", "--window", "💎"])
    }

    @Test func anAgentWithNoAccountCannotLaunch() {
        var s = sentence()
        s.pick("Devin")
        #expect(s.request == nil && s.problem != nil && s.options.count == 4)
    }

    @Test func arrowsStopAndTabWraps() {
        var s = sentence()
        s.step(-1)
        #expect(s.word == .agent)
        s.cycle(-1)
        #expect(s.word == .host && s.onLast)
    }

    @Test func typingFiltersAndSelects() {
        var s = sentence()
        s.focus(.account)
        s.type("alp")
        #expect(s.options.map(\.title) == ["alpha"] && s.text(.account) == "🎱 alpha")
    }

    @Test func oneCheckoutOnTwoHostsIsOneProject() {
        let a = LaunchHost(rex: "", shown: "One", path: "/home/a/dev/x", session: "x")
        let b = LaunchHost(rex: "Two", shown: "Two", path: "/home/b/dev/x", session: "x")
        let found = [(host: a, relative: "dev/x"), (host: b, relative: "dev/x"), (host: a, relative: "")]
        let projects = LaunchProject.group(found)
        #expect(projects.count == 1 && projects[0].hosts.count == 2 && projects[0].name == "x")
    }
}
