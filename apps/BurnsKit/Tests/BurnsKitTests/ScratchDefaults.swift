import Foundation

/// Defaults for one test, held in memory. A named suite is a file in ~/Library/Preferences, and
/// cfprefsd writes it back even after the test deletes it, so every run used to leave one file per
/// test behind (725 of them by 2026-11-04).
final class ScratchDefaults {
    let defaults: UserDefaults = MemoryDefaults(empty: ())!
}

/// Overrides the primitive accessors that the typed getters and setters go through, so nothing
/// reaches cfprefsd.
private final class MemoryDefaults: UserDefaults {
    private var values: [String: Any] = [:]

    init?(empty: ()) { super.init(suiteName: nil) }

    override func object(forKey key: String) -> Any? { values[key] }
    override func set(_ value: Any?, forKey key: String) { values[key] = value }
    override func removeObject(forKey key: String) { values[key] = nil }
    override func dictionaryRepresentation() -> [String: Any] { values }
}
