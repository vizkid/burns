import Foundation
import SwiftUI
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Live pane shares", with synthetic accounts.
struct PaneSharesTests {
    let alpha = AccountID(provider: "claude", name: "alpha")

    func row(_ block: String, _ model: String, _ tokens: Int64, account: String = "alpha", main: Int64? = nil, agents: Int? = nil) -> WirePane {
        WirePane(provider: "claude", account: account, block: block, model: model, tokens: tokens, last: Date(timeIntervalSince1970: 0), mainTokens: main, agents: agents)
    }
    func host(_ name: String, _ rows: [WirePane], live: [String: LivePane]?) -> HostPanes {
        HostPanes(host: name, icon: "macstudio", rows: rows, live: live)
    }

    @Test func modelNamesReadAsFamilyAndVersion() {
        #expect(PaneShares.modelName("claude-opus-5-5") == "Opus 5.5")
        #expect(PaneShares.modelName("claude-haiku-5-5-20261001") == "Haiku 5.5")
        #expect(PaneShares.modelName("gpt-6.1-sol") == "Sol 6.1")
        #expect(PaneShares.modelName("gpt-6-astra") == "Astra 6")
        #expect(PaneShares.modelName("mystery") == "mystery")
    }

    @Test func aPaneShowsOnceUnderItsMainModelWithItsWholeShare() {
        let live = ["b1": LivePane(session: "work", label: "cap"), "b2": LivePane(session: "work", label: "shell")]
        let s = PaneShares.build([host("One", [row("b1", "claude-opus-5-5", 60), row("b1", "claude-haiku-5-5", 15), row("b2", "claude-opus-5-5", 25)], live: live)], account: alpha)
        #expect(s.panes.map(\.title) == ["work · cap", "work"])
        #expect(s.panes[0].model == "Opus 5.5")
        #expect(s.panes[0].tokens == 75)
        #expect(s.panes[0].share == 0.75)
        #expect(s.panes[1].share == 0.25)
        #expect(s.unread.isEmpty)
    }

    @Test func subagentsCountInTheShareButDoNotNameThePane() {
        let live = ["b1": LivePane(session: "work", label: "research")]
        let rows = [row("b1", "claude-haiku-5-5", 750, main: 0), row("b1", "claude-fable-5-1", 140, main: 15)]
        let s = PaneShares.build([host("One", rows, live: live)], account: alpha)
        #expect(s.panes[0].model == "Fable 5.1")
        #expect(s.panes[0].tokens == 890)
    }

    @Test func aPaneNamesItsSubagentsAndWhatTheyUsed() {
        let live = ["b1": LivePane(session: "work", label: "research")]
        let rows = [row("b1", "claude-haiku-5-5", 7_800_000, main: 0, agents: 7), row("b1", "claude-fable-5-1", 1_500_000, main: 160_000, agents: 7)]
        let p = PaneShares.build([host("One", rows, live: live)], account: alpha).panes[0]
        #expect(p.agents == 7)
        #expect(p.modelLine == "Fable 5.1 +7 agents")
        #expect(p.subagentModels == "Subagents: Haiku 5.5 7.8M, Fable 5.1 1.3M")
    }

    @Test func oneSubagentReadsSingularAndNoneAddsNothing() {
        let live = ["b1": LivePane(session: "work", label: "a"), "b2": LivePane(session: "work", label: "b")]
        let rows = [row("b1", "claude-opus-5-5", 100, main: 60, agents: 1), row("b2", "claude-opus-5-5", 50, main: 50, agents: 0)]
        let s = PaneShares.build([host("One", rows, live: live)], account: alpha).panes
        #expect(s[0].modelLine == "Opus 5.5 +1 agent")
        #expect(s[1].modelLine == "Opus 5.5")
        #expect(s[1].subagentModels == nil)
    }

    @Test func anOlderHostFallsBackToTheModelWithTheMostTokens() {
        let live = ["b1": LivePane(session: "work", label: "cap")]
        let s = PaneShares.build([host("One", [row("b1", "claude-haiku-5-5", 7), row("b1", "claude-opus-5-5", 9)], live: live)], account: alpha)
        #expect(s.panes[0].model == "Opus 5.5")
    }

    @Test func aBlockRexNoLongerListsIsNotLive() {
        let s = PaneShares.build([host("One", [row("gone", "claude-opus-5-5", 90), row("b1", "claude-opus-5-5", 10)], live: ["b1": LivePane(session: "work", label: "")])], account: alpha)
        #expect(s.panes.map(\.title) == ["work"])
        #expect(s.panes[0].share == 1)
    }

