import Foundation
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Launch": the New Agent Session action opens `burns://new?host=H&session=S`, and the
/// panel starts on that session. Synthetic hosts and sessions.
struct LaunchTargetTests {
    func sentence() -> LaunchSentence {
        let account = LaunchAccount(id: AccountID(provider: "codex", name: "alpha"), emoji: "🎱", left: "72% left", entry: nil)
        let x = LaunchProject(name: "x", path: "dev/x", hosts: [LaunchHost(rex: "", shown: "One", path: "/home/a/dev/x", session: "x")])
        let y = LaunchProject(name: "y", path: "dev/y", hosts: [
            LaunchHost(rex: "", shown: "One", path: "/home/a/dev/y", session: nil),
            LaunchHost(rex: "Two", shown: "Two", path: "/home/b/dev/y", session: "why"),
        ])
        return LaunchSentence(accounts: [account], projects: [x, y])
    }

    @Test func theURLNamesAHostAndASession() {
        let t = LaunchTarget(url: URL(string: "burns://new?host=Two&session=my%20work")!, here: "one")
        #expect(t == LaunchTarget(host: "Two", session: "my work"))
        #expect(LaunchTarget(url: URL(string: "burns://new")!, here: "one") == nil)
        #expect(LaunchTarget(url: URL(string: "burns://new?host=Two")!, here: "one") == nil)
    }

    /// The action finds a session on the Mac's own server with no label for it, and an entry for
    /// this Mac under its tailnet name is the same machine.
    @Test func thisMacIsAnEmptyHost() {
        #expect(LaunchTarget(url: URL(string: "burns://new?session=x")!, here: "one")?.host == "")
        #expect(LaunchTarget(url: URL(string: "burns://new?host=One.tail.ts.net&session=x")!, here: "one")?.host == "")
    }

    @Test func thePanelStartsOnTheTargetSession() {
        var s = sentence()
        s.aim(LaunchTarget(host: "two", session: "why"))
        #expect(s.text(.project) == "y" && s.text(.host) == "Two")
        #expect(s.request?.host.session == "why")
    }

    /// Two sessions in one folder on one host: the target picks its own.
    @Test func theTargetPicksItsSessionAmongItsFolderSiblings() {
        var s = sentence()
        var projects = s.projects
        var other = projects[1].hosts[1]
        other.session = "why not"
        projects[1].hosts.append(other)
        s = LaunchSentence(accounts: s.accounts, projects: projects)
        s.aim(LaunchTarget(host: "Two", session: "why not"))
        #expect(s.request?.host.session == "why not")
    }

    /// The panel opens before the read that lists the session, so the target waits for it.
    @Test func aTargetWaitsForTheReadThatListsIt() {
        var s = LaunchSentence(accounts: sentence().accounts, projects: [])
        s.aim(LaunchTarget(host: "Two", session: "why"))
        s.adopt(sentence())
        #expect(s.text(.project) == "y" && s.text(.host) == "Two")
        // Once it has landed, a later read keeps it as a choice.
        s.adopt(sentence())
        #expect(s.text(.host) == "Two")
    }

    /// A target that names no listed session changes nothing, and the operator's own choice wins.
    @Test func theOperatorOverridesTheTarget() {
        var s = sentence()
        s.aim(LaunchTarget(host: "Two", session: "gone"))
        #expect(s.text(.project) == "x")
        s.focus(.project)
        s.pick("dev/x")
        s.adopt(sentence())
        #expect(s.text(.project) == "x")
    }

    /// The host list names the session a launch joins.
    @Test func theHostListNamesTheSession() {
        var s = sentence()
        s.focus(.project)
        s.pick("dev/y")
        s.focus(.host)
        #expect(s.options.map(\.note) == ["new session", "session why"])
    }

    /// `rex block call process`: the folder is the pane's own process's, which Rex knows for every
    /// pane. `pwd` answers only for a shell that reports its folder, and agent panes do not.
    @Test func aPaneFolderComesFromItsProcess() {
        let shell = #"{"child":{"pid":1,"name":"zsh","cwd":"/home/b/dev/y"},"foreground":{"pid":2,"name":"codex","cwd":"/home/b/dev/y/sub"},"home":"/home/b"}"#
        #expect(RexFolder.path(process: Data(shell.utf8)) == "/home/b/dev/y")
        let bare = #"{"child":null,"foreground":{"pid":2,"cwd":"/home/b/dev/z"}}"#
        #expect(RexFolder.path(process: Data(bare.utf8)) == "/home/b/dev/z")
        #expect(RexFolder.path(process: Data(#"{"child":null,"foreground":null}"#.utf8)) == nil)
        #expect(RexFolder.path(process: Data("not json".utf8)) == nil)
    }
}
