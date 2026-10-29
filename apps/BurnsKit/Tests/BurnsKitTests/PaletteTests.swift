import SwiftUI
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Showing usage": the usage arc and the chart's line carry the pace colour; the
/// time arc stays neutral, so it never hides the usage colour.
struct PaletteTests {
    let p = BurnsPalette.standard()

    @Test func usageTakesThePaceColour() {
        #expect(p.usage(nil) == p.used)
        #expect(p.usage(.onPace) == p.used)
        #expect(p.usage(.over) == p.over)
        #expect(p.usage(.critical) == p.critical)
    }
}
