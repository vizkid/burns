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

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    UsageView(model: store.model, diameter: 52) { ring, selected in
                        Button { store.select(ring.id) } label: { RingCell(ring: ring, selected: selected, diameter: 52, showsName: true) }
                            .buttonStyle(.plain)
                    }
                    .frame(height: store.model.rings.isEmpty ? 120 : 320)
                    HostStatusList(statuses: store.statuses)
                }
                .padding()
            }
            .refreshable { await store.refresh() }
            .navigationTitle("Burns")
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
