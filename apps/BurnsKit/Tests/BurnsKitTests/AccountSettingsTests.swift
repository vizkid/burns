import Testing
#if os(macOS)
import AppKit
#endif
@testable import BurnsKit

private func entry(_ provider: String, _ name: String, colour: String? = nil, emoji: String? = nil) -> CentralAccount {
    CentralAccount(provider: provider, name: name, home: ".\(provider)-\(name)", kind: "subscription", colour: colour, emoji: emoji)
}

/// docs/SPEC.md, "Accounts in the apps": colour and icon belong to the name.
struct AccountSettingsTests {
    @Test func aNameGroupsItsProviders() {
        let groups = AccountGroup.build([entry("claude", "alpha"), entry("claude", "beta"), entry("codex", "alpha")])
        #expect(groups.map(\.name) == ["alpha", "beta"])
        #expect(groups[0].entries.count == 2)
    }

    @Test func aDefaultSwatchSkipsOnesAlreadyChosen() {
        let groups = AccountGroup.build([entry("claude", "alpha"), entry("claude", "beta", colour: "purple")])
        #expect(groups[0].swatch == .blue)
        #expect(groups[1].swatch == .purple)
    }

    /// docs/SPEC.md, "Account icons in the apps".
    @Test func anAccountWithNoIconShowsAFilledCircle() {
        let groups = AccountGroup.build([entry("claude", "alpha"), entry("claude", "beta", emoji: "🦊")])
        #expect(groups[0].emoji == "●" && groups[0].symbol == "circle.fill")
        #expect(groups[1].emoji == "🦊" && groups[1].symbol == nil)
    }

    @Test func aPickedSymbolKeepsItsGlyph() {
        var a = entry("claude", "alpha")
        a.symbol = "diamond.fill"; a.emoji = "◆"
        let group = AccountGroup.build([a])[0]
        #expect(group.symbol == "diamond.fill" && group.emoji == "◆")
    }

    @Test func picking() {
        let diamond = SymbolCatalogue.all.first { $0.symbol == "diamond.fill" }!
        var a = entry("claude", "alpha", emoji: "🦊")
        SymbolCatalogue.apply(diamond, to: &a)
        #expect(a.symbol == "diamond.fill" && a.emoji == "◆")
        SymbolCatalogue.apply(nil, to: &a)
        #expect(a.symbol == nil && a.emoji == nil)
    }

    /// A glyph with emoji presentation would bring the emoji back into Rex.
    @Test func everyGlyphIsOneTextCharacter() {
        for item in SymbolCatalogue.all {
            #expect(item.glyph.count == 1 && item.glyph.unicodeScalars.count == 1, "\(item.symbol)")
            #expect(!item.glyph.unicodeScalars.first!.properties.isEmoji, "\(item.symbol)")
        }
        #expect(Set(SymbolCatalogue.all.map(\.symbol)).count == SymbolCatalogue.all.count)
        #expect(Set(SymbolCatalogue.all.map(\.glyph)).count == SymbolCatalogue.all.count)
    }

    #if os(macOS)
    @Test func everySymbolExists() {
        for item in SymbolCatalogue.all {
            #expect(NSImage(systemSymbolName: item.symbol, accessibilityDescription: nil) != nil, "\(item.symbol)")
        }
    }
    #endif

    @Test func theSymbolRoundTrips() throws {
        var a = entry("claude", "alpha")
        a.symbol = "triangle.fill"
        let back = try JSONDecoder().decode(CentralAccount.self, from: JSONEncoder().encode(a))
        #expect(back.symbol == "triangle.fill" && back.extra["symbol"] == nil)
    }

    @Test func namesFollowTheHostRule() {
        #expect(AccountGroup.validName("work-2"))
        #expect(!AccountGroup.validName("Work"))
        #expect(!AccountGroup.validName(""))
    }
}