    @Test func otherAccountsAreLeftOutAndHostsMerge() {
        let one = host("One", [row("b1", "claude-opus-5-5", 30), row("b9", "claude-opus-5-5", 500, account: "beta")], live: ["b1": LivePane(session: "a", label: "x"), "b9": LivePane(session: "a", label: "y")])
        let two = host("Two", [row("b1", "claude-opus-5-5", 10)], live: ["b1": LivePane(session: "b", label: "x")])
        let s = PaneShares.build([one, two], account: alpha)
        #expect(s.panes.map(\.host) == ["One", "Two"])
        #expect(s.panes.map(\.share) == [0.75, 0.25])
    }

    @Test func aHostNotReadIsNamedAndNeverZero() {
        let s = PaneShares.build([host("One", [row("b1", "claude-opus-5-5", 10)], live: nil), HostPanes(host: "Two", icon: "pc", rows: nil, live: [:])], account: alpha)
        #expect(s.panes.isEmpty)
        #expect(s.unread == ["One", "Two"])
    }

    @Test func segmentWidthsFillTheBarWithEveryMinimumKept() {
        let w = PaneShares.widths([0.996, 0.001, 0.001, 0.001, 0.001], room: 350, minimum: 3)
        #expect(abs(w.reduce(0, +) - 350) < 0.001)
        #expect(w.allSatisfy { $0 >= 3 })
        #expect(PaneShares.widths([0.5, 0.5], room: 4, minimum: 3) == [2, 2])
    }

    @Test func tintStepsFadeByRankAndStopAfterFive() {
        #expect(PaneShares.strength(rank: 0) == 1)
        #expect(PaneShares.strength(rank: 4)! < PaneShares.strength(rank: 3)!)
        #expect(PaneShares.strength(rank: 5) == nil)
    }

    @Test func aHostsIconComesFromAnyOfItsRexEntries() {
        let rex = RexHosts(entries: [.init(label: "One"), .init(label: "one.tailnet.ts.net", icon: "server.rack"), .init(label: "Two", icon: "macstudio"), .init(label: "Three")])
        #expect(rex.icon(for: "one") == "server.rack")
        #expect(rex.icon(for: "Two") == "macstudio")
        #expect(rex.icon(for: "Three") == nil)
        #expect(rex.icon(for: "Four") == nil)
    }

    @Test func percentsNeverRoundDownToNothing() {
        #expect(PaneShares.percent(0.004) == "<1%")
        #expect(PaneShares.percent(0.494) == "49%")
    }

    @Test func aConfirmedAnswerNamesItsPanes() {
        let a = PanesAnswer(host: "One", checked: true, panes: [WirePane(provider: "claude", account: "alpha", block: "b1", model: "claude-opus-5-5", tokens: 5, last: Date(timeIntervalSince1970: 0), session: "work", label: "cap")])
        let s = PaneShares.build([HostPanes(host: "One", icon: "pc", answer: a)], account: alpha)
        #expect(s.panes.map(\.title) == ["work · cap"])
    }

    @Test func anUnconfirmedAnswerIsUnknownNotEmpty() {
        for checked in [false, nil] as [Bool?] {
            let a = PanesAnswer(host: "One", checked: checked, panes: [row("b1", "claude-opus-5-5", 5)])
            let s = PaneShares.build([HostPanes(host: "One", icon: "pc", answer: a)], account: alpha)
            #expect(s.panes.isEmpty && s.unread == ["One"])
        }
    }

    @Test func wireAnswerDecodes() throws {
        let json = #"{"host":"h","panes":[{"provider":"claude","account":"alpha","block":"block:1","model":"claude-opus-5-5","tokens":12,"last":"2026-11-01T10:25:16.044Z"}]}"#
        let a = try Wire.decoder.decode(PanesAnswer.self, from: Data(json.utf8))
        #expect(a.panes[0].tokens == 12)
    }
}

/// docs/SPEC.md, "Showing usage": the rings keep the pace visible when usage passes it.
struct RingPaceTests {
    @Test func theTimeArcStepsDownFromTheUsageColour() {
        let p = BurnsPalette.standard()
        #expect(p.timeOverUsage(.critical) == p.over)
        #expect(p.timeOverUsage(.over) == p.used)
        #expect(p.timeOverUsage(.onPace) == p.time)
    }

    @Test func timeDrawsOverUsageOnlyPastPace() {
        #expect(SpendRings<EmptyView>.timeOnTop(fill: 0.6, elapsed: 0.25) == 0.25)
        #expect(SpendRings<EmptyView>.timeOnTop(fill: 0.2, elapsed: 0.25) == nil)
        #expect(SpendRings<EmptyView>.timeOnTop(fill: 0.6, elapsed: nil) == nil)
        #expect(SpendRings<EmptyView>.timeOnTop(fill: 0.6, elapsed: 0) == nil)
    }
}
