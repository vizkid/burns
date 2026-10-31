import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Accounts in the apps": one swatch per account name, shared by its providers, and
/// a name with no colour takes the next unused swatch in list order.
struct SwatchTests {
    @Test func namesTakeTheNextUnusedSwatchInOrder() {
        #expect(Swatch.assign(["alpha", "beta"]) == ["alpha": .purple, "beta": .blue])
    }

    @Test func aNameSharesOneSwatchAcrossProviders() {
        #expect(Swatch.assign(["alpha", "alpha", "beta"]) == ["alpha": .purple, "beta": .blue])
    }

    /// More names than swatches start the list again: a repeat, not a missing colour.
    @Test func swatchesRepeatPastTheLast() {
        let names = (0..<7).map { "n\($0)" }
        #expect(Swatch.assign(names)["n6"] == .purple)
    }

    @Test func noSwatchIsAPaceColour() {
        let p = Swatch.allCases.map(\.dark)
        #expect(!p.contains(0xF5BD33) && Set(p).count == Swatch.allCases.count)
    }

    @Test func glyphsDimWhenNotSelected() {
        #expect(Swatch.opacity(selected: true) == 1)
        #expect(Swatch.opacity(selected: false) == 0.55)
    }
}
