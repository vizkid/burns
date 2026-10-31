import AppKit
import BurnsKit
import SwiftUI

/// Burns for Mac (docs/SPEC.md, "Apps and widgets"): a flame in the menu bar, and a window with
/// the ring row over the selected account's burndown.
@main
struct BurnsMacApp: App {
    @StateObject private var store = UsageStore()

    /// `open -a Burns --args --hosts a.ts.net,b.ts.net` replaces the host list: macOS keeps the
    /// app group's preferences from other processes, so a script cannot write them directly.
    init() {
        let args = ProcessInfo.processInfo.arguments
        if let i = args.firstIndex(of: "--hosts"), i + 1 < args.count {
            HostList(defaults: AppGroup.defaults).hosts = args[i + 1].split(separator: ",").map(String.init)
        }
    }

    var body: some Scene {
        MenuBarExtra {
            MenuWindow(store: store)
        } label: {
            Image(nsImage: FlameIcon.image(store.model.pickVerdict))
                .task { await store.poll() }
        }
        .menuBarExtraStyle(.window)
        Settings {
            TabView {
                HostSettings(store: store).tabItem { Label("Hosts", systemImage: "server.rack") }
                AccountSettings(store: store).tabItem { Label("Accounts", systemImage: "person.2") }
            }
            .frame(width: 640, height: 480)
        }
    }
}

struct MenuWindow: View {
    @ObservedObject var store: UsageStore
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            header
            UsageView(model: store.model, diameter: 40) { ring, selected in
                Button { store.select(ring.id) } label: { RingCell(ring: ring, selected: selected, diameter: 40, showsName: true) }
                    .buttonStyle(.plain)
            }
            .frame(height: store.model.rings.isEmpty ? 60 : 250)
            Divider()
            HostStatusList(statuses: store.statuses)
        }
        .padding(14)
        .frame(width: 380)
        .task { await store.refresh() }
    }
}

extension MenuWindow {
    /// Actions in the top right, as in Apple's menu bar extras, never along the bottom edge: refresh,
    /// settings and Quit.
    var header: some View {
        HStack(spacing: 12) {
            Text("Usage").font(.headline)
            Spacer()
            Button { Task { await store.refresh() } } label: { Image(systemName: "arrow.clockwise") }
                .help("Refresh").accessibilityLabel("Refresh")
            Button { NSApp.activate(); openSettings() } label: { Image(systemName: "gearshape") }
                .help("Settings").accessibilityLabel("Settings")
            Button { NSApp.terminate(nil) } label: { Image(systemName: "power") }
                .keyboardShortcut("q")
                .help("Quit Burns").accessibilityLabel("Quit Burns")
        }
        .buttonStyle(.borderless)
        .imageScale(.large)
    }
}

/// The menu bar flame: a template image on or under pace, amber or red when Enter's pick is not.
enum FlameIcon {
    static func image(_ verdict: SpendPace.Verdict?) -> NSImage {
        let color: NSColor? = switch verdict {
        case .critical?: .systemRed
        case .over?: .systemOrange
        default: nil
        }
        return BurnsFlame.menuBarImage(tint: color) ?? NSImage()
    }
}

extension UsageStore {
    /// Every 60 seconds while the app runs: the flame's colour needs fresh quota.
    func poll() async {
        while !Task.isCancelled {
            await refresh()
            try? await Task.sleep(for: .seconds(60))
        }
    }
}
