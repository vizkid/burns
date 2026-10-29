import XCTest
@testable import BurnsKit

/// Item 9: each spend bar shows the time until its window resets (COMMANDER_SPEC report section 6).
final class SpendResetTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 4_070_000_000)

    func testCountdownShape() {
        XCTAssertNil(SpendReset.countdown(to: nil, now: now), "no reset data shows nothing")
        XCTAssertNil(SpendReset.countdown(to: now.addingTimeInterval(-60), now: now), "a past reset shows nothing")
        XCTAssertNil(SpendReset.countdown(to: now, now: now), "never 0m")
        XCTAssertEqual(SpendReset.countdown(to: now.addingTimeInterval(30), now: now), "<1m")
        XCTAssertEqual(SpendReset.countdown(to: now.addingTimeInterval(14 * 60), now: now), "14m")
        XCTAssertEqual(SpendReset.countdown(to: now.addingTimeInterval(2 * 3600 + 14 * 60 + 59), now: now), "2h 14m")
        XCTAssertEqual(SpendReset.countdown(to: now.addingTimeInterval(3 * 3600), now: now), "3h")
        XCTAssertEqual(SpendReset.countdown(to: now.addingTimeInterval(163 * 3600 + 5 * 60), now: now), "6d 19h")
        XCTAssertEqual(SpendReset.countdown(to: now.addingTimeInterval(6 * 86400), now: now), "6d")
    }
}
