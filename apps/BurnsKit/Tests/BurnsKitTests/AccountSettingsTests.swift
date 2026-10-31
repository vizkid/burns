import Testing
@testable import BurnsKit

private func entry(_ provider: String, _ name: String, colour: String? = nil, emoji: String? = nil) -> CentralAccount {
    CentralAccount(provider: provider, name: name, home: ".\(provider)-\(name)", kind: "subscription", colour: colour, emoji: emoji)
}

/// docs/SPEC.md, "Accounts in the apps": colour and emoji belong to the name.
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

    @Test func theEmojiDefaultsToTheSwatchCircle() {
        let groups = AccountGroup.build([entry("claude", "alpha"), entry("claude", "beta", emoji: "🦊")])
        #expect(groups[0].emoji == "🟣")
        #expect(groups[1].emoji == "🦊")
    }

    @Test func thePickerOffersTheCirclesFirst() {
        #expect(Array(EmojiCatalogue.all.prefix(9)).map(\.emoji) == EmojiCatalogue.circles)
        #expect(EmojiCatalogue.all.count > 1000)
    }

    @Test func thePickerHoldsEveryDefault() {
        for s in Swatch.allCases { #expect(EmojiCatalogue.all.contains { $0.emoji == s.circle }) }
    }

    @Test func searchUsesUnicodeNames() {
        #expect(EmojiCatalogue.matching("fox").contains { $0.emoji == "🦊" })
        #expect(EmojiCatalogue.matching("blue circle").map(\.emoji) == ["🔵"])
        #expect(EmojiCatalogue.matching("zzzz").isEmpty)
    }

    @Test func noPickerEntryIsASkinToneOrFlagPart() {
        #expect(!EmojiCatalogue.all.contains { $0.emoji == "🏽" })
    }

    @Test func namesFollowTheHostRule() {
        #expect(AccountGroup.validName("work-2"))
        #expect(!AccountGroup.validName("Work"))
        #expect(!AccountGroup.validName(""))
    }
}
