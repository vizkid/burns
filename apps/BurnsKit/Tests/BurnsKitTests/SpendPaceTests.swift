import Foundation
import Testing
@testable import BurnsKit

/// COMMANDER_SPEC report section 6, "Pace": whether a quota window is being used faster than it
/// refills. The operator, 2026-09-30: "The spend bars don't tell me if I'm running over or under the pace".
struct SpendPaceTests {
    @Test func aRoundedZeroAheadKeepsTheOverPaceWarning() throws {
        let p = try #require(five(60.1, elapsed: 3))
        #expect(p.verdict == .over)
        #expect(p.aheadPoints == 0)
        #expect(p.words() == "On the edge")
        #expect(p.short(calendar: Self.utc, now: Self.now) == "On the edge · resets 2h")
    }

    @Test func veryLargeFiniteUsageReportsOutWithoutIntegerOverflow() throws {
        let p = try #require(five(Double.greatestFiniteMagnitude, elapsed: 3))
        #expect(p.words() == "out")
        #expect(p.aheadPoints == nil)
    }

    /// 12:00 UTC on a Wednesday.
    static let now = Date(timeIntervalSince1970: 1_790_769_600)
    static let h = 3600.0
    static var utc: Calendar { var c = Calendar(identifier: .gregorian); c.timeZone = TimeZone(identifier: "UTC")!; return c }

    /// A five-hour window `elapsed` hours in, with `used` percent.
    func five(_ used: Double?, elapsed: Double) -> SpendPace? {
        SpendPace(usedPercent: used, resetsAt: Self.now.addingTimeInterval((5 - elapsed) * Self.h), length: SpendPace.fiveHours, now: Self.now)
    }

    @Test func underPaceFinishesTheWindowBelowTheLimit() throws {
        // Half the window gone, 31% used: 62% at the reset.
        let p = try #require(five(31, elapsed: 2.5))
        #expect(p.elapsed == 0.5 && p.verdict == .onPace)
        #expect(p.projectedPercent == 62 && p.runsOutAt == nil)
        #expect(p.short(calendar: Self.utc, now: Self.now) == nil)   // words only when it matters
        #expect(p.sentence(calendar: Self.utc, now: Self.now) == "At this pace you finish the window at 62% when it resets in 2h 30m")
    }

    /// Exactly on pace is not over it.
    @Test func onPaceIsNeutral() throws {
        let p = try #require(five(50, elapsed: 2.5))
        #expect(p.verdict == .onPace && p.projectedPercent == 100 && p.runsOutAt == nil)
    }

    /// 100% projected before the reset, with a quarter of the window or less left by then: amber.
    @Test func overPaceLateInTheWindowIsAmber() throws {
        // 3h in, 66% used: 100% at 3h/0.66 = 4h32m, 27m before the reset.
        let p = try #require(five(66, elapsed: 3))
        #expect(p.verdict == .over)
        let out = try #require(p.runsOutAt)
        #expect(abs(out.timeIntervalSince(Self.now) - (3 / 0.66 - 3) * Self.h) < 1)
        // Ahead is points above an even pace: 66 used, 60 elapsed.
        #expect(p.aheadPoints == 6)
        #expect(p.short(calendar: Self.utc, now: Self.now) == "6% ahead · resets 2h")
        #expect(p.words() == "6% ahead")
        #expect(p.sentence(calendar: Self.utc, now: Self.now) == "At this pace you reach 100% at 13:32, 27m before the reset in 2h")
    }

    /// 100% projected with more than a quarter of the window left: red, and the words name the time.
    @Test func farOverPaceIsRedAndNamesTheTime() throws {
        // 1h in, 50% used: 100% at 2h, 3h before the reset.
        let p = try #require(five(50, elapsed: 1))
        #expect(p.verdict == .critical && abs(p.projectedPercent! - 250) < 1e-6)
        #expect(p.short(calendar: Self.utc, now: Self.now) == "30% ahead · resets 4h")
        #expect(p.sentence(calendar: Self.utc, now: Self.now) == "At this pace you reach 100% at 13:00, 3h before the reset in 4h")
    }

    /// A seven-day window runs out on another day: the words name the day too.
    @Test func aSevenDayWindowNamesTheDay() throws {
        // 2 days in, 60% used: 100% at 3d8h, so 1d8h from now; 3d16h before the reset.
        let p = try #require(SpendPace(usedPercent: 60, resetsAt: Self.now.addingTimeInterval(5 * 24 * Self.h), length: SpendPace.sevenDays, now: Self.now))
        #expect(p.verdict == .critical)
        // 60 used, 2 of 7 days gone (28.6): 31 points ahead.
        #expect(p.short(calendar: Self.utc, now: Self.now) == "31% ahead · resets 5d")
        #expect(p.sentence(calendar: Self.utc, now: Self.now) == "At this pace you reach 100% at Thu 20:00, 3d 16h before the reset in 5d")
    }

    /// Already at or past 100%: red whatever the clock says, the first minutes included.
    @Test func reachedIsRed() throws {
        // "Out" is the fact here, so it stays, and always with the reset countdown.
        for (used, elapsed, reset) in [(100.0, 4.0, "1h"), (137.0, 2.0, "3h"), (100.0, 0.05, "4h 57m")] {
            let p = try #require(five(used, elapsed: elapsed))
            #expect(p.verdict == .critical && p.runsOutAt == nil)
            #expect(p.short(calendar: Self.utc, now: Self.now) == "out · resets " + reset)
            #expect(p.words() == "out")
        }
        #expect(five(100, elapsed: 4)?.sentence(calendar: Self.utc, now: Self.now) == "You have reached 100%. The window resets at 13:00, in 1h")
    }

