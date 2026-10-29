import BurnsKit
import SwiftUI

/// The host list (docs/SPEC.md, "Apps and widgets"): one tailnet name per smithersd.
struct HostSettings: View {
    @ObservedObject var store: UsageStore
    @State private var draft = ""

    var body: some View {
        Form {
            Section {
                ForEach(store.hosts, id: \.self) { host in
                    HStack {
                        Text(host)
                        Spacer()
                        Button(role: .destructive) { store.hosts.removeAll { $0 == host } } label: { Image(systemName: "minus.circle") }
                            .buttonStyle(.borderless)
                            .accessibilityLabel("Remove \(host)")
                    }
                }
                HStack {
                    TextField("host.tailnet.ts.net", text: $draft)
                        .autocorrectionDisabled()
                        #if os(iOS)
                        .textInputAutocapitalization(.never)
                        .keyboardType(.URL)
                        #endif
                        .onSubmit(add)
                    Button("Add", action: add).disabled(HostList.clean([draft]).isEmpty)
                }
            } header: {
                Text("Hosts")
            } footer: {
                Text("Each host runs smithersd. Burns reads it over Tailscale on port 9766.")
            }
        }
    }

    private func add() {
        store.hosts = HostList.clean(store.hosts + [draft])
        draft = ""
    }
}

/// Each host's freshness, under the chart: a host that did not answer shows its cached time.
struct HostStatusList: View {
    let statuses: [HostStatus]
    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            ForEach(statuses, id: \.address) { s in
                HStack(spacing: 4) {
                    Circle().fill(s.error == nil ? Color.green : Color.orange).frame(width: 6, height: 6)
                    Text(s.address.split(separator: ".").first.map(String.init) ?? s.address)
                    Spacer()
                    Text(s.error.map { "\($0) · data \(ageText(s.snapshot?.fetchedAt))" } ?? ageText(s.snapshot?.fetchedAt))
                        .foregroundStyle(.secondary)
                }
                .font(.caption)
            }
        }
    }
}
