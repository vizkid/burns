import BurnsKit
import Foundation
import IOKit.ps

/// Each host's icon from Rex, which only a Mac can ask. `UsageStore` shares them with the phone.
enum RexIcons {
    private struct HostList: Decodable {
        struct Entry: Decodable {
            struct Host: Decodable { let label: String; let icon: String? }
            let host: Host
        }
        let hosts: [Entry]
    }

    /// Nil when Rex did not answer: the icons already known stay.
    @MainActor
    static func read(_ statuses: [HostStatus]) async -> [String: String]? {
        guard let known = await Rex.json(HostList.self, ["hosts", "list", "--json"])?.hosts.map(\.host) else { return nil }
        let entries = known.map { (label: $0.label, icon: $0.icon) }
        let here = ProcessInfo.processInfo.hostName.split(separator: ".").first.map { $0.lowercased() } ?? ""
        var icons: [String: String] = [:]
        for name in statuses.compactMap(\.snapshot?.usage.host) {
            if let picked = PaneShares.icon(host: name, entries: entries) {
                icons[name] = picked
            } else if name.lowercased() == here {
                icons[name] = hasBattery ? "laptopcomputer" : HostIcon.fallback // Rex stores none for its own machine.
            }
        }
        return icons
    }

    private static var hasBattery: Bool {
        let info = IOPSCopyPowerSourcesInfo().takeRetainedValue()
        return CFArrayGetCount(IOPSCopyPowerSourcesList(info).takeRetainedValue()) > 0
    }
}