    /// A window that just reset: under 3% elapsed the ratio is noise, so there is a tick and no verdict.
    @Test func theFirstMinutesGiveNoVerdict() throws {
        let p = try #require(five(4, elapsed: 0.1))   // 6 of 300 minutes: 2%
        #expect(p.verdict == nil && p.projectedPercent == nil && p.runsOutAt == nil)
        #expect(abs(p.elapsed - 0.02) < 1e-9)
        #expect(p.short(calendar: Self.utc, now: Self.now) == nil && p.sentence(calendar: Self.utc, now: Self.now) == nil)
        // At 3% the verdict starts.
        #expect(five(10, elapsed: 0.15)?.verdict == .critical)
    }

    /// A window nearly over: the projection barely moves, and a late overshoot is amber.
    @Test func aWindowNearlyOver() throws {
        let under = try #require(five(80, elapsed: 4.9))
        #expect(under.verdict == .onPace && Int(under.projectedPercent!.rounded()) == 82)
        let over = try #require(five(99, elapsed: 4.9))
        #expect(over.verdict == .over)
        #expect(over.sentence(calendar: Self.utc, now: Self.now) == "At this pace you reach 100% at 12:02, 3m before the reset in 6m")
    }

    /// Nothing used: on pace, and the sentence says so without dividing by zero.
    @Test func nothingUsed() throws {
        let p = try #require(five(0, elapsed: 2))
        #expect(p.verdict == .onPace && p.projectedPercent == 0)
    }

    /// No reset data, or a reset in the past (clock skew, or a reading from before the reset):
    /// no marker at all. The used percent then belongs to a window that is gone.
    @Test func noResetOrAPastResetGivesNoMarker() {
        #expect(SpendPace(usedPercent: 40, resetsAt: nil, length: SpendPace.fiveHours, now: Self.now) == nil)
        #expect(SpendPace(usedPercent: 40, resetsAt: Self.now.addingTimeInterval(-60), length: SpendPace.fiveHours, now: Self.now) == nil)
        #expect(SpendPace(usedPercent: 40, resetsAt: Self.now, length: SpendPace.fiveHours, now: Self.now) == nil)
        // The daemon's "not reported" instant (Go's zero time, or the epoch).
        #expect(SpendPace(usedPercent: 40, resetsAt: Date(timeIntervalSince1970: 0), length: SpendPace.fiveHours, now: Date(timeIntervalSince1970: -10)) == nil)
    }

    /// Clock skew the other way: the reset is further off than the window is long. The tick sits
    /// at the start and there is no verdict.
    @Test func aResetBeyondTheWindowGivesATickAtTheStart() throws {
        let p = try #require(SpendPace(usedPercent: 40, resetsAt: Self.now.addingTimeInterval(6 * Self.h), length: SpendPace.fiveHours, now: Self.now))
        #expect(p.elapsed == 0 && p.verdict == nil)
    }

    /// No used percent: the tick still shows where the window is.
    @Test func noUsedPercentKeepsTheTick() throws {
        let p = try #require(five(nil, elapsed: 2.5))
        #expect(p.elapsed == 0.5 && p.verdict == nil)
    }

    /// Codex check P3: a percent that is no reading gives no verdict.
    @Test func aPercentThatIsNotAReadingGivesNoVerdict() throws {
        for bad in [Double.nan, .infinity, -5] {
            let p = try #require(five(bad, elapsed: 2.5))
            #expect(p.verdict == nil && p.projectedPercent == nil && p.elapsed == 0.5)
        }
    }

    /// Codex check P3: the tick stays inside the bar, and a bar too narrow for it has none.
    @Test func theTickStaysInsideTheBar() throws {
        let p = try #require(five(31, elapsed: 2.5))
        #expect(p.tickOffset(width: 60) == 29.25)
        #expect(p.tickOffset(width: 0) == nil && p.tickOffset(width: 1) == nil)
        #expect(five(31, elapsed: 4.999)?.tickOffset(width: 60) == 58.5)
        #expect(SpendPace(usedPercent: 1, resetsAt: Self.now.addingTimeInterval(6 * Self.h), length: SpendPace.fiveHours, now: Self.now)?.tickOffset(width: 60) == 0)
    }

    /// The projection is capped for display.
    @Test func theProjectionIsCapped() throws {
        #expect(five(90, elapsed: 0.2)?.projectedPercent == 999)
    }

    /// One line under an account's bars, only for the windows with something to say. Each bar shows its
    /// own reset, so the line carries the pace alone.
    @Test func theAccountLineNamesOnlyWhatMatters() throws {
        let over = five(66, elapsed: 3), fine = five(31, elapsed: 2.5), out = five(100, elapsed: 4)
        #expect(SpendPace.line(fiveHour: fine, sevenDay: fine, calendar: Self.utc, now: Self.now) == nil)
        #expect(SpendPace.line(fiveHour: over, sevenDay: fine, calendar: Self.utc, now: Self.now) == "5h 6% ahead")
        #expect(SpendPace.line(fiveHour: over, sevenDay: out, calendar: Self.utc, now: Self.now) == "5h 6% ahead · 7d out")
        #expect(SpendPace.line(fiveHour: nil, sevenDay: nil, calendar: Self.utc, now: Self.now) == nil)
        #expect(SpendPace.worst(over, out) == .critical && SpendPace.worst(fine, nil) == .onPace && SpendPace.worst(nil, nil) == nil)
    }
}
