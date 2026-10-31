import Foundation
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Showing usage": the burndown of the longest live window. x runs from a window's start (0) to its
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
        let line = try #require(b.line)
        #expect(line.actual == [Point(x: 0, left: 100), Point(x: x(5), left: 80), Point(x: x(1), left: 70)])
        #expect(line.steady == [Point(x: 0, left: 100), Point(x: 1, left: 0)])
    }

    /// Rule 10: a reading lower than an earlier one in the window is stale and drawn nowhere,
    /// so the line never climbs back up. Readings are whole percents, so the line keeps the moment
    /// usage first reached each one; a repeat adds a flat shoulder that reads as a staircase.
    @Test func staleReadingsAreSkipped() throws {
        let u = usage([10080: Window(percent: 30, resetsAt: weekReset, observedAt: now - h)])
        let rs = [reading(10080, 20, resets: weekReset, hoursAgo: 5), reading(10080, 25, resets: weekReset, hoursAgo: 4),
                  reading(10080, 22, resets: weekReset, hoursAgo: 3), reading(10080, 25, resets: weekReset, hoursAgo: 2),
                  reading(10080, 30, resets: weekReset, hoursAgo: 1)]
        let line = try #require(Burndown.build(account: u, readings: rs, now: now).line)
        #expect(line.actual == [Point(x: 0, left: 100), Point(x: x(5), left: 80), Point(x: x(4), left: 75), Point(x: x(1), left: 70)])
    }

    /// The current reading is drawn even when it was observed no later than the last reading:
    /// the line must end at the percent the headline states.
    @Test func lineEndsAtTheCurrentPercent() throws {
        let u = usage([10080: Window(percent: 8, resetsAt: weekReset, observedAt: now - 2 * h)])
        let line = try #require(Burndown.build(account: u, readings: [reading(10080, 5, resets: weekReset, hoursAgo: 2)], now: now).line)
        #expect(line.actual.last?.left == 92)
    }

    /// Readings record changes, so a quiet spell has none. When the current percent was observed
    /// after the last change, the line holds flat to that observation: otherwise the area stops at
    /// the last change and leaves a gap before the projection (Viz, 2026-10-31).
    @Test func lineHoldsToTheLatestObservation() throws {
        let u = usage([10080: Window(percent: 30, resetsAt: weekReset, observedAt: now - h)])
        let line = try #require(Burndown.build(account: u, readings: [reading(10080, 30, resets: weekReset, hoursAgo: 30)], now: now).line)
        #expect(line.actual == [Point(x: 0, left: 100), Point(x: x(30), left: 70), Point(x: x(1), left: 70)])
    }

    /// No readings yet: a straight line from the start to the current reading, held to its observation.
    @Test func noReadingsDrawsAStraightLine() throws {
        let u = usage([10080: Window(percent: 40, resetsAt: weekReset, observedAt: now - 2 * h)])
        let line = try #require(Burndown.build(account: u, readings: [], now: now).line)
        #expect(line.actual == [Point(x: 0, left: 100), Point(x: x(2), left: 60)])
    }

    /// The current rate projected to the reset: 40% used with 4/7 of the week gone reaches 70% used.
    @Test func projectionRunsToTheReset() throws {
        let u = usage([10080: Window(percent: 40, resetsAt: weekReset, observedAt: now)])
        let line = try #require(Burndown.build(account: u, readings: [], now: now).line)
        let end = try #require(line.projection.last)
        #expect(line.projection.first == Point(x: x(0), left: 60))
        #expect(end.x == 1 && abs(end.left - 30) < 0.01)
        #expect(line.verdict == .onPace)
    }

    /// Codex reports only when a turn runs, so the newest observation can be hours old. The
    /// projection starts where the line ends, at that observation, so no gap opens between them;
    /// its end is still the pace at now (Viz, 2026-10-31).
    @Test func projectionStartsAtTheLastObservation() throws {
        let u = usage([10080: Window(percent: 40, resetsAt: weekReset, observedAt: now - 19 * h)])
        let line = try #require(Burndown.build(account: u, readings: [], now: now).line)
        let end = try #require(line.projection.last)
        #expect(line.projection.first == line.actual.last)
        #expect(line.projection.first == Point(x: x(19), left: 60))
        #expect(end.x == 1 && abs(end.left - 30) < 0.01)
    }

    /// Over pace, the projection stops where it reaches 0% left.
    @Test func projectionStopsAtEmpty() throws {
        let u = usage([10080: Window(percent: 80, resetsAt: weekReset, observedAt: now)])
        let line = try #require(Burndown.build(account: u, readings: [], now: now).line)
        let end = try #require(line.projection.last)
        #expect(end.left == 0 && end.x < 1)
        #expect(abs(end.x - x(0) * 100 / 80) < 0.0001)
    }

    /// An unknown window draws no line and is named; readings of other windows stay out.
    @Test func unknownWindowDrawsNoLine() {
        let u = usage([300: Window(percent: 50, resetsAt: now - 60, observedAt: now - h),
                       10080: Window(percent: 10, resetsAt: weekReset, observedAt: now)])
        let b = Burndown.build(account: u, readings: [reading(300, 50, resets: now - 60, hoursAgo: 1)], now: now)
        #expect(b.line?.minutes == 10080)
        #expect(b.unknown == [300])
    }

    /// Only the longest live window is charted, with its day grid; the five-hour window is the
    /// ring's (Viz, 2026-10-29: "I don't think we need the 5h burndown line at all").
    @Test func chartsTheLongestWindowOnly() {
        let u = usage([300: Window(percent: 50, resetsAt: now + 2 * h, observedAt: now),
                       10080: Window(percent: 10, resetsAt: weekReset, observedAt: now)])
        let b = Burndown.build(account: u, readings: [], now: now)
        #expect(b.line?.minutes == 10080)
        #expect(b.grid.count == 7)
        #expect(b.grid.filter(\.elapsed).count == 4, "four whole days of the week are gone")
        // With the week unknown, the five-hour window is the longest live one.
        let five = usage([300: Window(percent: 50, resetsAt: now + 2 * h, observedAt: now),
                          10080: Window(percent: 10, resetsAt: now - 60, observedAt: now - h)])
        #expect(Burndown.build(account: five, readings: [], now: now).line?.minutes == 300)
    }

    /// The headline speaks for the charted window.
    @Test func headline() {
        let five = Window(percent: 66, resetsAt: now + 2 * h, observedAt: now)
        let week = Window(percent: 70, resetsAt: weekReset, observedAt: now)
        #expect(Burndown.build(account: usage([300: five, 10080: week]), readings: [], now: now).headline == "30% left · 13% ahead · resets 3d")
        #expect(Burndown.build(account: usage([300: five]), readings: [], now: now).headline == "34% left · 6% ahead · resets 2h")
        let spent = Window(percent: 100, resetsAt: now + 2 * h, observedAt: now)
        #expect(Burndown.build(account: usage([300: spent]), readings: [], now: now).headline == "Budget spent · resets 2h")
        #expect(Burndown.build(account: usage([:]), readings: [], now: now).headline == "Usage unknown")
    }
}
