import CoreGraphics
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Apps and widgets": on the iPhone the usage view grows to fill the screen above the
/// host list, and never shrinks below its minimum, where the list scrolls instead.
struct ScreenFitTests {
    @Test func usageFillsWhatTheHostListLeaves() {
        // 800 pt visible, 200 pt of host list, 16 pt between them, 16 pt padding above and below.
        #expect(ScreenFit.usageHeight(visible: 800, below: 200) == CGFloat(800 - 200 - 16 - 32))
    }

    @Test func usageKeepsItsMinimumWhenTheHostListIsLong() {
        #expect(ScreenFit.usageHeight(visible: 600, below: 500) == ScreenFit.minimum)
    }

    @Test func beforeAnyMeasurementUsageHasItsMinimum() {
        #expect(ScreenFit.usageHeight(visible: 0, below: 0) == ScreenFit.minimum)
    }
}
