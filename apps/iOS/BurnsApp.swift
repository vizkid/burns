import BurnsKit
import SwiftUI

/// Burns for iOS (docs/SPEC.md, "Apps and widgets"): the ring row over the selected account's
/// burndown, and the host list. No launch.
@main
struct BurnsApp: App {
    @StateObject private var store = UsageStore()
    @Environment(\.scenePhase) private var phase

    var body: some Scene {
        WindowGroup {
            HomeView(store: store)
                .onChange(of: phase) { _, now in if now == .active { Task { await store.refresh() } } }
        }
    }
}

struct HomeView: View {
    @ObservedObject var store: UsageStore
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
                    HostStatusList(statuses: store.statuses)
                        .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { hosts = $0 }
                }
                .padding(ScreenFit.padding)
            }
            // The height inside the large title and the home indicator, so the screen fits at rest.
            .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { visible = $0 }
            .refreshable { await store.refresh() }
            .navigationTitle("Burns")
            // The large title shares the gear's row, which leaves more height for the burndown.
            .toolbarTitleDisplayMode(.inlineLarge)
            .toolbar {
                Button { showsSettings = true } label: { Image(systemName: "gear") }.accessibilityLabel("Hosts")
            }
            .sheet(isPresented: $showsSettings) {
                NavigationStack {
                    HostSettings(store: store)
                        .navigationTitle("Hosts")
                        .toolbar { Button("Done") { showsSettings = false } }
                }
            }
        }
        .task { await store.refresh() }
    }
}
