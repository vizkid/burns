#if os(macOS)
import AppKit
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Icons": the menu bar shows Burns' own flame, traced from the app icon's flame,
/// as a template image so macOS draws it in the menu bar's colour.
struct FlameTests {
    @Test func flameIsATemplateImageForTheMenuBar() throws {
        let flame = try #require(BurnsFlame.menuBarImage())
        #expect(flame.isTemplate)
        #expect(flame.size.height == BurnsFlame.menuBarHeight)
        #expect(flame.accessibilityDescription == "Burns")
    }

    @Test func tintedFlameKeepsItsSizeAndIsNotATemplate() throws {
        let flame = try #require(BurnsFlame.menuBarImage(tint: .systemRed))
        #expect(!flame.isTemplate)
        #expect(flame.size == BurnsFlame.menuBarImage()?.size)
    }
}
#endif
