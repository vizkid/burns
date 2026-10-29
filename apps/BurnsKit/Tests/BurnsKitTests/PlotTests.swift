import CoreGraphics
import Testing
@testable import BurnsKit

/// The points the chart draws from a burndown's actual line: a monotone curve needs readings
/// spaced apart, or near-vertical steps overshoot; and the straight span before the first reading
/// must stay straight.
struct PlotTests {
    @Test func readingsCloserThanTheGapMergeIntoTheLater() {
        let actual = [Point(x: 0, left: 100), Point(x: 0.30, left: 77), Point(x: 0.301, left: 76), Point(x: 0.302, left: 75), Point(x: 0.40, left: 70)]
        let plotted = BurndownChart.plotted(actual, minGap: 0.01)
        #expect(plotted.last == Point(x: 0.40, left: 70))
        #expect(plotted.contains(Point(x: 0.302, left: 75)) && !plotted.contains(Point(x: 0.30, left: 77)))
        #expect(zip(plotted, plotted.dropFirst()).allSatisfy { $1.x - $0.x >= 0.01 - 1e-12 })
    }

    @Test func theLeadStaysStraight() {
        let actual = [Point(x: 0, left: 100), Point(x: 0.4, left: 80), Point(x: 0.5, left: 70)]
        let plotted = BurndownChart.plotted(actual, minGap: 0.01)
        let lead = plotted.filter { $0.x <= 0.4 }
        #expect(lead.count >= 4, "points along the lead hold the curve to it")
        #expect(lead.allSatisfy { abs($0.left - (100 - 50 * $0.x)) < 1e-9 })
    }

    @Test func shortLinesAreUnchanged() {
        let two = [Point(x: 0, left: 100), Point(x: 0.3, left: 60)]
        #expect(BurndownChart.plotted(two, minGap: 0.01).first == two[0])
        #expect(BurndownChart.plotted(two, minGap: 0.01).last == two[1])
        #expect(BurndownChart.plotted([], minGap: 0.01).isEmpty)
    }

    /// A dash pattern started part-way along the steady line continues its dashes in step.
    @Test func dashPhaseContinuesThePattern() {
        let phase = BurndownChart.dashPhase(origin: .zero, start: CGPoint(x: 6, y: 8), period: 8)
        #expect(abs(phase - 2) < 1e-9, "10 points along an 8-point pattern is 2 into it")
        #expect(BurndownChart.dashPhase(origin: .zero, start: CGPoint(x: 3, y: 4), period: 0) == 0)
    }
}
