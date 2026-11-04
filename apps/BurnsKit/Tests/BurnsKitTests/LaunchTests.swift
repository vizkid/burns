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

    @Test func aLateReadFillsAnEmptySentence() {
        var s = LaunchSentence(accounts: sentence().accounts, projects: [])
        #expect(s.text(.project) == "no project" && s.request == nil)
        s.adopt(sentence())
        #expect(s.text(.project) == "x" && s.text(.host) == "One" && s.request != nil)
    }

    /// F10: the read the panel opened from can be hours old.
    @Test func aLateReadMovesAnUntouchedDefault() {
        let claudeFirst = LaunchSentence(accounts: Array(sentence().accounts.reversed()), projects: [])
        var none = LaunchSentence(accounts: [], projects: [])
        none.adopt(claudeFirst)
        #expect(none.text(.agent) == "Claude")
        var codex = sentence()
        codex.adopt(claudeFirst)
        #expect(codex.text(.agent) == "Claude" && codex.text(.account) == "🎱 alpha")
    }

    @Test func aLateReadKeepsATouchedAgent() {
        var s = sentence()
        s.pick("Codex")
        s.adopt(LaunchSentence(accounts: Array(sentence().accounts.reversed()), projects: []))
        #expect(s.text(.agent) == "Codex")
    }

    /// Return on a word takes it, even with no change.
    @Test func aConfirmedWordIsKept() {
        var s = sentence()
        s.confirm()
        #expect(s.word == .model)
        s.adopt(LaunchSentence(accounts: Array(sentence().accounts.reversed()), projects: []))
        #expect(s.text(.agent) == "Codex")
    }

    @Test func aLateReadMovesAnUntouchedProject() {
        var s = sentence()
        let y = LaunchHost(rex: "", shown: "One", path: "/home/a/dev/y", session: "y")
        s.adopt(LaunchSentence(accounts: s.accounts, projects: [LaunchProject(name: "y", path: "dev/y", hosts: [y])] + s.projects))
        #expect(s.text(.project) == "y")
    }

    /// A model comes from the account's catalog, so choosing one holds the account.
    @Test func aChosenModelHoldsItsAccount() {
        var s = sentence()
        s.focus(.model); s.pick("m2")
        s.adopt(LaunchSentence(accounts: Array(sentence().accounts.prefix(2).reversed()), projects: sentence().projects))
        #expect(s.text(.account) == "💎 beta" && s.request?.model == "m2")
    }

    /// F35.
    @Test func escStepsBackThenCloses() {
        var s = sentence()
        s.focus(.account)
        let first = s.back(), firstWord = s.word
        let second = s.back(), secondWord = s.word
        let last = s.back()
        #expect(first && firstWord == .model && second && secondWord == .agent && !last)
    }

    @Test func theNoteSaysWhyTheDefaultWon() {
        var s = LaunchSentence(accounts: sentence().accounts, projects: [], reason: "Codex beta: 96% left", unreachable: ["two: no Rex entry"])
        #expect(s.note == "Codex beta: 96% left · Not offered: two: no Rex entry")
        s.focus(.account); s.pick("alpha")
        #expect(s.note == "Not offered: two: no Rex entry")
    }

    @Test func aLateReadKeepsEveryChoice() {
        var s = sentence()
        s.focus(.account); s.pick("alpha")
        s.focus(.model); s.pick("m1")
        s.focus(.host); s.pickHost("Two")
        var fresh = sentence()
        fresh = LaunchSentence(accounts: Array(fresh.accounts.reversed()), projects: [LaunchProject(name: "y", path: "dev/y", hosts: [])] + fresh.projects)
        s.adopt(fresh)
        #expect(s.word == .host)
        #expect(s.text(.account) == "🎱 alpha" && s.text(.project) == "x" && s.text(.host) == "Two")
        #expect(s.request?.model == "m1")
    }

    @Test func aLateReadDropsAChoiceThatIsGone() {
        var s = sentence()
        s.focus(.account); s.pick("alpha")
        s.adopt(LaunchSentence(accounts: sentence().accounts.filter { $0.id.name != "alpha" }, projects: sentence().projects))
        #expect(s.text(.account) == "💎 beta")
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
        s.pick("m2")
        s.focus(.account)
        s.pick("alpha")
        #expect(s.request?.model == "m2")
    }

    /// A default is a fact of the host's home, so the model word follows the host.
    @Test func theModelFollowsTheHost() {
        var s = sentence()
        let id = AccountID(provider: "codex", name: "beta")
        let hosts = [LaunchHost(rex: "", shown: "One", path: "/p", session: "x", entries: [id: entry("codex", "beta", "m1", "low", nil)]),
                     LaunchHost(rex: "Two", shown: "Two", path: "/p", session: "x", entries: [id: entry("codex", "beta", "m2", "high", nil)])]
        s = LaunchSentence(accounts: s.accounts, projects: [LaunchProject(name: "x", path: "dev/x", hosts: hosts)])
        #expect(s.text(.model) == "M1 low")
        s.focus(.host)
        s.pickHost("Two")
        #expect(s.text(.model) == "M2 high" && s.request?.model == nil)
    }

    @Test func aHostWithNoSessionStartsOne() {
        var s = sentence()
        s.focus(.host)
        s.pickHost("Two")
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
        let found = [(host: a, relative: "dev/x"), (host: b, relative: "dev/x"), (host: a, relative: ""), (host: a, relative: "dev/x")]
        let projects = LaunchProject.group(found)
        #expect(projects.count == 1 && projects[0].hosts.count == 2 && projects[0].name == "x")
    }

    /// Several sessions often share one checkout on one host. Each is its own row, so the operator
    /// can choose which session the tab joins.
    @Test func everySessionInAFolderIsOffered() {
        let a = LaunchHost(rex: "Two", shown: "Two", path: "/home/b/dev/x", session: "x")
        var b = a
        b.session = "x review"
        let projects = LaunchProject.group([(host: a, relative: "dev/x"), (host: b, relative: "dev/x")])
        var s = LaunchSentence(accounts: sentence().accounts, projects: projects)
        s.focus(.host)
        #expect(s.options.map(\.note) == ["session x", "session x review"])
        s.pick(s.options[1].id)
        #expect(s.text(.host) == "Two" && s.request?.host.session == "x review")
        s.adopt(LaunchSentence(accounts: s.accounts, projects: projects))
        #expect(s.request?.host.session == "x review")
    }
}

extension LaunchSentence {
    /// Picks the first host row shown with this name.
    mutating func pickHost(_ shown: String) {
        if let o = options.first(where: { $0.title == shown }) { pick(o.id) }
    }
}
