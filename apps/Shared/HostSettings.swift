import BurnsKit
import SwiftUI

/// The host list (docs/SPEC.md, "Apps and widgets"): one tailnet name per smithersd. A grouped
/// form, as in System Settings: the hosts and the add row in one section, the note as its footer.
struct HostSettings: View {
    @ObservedObject var store: UsageStore
    @State private var draft = ""

    var body: some View {
        Form {
            Section {
                ForEach(store.hosts, id: \.self) { host in
                    HStack {
                        Label(host, systemImage: "server.rack")
                        Spacer()
                        Button(role: .destructive) { store.hosts.removeAll { $0 == host } } label: { Image(systemName: "minus.circle") }
                            .buttonStyle(.borderless)
                            .help("Remove \(host)")
                            .accessibilityLabel("Remove \(host)")
                    }
                }
                HStack {
                    TextField("Add a host", text: $draft, prompt: Text("host.tailnet.ts.net"))
                        .labelsHidden()
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
                    .font(.footnote).foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
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
