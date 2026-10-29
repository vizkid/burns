import Foundation
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Showing usage": the combined burndown. x runs from a window's start (0) to its
/// reset (1); y is percent left.
struct BurndownTests {
    let now = Fixture.now, h = Fixture.h
    let id = AccountID(provider: "claude", name: "alpha")
    /// Seven-day window resetting in 72 hours: it started 96 hours ago.
    var weekReset: Date { now + 72 * h }
    func x(_ hoursAgo: Double) -> Double { (96 - hoursAgo) / 168 }

    func reading(_ minutes: Int, _ pct: Double, resets: Date, hoursAgo: Double) -> Reading {
        Reading(provider: "claude", account: "alpha", limitId: "claude", minutes: minutes, percent: pct, resetsAt: resets, observedAt: now - hoursAgo * h)
    }

    func usage(_ windows: [Int: Window]) -> AccountUsage { AccountUsage(id: id, kind: "subscription", windows: windows) }

    /// The line starts at 100% at the window's start and runs straight from reading to reading.
    /// Readings are samples of usage that grew between them, so no flat hold and cliff
    /// ("a gap with no readings draws no line, never a flat one").
    @Test func actualLineJoinsReadings() throws {
        let u = usage([10080: Window(percent: 30, resetsAt: weekReset, observedAt: now - h)])
        let b = Burndown.build(account: u, readings: [reading(10080, 20, resets: weekReset, hoursAgo: 5), reading(10080, 30, resets: weekReset, hoursAgo: 1)], now: now)
        let line = try #require(b.lines.first)
        #expect(line.actual == [Point(x: 0, left: 100), Point(x: x(5), left: 80), Point(x: x(1), left: 70)])
        #expect(line.steady == [Point(x: 0, left: 100), Point(x: 1, left: 0)])
    }

    /// Rule 10: a reading lower than an earlier one in the window is stale and drawn nowhere,
    /// so the line never climbs back up.
    @Test func staleReadingsAreSkipped() throws {
        let u = usage([10080: Window(percent: 30, resetsAt: weekReset, observedAt: now - h)])
        let rs = [reading(10080, 20, resets: weekReset, hoursAgo: 5), reading(10080, 25, resets: weekReset, hoursAgo: 4),
                  reading(10080, 22, resets: weekReset, hoursAgo: 3), reading(10080, 25, resets: weekReset, hoursAgo: 2),
                  reading(10080, 30, resets: weekReset, hoursAgo: 1)]
        let line = try #require(Burndown.build(account: u, readings: rs, now: now).lines.first)
        // The later 25 stays: usage was still flat two hours ago.
        #expect(line.actual == [Point(x: 0, left: 100), Point(x: x(5), left: 80), Point(x: x(4), left: 75), Point(x: x(2), left: 75), Point(x: x(1), left: 70)])
    }

    /// No readings yet: a straight line from the start to the current reading, held to its observation.
    @Test func noReadingsDrawsAStraightLine() throws {
        let u = usage([10080: Window(percent: 40, resetsAt: weekReset, observedAt: now - 2 * h)])
        let line = try #require(Burndown.build(account: u, readings: [], now: now).lines.first)
        #expect(line.actual == [Point(x: 0, left: 100), Point(x: x(2), left: 60)])
    }

    /// The current rate projected to the reset: 40% used with 4/7 of the week gone reaches 70% used.
    @Test func projectionRunsToTheReset() throws {
        let u = usage([10080: Window(percent: 40, resetsAt: weekReset, observedAt: now)])
        let line = try #require(Burndown.build(account: u, readings: [], now: now).lines.first)
        let end = try #require(line.projection.last)
        #expect(line.projection.first == Point(x: x(0), left: 60))
        #expect(end.x == 1 && abs(end.left - 30) < 0.01)
        #expect(line.verdict == .onPace)
    }

    /// Over pace, the projection stops where it reaches 0% left.
    @Test func projectionStopsAtEmpty() throws {
        let u = usage([10080: Window(percent: 80, resetsAt: weekReset, observedAt: now)])
        let line = try #require(Burndown.build(account: u, readings: [], now: now).lines.first)
        let end = try #require(line.projection.last)
        #expect(end.left == 0 && end.x < 1)
        #expect(abs(end.x - x(0) * 100 / 80) < 0.0001)
    }

    /// An unknown window draws no line and is named; readings of other windows stay out.
    @Test func unknownWindowDrawsNoLine() {
        let u = usage([300: Window(percent: 50, resetsAt: now - 60, observedAt: now - h),
                       10080: Window(percent: 10, resetsAt: weekReset, observedAt: now)])
        let b = Burndown.build(account: u, readings: [reading(300, 50, resets: now - 60, hoursAgo: 1)], now: now)
        #expect(b.lines.map(\.minutes) == [10080])
        #expect(b.unknown == [300])
    }

    /// Both windows on one axis, seven-day first (it is drawn heavier), with a day grid.
    @Test func combinedWindowsAndGrid() {
        let u = usage([300: Window(percent: 50, resetsAt: now + 2 * h, observedAt: now),
                       10080: Window(percent: 10, resetsAt: weekReset, observedAt: now)])
        let b = Burndown.build(account: u, readings: [], now: now)
        #expect(b.lines.map(\.minutes) == [10080, 300])
        #expect(b.grid.count == 7)
        #expect(b.grid.filter(\.elapsed).count == 4, "four whole days of the week are gone")
    }

    /// The headline speaks for the window with the least left.
    @Test func headline() {
        let five = Window(percent: 66, resetsAt: now + 2 * h, observedAt: now)
        let week = Window(percent: 10, resetsAt: weekReset, observedAt: now)
        #expect(Burndown.build(account: usage([300: five, 10080: week]), readings: [], now: now).headline == "34% left · 6% ahead · resets 2h")
        let spent = Window(percent: 100, resetsAt: now + 2 * h, observedAt: now)
        #expect(Burndown.build(account: usage([300: spent]), readings: [], now: now).headline == "Budget spent · resets 2h")
        #expect(Burndown.build(account: usage([:]), readings: [], now: now).headline == "Usage unknown")
    }
}
