import BurnsKit
import Foundation
import IOKit.ps

/// Each host's icon from Rex, which only a Mac can ask. `UsageStore` shares them with the phone.
enum RexIcons {
    /// Nil when Rex did not answer: the icons already known stay.
    @MainActor
    static func read(_ statuses: [HostStatus]) async -> [String: String]? {
        guard let rex = await Rex.hosts() else { return nil }
        var icons: [String: String] = [:]
        for name in statuses.compactMap(\.snapshot?.usage.host) {
            if let picked = rex.icon(for: name) {
                icons[name] = picked
            } else if RexHosts.short(name) == RexHosts.thisMachine {
                icons[name] = hasBattery ? "laptopcomputer" : HostIcon.fallback // Rex stores none for its own machine.
            }
        }
        return icons
    }

    /// An internal battery. A UPS is a power source too, and a desktop on one is no laptop.
    private static let hasBattery: Bool = {
        let info = IOPSCopyPowerSourcesInfo().takeRetainedValue()
        let sources = IOPSCopyPowerSourcesList(info).takeRetainedValue() as [CFTypeRef]
        return sources.contains { source in
            let description = IOPSGetPowerSourceDescription(info, source)?.takeUnretainedValue() as? [String: Any]
            return description?[kIOPSTypeKey] as? String == kIOPSInternalBatteryType
        }
    }()
}
