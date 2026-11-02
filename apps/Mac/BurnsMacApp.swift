import AppKit
import BurnsKit
import SwiftUI

/// Burns for Mac (docs/SPEC.md, "Apps and widgets"): a flame in the menu bar, and a window with
/// the ring row over the selected account's burndown.
@main
struct BurnsMacApp: App {
    private var store: UsageStore { .shared }
    @NSApplicationDelegateAdaptor(BurnsDelegate.self) private var delegate

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
    let store: UsageStore
    @State private var panes = PaneReader()
    /// The menu window: other windows of this app becoming key are not an opening.
    @State private var window: NSWindow?
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            header
            UsageView(model: store.model, diameter: 40) { ring, selected in
                Button { store.select(ring.id) } label: { RingCell(ring: ring, selected: selected, diameter: 40, showsName: true) }
                    .buttonStyle(.plain)
            }
            .frame(height: store.model.rings.isEmpty ? 60 : store.model.usageHeight(full: 250))
            Divider()
            CostPanel(store: store, reader: panes)
            LivePanes(store: store, reader: panes)
            if !store.model.rings.isEmpty { Divider() }
            HostStatusLine(statuses: store.statuses, names: store.hostNames, icons: store.hostIcons)
        }
        .padding(14)
        .frame(width: 380)
        .task {
            LaunchWindow.prepare()
            await reload()
        }
        .background(WindowReader { window = $0 })
        // The window keeps its view between openings, so `.task` runs once. This is each opening.
        .onReceive(NotificationCenter.default.publisher(for: NSWindow.didBecomeKeyNotification)) { note in
            if let window, note.object as? NSWindow === window { Task { await reload() } }
        }
    }

    private func reload() async { await panes.reload(store, icons: RexIcons.read) }
}

extension MenuWindow {
    /// Actions in the top right, as in Apple's menu bar extras, never along the bottom edge: refresh,
    /// settings and Quit, after New Agent.
    var header: some View {
        HStack(spacing: 12) {
            Text("Usage").font(.headline)
            Spacer()
            Button { LaunchWindow.show() } label: { Image(systemName: "plus") }
                .help("New Agent").accessibilityLabel("New Agent")
            Button { Task { await reload() } } label: { Image(systemName: "arrow.clockwise") }
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

/// Reports the window a view sits in.
private struct WindowReader: NSViewRepresentable {
    let found: @MainActor (NSWindow?) -> Void

    final class Reader: NSView {
        var found: (@MainActor (NSWindow?) -> Void)?
        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            let window = window, found = found
            Task { @MainActor in found?(window) } // Not during SwiftUI's own update.
        }
    }

    func makeNSView(context: Context) -> Reader {
        let view = Reader()
        view.found = found
        return view
    }

    func updateNSView(_ view: Reader, context: Context) { view.found = found }
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
