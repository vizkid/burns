import Foundation
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Beside, typed folders and new sessions". Synthetic hosts, sessions and folders.
struct LaunchPlacesTests {
    let smithers = ["--", "smithers", "launch", "--provider", "codex", "--account", "alpha"]
    let pane = LaunchPane(rex: "Two", shown: "Two", session: "why", block: "block:b", path: "/home/b/dev/y/sub", relative: "dev/y/sub")

    func sentence() -> LaunchSentence {
        let account = LaunchAccount(id: AccountID(provider: "codex", name: "alpha"), emoji: "🎱", left: "72% left", entry: nil)
        let x = LaunchProject(name: "x", path: "dev/x", hosts: [LaunchHost(rex: "", shown: "One", path: "/home/a/dev/x", session: "x")])
        let y = LaunchProject(name: "y", path: "dev/y", hosts: [
            LaunchHost(rex: "", shown: "One", path: "/home/a/dev/y", session: nil),
            LaunchHost(rex: "Two", shown: "Two", path: "/home/b/dev/y", session: "why"),
        ])
        return LaunchSentence(accounts: [account], projects: [x, y])
    }

    /// The pane's folder is known only after a read, so the beside target waits for it.
    @Test func theBesideKeyStartsBesideThePane() {
        var s = sentence()
        s.aim(LaunchTarget(host: "Two", session: "why", block: "block:b"))
        var fresh = sentence()
        fresh.pane = pane
        s.adopt(fresh)
        #expect(s.text(.project) == "sub" && s.text(.host) == "Two beside this pane")
        #expect(s.request?.arguments == ["-S", "Two", "split", "-s", "why", "-b", "block:b", "-c", "/home/b/dev/y/sub", "--label", "🎱"] + smithers)
    }

    @Test func theURLNamesTheBlock() {
        let t = LaunchTarget(url: URL(string: "burns://new?host=Two&session=why&block=block:b")!, here: "one")
        #expect(t == LaunchTarget(host: "Two", session: "why", block: "block:b"))
    }

    /// Another folder on the pane's host can still go beside it. A folder only on other hosts cannot.
    @Test func besideStaysOnTheHostsFolders() {
        var s = sentence()
        s.pane = pane
        s.focus(.project); s.pick("dev/y"); s.focus(.host)
        #expect(s.options.first?.note == "beside this pane")
        s.pick(s.options[0].id)
        #expect(s.request?.arguments.prefix(9) == ["-S", "Two", "split", "-s", "why", "-b", "block:b", "-c", "/home/b/dev/y"])
        s.focus(.project); s.pick("dev/x"); s.focus(.host)
        #expect(!s.options.contains { $0.note == "beside this pane" })
    }

    @Test func anotherRowLeavesBeside() {
        var s = sentence()
        s.pane = pane
        s.focus(.project); s.pick("dev/y"); s.focus(.host)
        s.pick(s.options.first { $0.note == "session why" }!.id)
        #expect(s.request?.arguments.prefix(5) == ["-S", "Two", "window", "new", "-s"])
    }

    /// A typed folder lists what each host's smithersd found, and starts a session there.
    @Test func aTypedFolderOffersWhatTheHostsHave() {
        var s = sentence()
        s.focus(.project)
        s.type("dev/n")
        #expect(s.completing == "dev/n")
        let new = LaunchProject(name: "new", path: "dev/new", hosts: [LaunchHost(rex: "Two", shown: "Two", path: "/home/b/dev/new", session: nil)])
        s.found([new], for: "dev/other")
        #expect(!s.options.contains { $0.id == "dev/new" })
        s.found([new], for: "dev/n")
        #expect(s.options.map(\.id) == ["dev/new"] && s.selected == "dev/new")
        s.confirm()
        #expect(s.word == .host && s.text(.project) == "new")
        #expect(s.request?.arguments.prefix(8) == ["-S", "Two", "new", "new", "-c", "/home/b/dev/new", "--window", "🎱"])
    }

    @Test func homePrefixesAreStripped() {
        var s = sentence()
        s.focus(.project)
        s.type("~/dev/n")
        #expect(s.completing == "dev/n")
        s.erase(); s.erase(); s.erase(); s.erase(); s.erase(); s.erase(); s.erase()
        s.type("/Users/a/dev/n")
        #expect(s.completing == "dev/n")
        while !s.query.isEmpty { s.erase() }
        s.type("/etc")
        #expect(s.completing == nil)
        s.erase(); s.erase(); s.erase(); s.erase()
        s.focus(.host)
        s.type("de")
        #expect(s.completing == nil)
    }

    /// Tab completes the selected folder, so the next part can be typed.
    @Test func tabCompletesAFolder() {
        var s = sentence()
        s.focus(.project)
        s.type("dev/n")
        s.found([LaunchProject(name: "new", path: "dev/new", hosts: [])], for: "dev/n")
        let done = s.complete()
        #expect(done && s.query == "dev/new/")
        s.focus(.agent)
        let elsewhere = s.complete()
        #expect(!elsewhere)
    }

    /// A typed name that no session on the host has starts one with that name.
    @Test func aTypedSessionStartsOne() {
        var s = sentence()
        s.focus(.host)
        s.type("Fresh")
        #expect(s.query == "Fresh")
        let make = s.options.first { $0.note == "new session Fresh" }
        #expect(make?.title == "One")
        s.pick(make!.id)
        s.confirm()
        #expect(s.text(.host) == "One")
        #expect(s.request?.arguments.prefix(7) == ["new", "Fresh", "-c", "/home/a/dev/x", "--window", "🎱", "--"])
        s.focus(.host)
        s.type("x")
        #expect(!s.options.contains { $0.note == "new session x" })
    }
}
