import BurnsKit
import SwiftUI

/// Burns for iOS (docs/SPEC.md, "Apps and widgets"): the ring row over the selected account's
/// burndown, its live pane shares, and a line of hosts. No launch.
@main
struct BurnsApp: App {
    @State private var store = UsageStore()

    var body: some Scene {
        WindowGroup {
            HomeView(store: store)
        }
    }
}

struct HomeView: View {
    let store: UsageStore
    @State private var panes = PaneReader()
    @Environment(\.scenePhase) private var phase
    @State private var showsSettings = false
    @State private var visible: CGFloat = 0
    @State private var hosts: CGFloat = 0

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: ScreenFit.spacing) {
                    UsageView(model: store.model, diameter: 52) { ring, selected in
                        Button { store.select(ring.id) } label: { RingCell(ring: ring, selected: selected, diameter: 52, showsName: true) }
                            .buttonStyle(.plain)
                    }
                    .frame(height: store.model.rings.isEmpty ? 120 : ScreenFit.usageHeight(visible: visible, below: hosts))
                    VStack(alignment: .leading, spacing: ScreenFit.spacing) {
                        LivePanes(store: store, reader: panes)
                        HostStatusLine(statuses: store.statuses, names: store.hostNames, icons: store.hostIcons)
                    }
                    .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { hosts = $0 }
                }
                .padding(ScreenFit.padding)
            }
            // The height inside the large title and the home indicator, so the screen fits at rest.
            .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { visible = $0 }
            .refreshable { await panes.reload(store) }
            .navigationTitle("Burns")
            // The large title shares the gear's row, which leaves more height for the burndown.
            .toolbarTitleDisplayMode(.inlineLarge)
            .toolbar {
                Button { showsSettings = true } label: { Image(systemName: "gear") }.accessibilityLabel("Settings")
            }
            .sheet(isPresented: $showsSettings) {
                NavigationStack {
                    Form {
                        NavigationLink("Hosts") { HostSettings(store: store).navigationTitle("Hosts") }
                        NavigationLink("Accounts") { AccountSettings(store: store) }
                    }
                        .navigationTitle("Settings")
                        .toolbar { Button("Done") { showsSettings = false } }
                }
            }
        }
        .task { await panes.reload(store) }
        .onChange(of: phase) { _, now in if now == .active { Task { await panes.reload(store) } } }
        // No fetch: the model rebuilds from the cache, so a reset passes and the arcs move while the
        // screen stays open. Rule 2 needs this between pulls.
        .task(id: phase) {
            while phase == .active, !Task.isCancelled {
                try? await Task.sleep(for: .seconds(60))
                if !Task.isCancelled { store.rebuild() }
            }
        }
    }
}
