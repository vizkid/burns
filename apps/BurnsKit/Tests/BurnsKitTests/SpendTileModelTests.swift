import Foundation
import Testing
@testable import BurnsKit

/// IOS_HOME_SPEC "Spend": one tile per account, its two windows as rings, the pace in words and
/// the reset countdowns. The operator, 2026-10-01: "I still like my rings for spend. Could we turn the
/// rings into 4x tiles like the summary tiles you had in R1 on the Spend page instead of the bars?"
struct SpendTileModelTests {
    /// 12:00 UTC on a Wednesday.
    static let now = Date(timeIntervalSince1970: 1_790_769_600)
    static let h = 3600.0
    static var utc: Calendar { var c = Calendar(identifier: .gregorian); c.timeZone = TimeZone(identifier: "UTC")!; return c }

    func model(_ account: String = "vela", five: Double?, fiveElapsed: Double? = nil, seven: Double?, sevenElapsedDays: Double? = nil,
               credits: Bool = false) -> SpendTileModel {
        SpendTileModel(account: account, fiveHourPercent: five,
                       fiveHourResetsAt: fiveElapsed.map { Self.now.addingTimeInterval((5 - $0) * Self.h) },
                       sevenDayPercent: seven,
                       sevenDayResetsAt: sevenElapsedDays.map { Self.now.addingTimeInterval((7 - $0) * 24 * Self.h) },
                       onCredits: credits, now: Self.now, calendar: Self.utc)
    }

    /// Under pace in both windows: no words, the countdowns, and VoiceOver reads both windows.
    @Test func underPaceSaysNothingButTheCountdowns() {
        let m = model(five: 31, fiveElapsed: 2.5, seven: 20, sevenElapsedDays: 3.5)
        #expect(m.verdict == nil && m.worst == .onPace)
        #expect(m.fiveHour.percentText == "31%" && m.sevenDay.percentText == "20%")
        #expect(m.fiveHour.resetsIn == "2h 30m" && m.sevenDay.resetsIn == "3d 12h")
        #expect(m.fiveHour.elapsed == 0.5 && m.sevenDay.elapsed == 0.5)
        #expect(m.accessibilityLabel == "vela: five-hour 31%, resets in 2h 30m; seven-day 20%, resets in 3d 12h")
    }

    /// The shape the coordinator gave, "vela: five-hour 62%, over pace; seven-day 30%", with each
    /// window's reset added after the review: the tile shows it, so VoiceOver must read it.
    @Test func overPaceIsSpokenWithItsWindow() {
        // 3h in, 66%: 100% at 4h33m, 27m before the reset: over pace, late in the window.
        let m = model(five: 66, fiveElapsed: 3, seven: 30, sevenElapsedDays: 3.5)
        #expect(m.fiveHour.verdict == .over && m.sevenDay.verdict == .onPace)
        #expect(m.verdict == "5h 6% ahead" && m.worst == .over)
        #expect(m.verdictWithReset == "5h 6% ahead · resets 2h")
        #expect(m.accessibilityLabel == "vela: five-hour 66%, 6% ahead, resets in 2h; seven-day 30%, resets in 3d 12h")
    }

    @Test func runsOutAndOutNameTheTime() {
        let m = model("nova", five: 12, fiveElapsed: 4.4, seven: 60, sevenElapsedDays: 2)
        #expect(m.verdict == "7d 31% ahead" && m.worst == .critical)
        #expect(m.accessibilityLabel == "nova: five-hour 12%, resets in 36m; seven-day 60%, 31% ahead, resets in 5d")
        let out = model(five: 42, fiveElapsed: 2.5, seven: 100, sevenElapsedDays: 5, credits: true)
        #expect(out.verdict == "7d out" && out.onCredits)
        #expect(out.verdictWithReset == "7d out · resets 2d")
        #expect(out.accessibilityLabel == "vela: five-hour 42%, resets in 2h 30m; seven-day 100%, out, resets in 2d; on credits")
    }

    /// A window the daemon did not report: no ring progress, "Unknown" in words, no countdown.
    @Test func anUnreportedWindowIsUnknownNotEmpty() {
        let m = model("xvela", five: nil, seven: 30, sevenElapsedDays: 4)
        #expect(m.fiveHour.percent == nil && m.fiveHour.percentText == "Unknown" && m.fiveHour.resetsIn == nil && m.fiveHour.elapsed == nil)
        #expect(m.accessibilityLabel == "xvela: five-hour unknown; seven-day 30%, resets in 3d")
    }

    /// The daemon's "not reported" instant is the epoch: no reset, no tick.
    @Test func theEpochIsNoReset() {
        let m = SpendTileModel(account: "vela", fiveHourPercent: 10, fiveHourResetsAt: Date(timeIntervalSince1970: 0),
                               sevenDayPercent: 10, sevenDayResetsAt: nil, onCredits: false, now: Self.now, calendar: Self.utc)
        #expect(m.fiveHour.elapsed == nil && m.fiveHour.resetsIn == nil)
    }

    /// The ring's fill never passes the whole ring.
    @Test func theFillIsClamped() {
        #expect(model(five: 137, fiveElapsed: 2, seven: -5, sevenElapsedDays: 1).fiveHour.fill == 1)
        #expect(model(five: 137, fiveElapsed: 2, seven: -5, sevenElapsedDays: 1).sevenDay.fill == 0)
    }
}
