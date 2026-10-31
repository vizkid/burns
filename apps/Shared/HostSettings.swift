import BurnsKit
import SwiftUI

/// The host list (docs/SPEC.md, "Apps and widgets"): one tailnet name per smithersd. A grouped
/// form, as in System Settings: the hosts and the add row in one section, the note as its footer.
struct HostSettings: View {
    @ObservedObject var store: UsageStore
    @State private var draft = ""
    @State private var review: HeldHost?

    var body: some View {
        Form {
            Section {
                ForEach(store.hosts, id: \.self) { host in
                    HStack {
                        Label(host, systemImage: "server.rack")
                        Spacer()
                        if let name = reported(host) {
                            TextField("Display name", text: display(name), prompt: Text(name))
                                .labelsHidden().frame(maxWidth: 110).multilineTextAlignment(.trailing)
                        }
                        if let sync = store.accountSync.first(where: { $0.host == host }) {
                            if case .review(let onHost) = sync.state {
                                Button(sync.state.words) { review = HeldHost(host: host, accounts: onHost) }.font(.caption)
                            } else {
                                Text(sync.state.words).font(.caption).foregroundStyle(.secondary)
                            }
                        }
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
        .sheet(item: $review) { held in
            NavigationStack {
                AccountReview(host: held.host, central: store.central, onHost: held.accounts,
                              keepHost: { store.useHost(held.host, held.accounts); review = nil },
                              keepList: { Task { await store.overwrite(held.host) }; review = nil })
                    .toolbar { Button("Cancel") { review = nil } }
            }
            .frame(minWidth: 420, minHeight: 340)
        }
    }

    /// The name the host's smithersd reports.
    private func reported(_ host: String) -> String? {
        store.statuses.first { $0.address == host }?.snapshot?.usage.host
    }

    private func display(_ name: String) -> Binding<String> {
        Binding(get: { store.hostNames[name] ?? "" }, set: { store.hostNames[name] = $0.isEmpty ? nil : $0 })
    }

    private func add() {
        store.hosts = HostList.clean(store.hosts + [draft])
        draft = ""
    }
}

/// A host held for review.
struct HeldHost: Identifiable {
    let host: String
    let accounts: [CentralAccount]
    var id: String { host }
}

/// Settings' Accounts page, on the live central list.
struct AccountSettings: View {
    @ObservedObject var store: UsageStore

    var body: some View {
        let list = Binding(get: { store.central }, set: { store.edit($0) })
        if store.central.isEmpty {
            ContentUnavailableView("No accounts yet", systemImage: "person.2", description: Text("Burns imports them from your hosts about a minute after first launch."))
        } else {
            #if os(macOS)
            AccountsPane(accounts: list, setUp: store.setUp)
            #else
            // The settings sheet owns the one navigation stack.
            AccountsForm(accounts: list, setUp: store.setUp)
            #endif
        }
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
