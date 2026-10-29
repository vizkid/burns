import Foundation
@testable import BurnsKit

/// Synthetic answers shaped like real smithersd output: offsets and six fractional digits in
/// observed_at, an account with no limits, a Codex account with only a weekly window, Azure.
enum Fixture {
    /// 2026-10-29 06:00 UTC.
    static let now = Date(timeIntervalSince1970: 1_793_253_600)
    static let h = 3600.0

    static func iso(_ d: Date) -> String {
        let f = ISO8601DateFormatter(); f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f.string(from: d)
    }

    static func window(_ pct: Double?, resets: Date, observed: Date) -> String {
        let p = pct.map { String($0) } ?? "null"
        return #"{"percent":\#(p),"state":"\#(pct == nil ? "unknown" : "live")","resets_at":"\#(iso(resets))","observed_at":"\#(iso(observed))"}"#
    }

    /// One host's /usage: alpha has Claude 5h and 7d, beta has Codex 7d only, beta has Azure.
    static func usage(host: String, alpha5: Double?, alpha7: Double?, beta7: Double?, observed: Date = now) -> Data {
        let a5 = alpha5.map { #""300":\#(window($0, resets: now + 2 * h, observed: observed))"# }
        let a7 = alpha7.map { #""10080":\#(window($0, resets: now + 72 * h, observed: observed))"# }
        let b7 = beta7.map { #""10080":\#(window($0, resets: now + 100 * h, observed: observed))"# }
        let alphaWindows = [a5, a7].compactMap { $0 }.joined(separator: ",")
        let alphaLimits = alphaWindows.isEmpty ? "[]" : #"[{"limit_id":"claude","windows":{\#(alphaWindows)},"credits":null}]"#
        let betaLimits = b7.map { #"[{"limit_id":"codex","windows":{\#($0)},"credits":{"has_credits":false,"unlimited":false,"balance":"0","observed_at":"2026-10-29T05:53:56.565Z"}}]"# } ?? "[]"
        return Data(#"""
        {"host":"\#(host)","observed_at":"2026-10-29T14:00:00.347688+08:00","accounts":[
         {"provider":"claude","account":"alpha","kind":"subscription","limits":\#(alphaLimits)},
         {"provider":"claude","account":"beta","kind":"subscription","limits":[]},
         {"provider":"codex","account":"alpha","kind":"subscription","limits":[]},
         {"provider":"codex","account":"beta","kind":"subscription","limits":\#(betaLimits)},
         {"provider":"codex","account":"beta-azure","kind":"azure","limits":[]}]}
        """#.utf8)
    }

    static func readings(host: String, _ rows: [(provider: String, account: String, minutes: Int, pct: Double, resets: Date, at: Date)]) -> Data {
        let items = rows.map {
            #"{"provider":"\#($0.provider)","account":"\#($0.account)","limit_id":"\#($0.provider)","minutes":\#($0.minutes),"percent":\#($0.pct),"resets_at":"\#(iso($0.resets))","observed_at":"\#(iso($0.at))"}"#
        }
        return Data(#"{"host":"\#(host)","readings":[\#(items.joined(separator: ","))]}"#.utf8)
    }

    static func snapshot(_ address: String, usage: Data, readings: Data = Data(#"{"host":"h","readings":[]}"#.utf8), fetched: Date = now) throws -> HostSnapshot {
        HostSnapshot(address: address, fetchedAt: fetched,
                     usage: try Wire.decoder.decode(UsageAnswer.self, from: usage),
                     readings: try Wire.decoder.decode(ReadingsAnswer.self, from: readings).readings)
    }
}
