import BurnsKit
import SwiftUI

/// The host list (docs/SPEC.md, "Apps and widgets"): one tailnet name per smithersd. A grouped
/// form, as in System Settings: the hosts and the add row in one section, the note as its footer.
struct HostSettings: View {
    let store: UsageStore
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
    let store: UsageStore

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

/// Every host on one line: its Rex icon, its name, and the age of its answer. A failed host swaps
/// the icon for a warning and the age for the reason, so colour never says it alone.
struct HostStatusLine: View {
    let statuses: [HostStatus]
    let names: [String: String]
    let icons: [String: String]

    private func name(_ s: HostStatus) -> String {
        let host = PaneReader.name(s)
        return names[host] ?? host
    }

    private func icon(_ s: HostStatus) -> String {
        s.snapshot.flatMap { icons[$0.usage.host] } ?? HostIcon.fallback
    }

    private func item(_ s: HostStatus, showsAge: Bool) -> some View {
        HStack(spacing: 4) {
            Image(systemName: s.error == nil ? icon(s) : "exclamationmark.triangle.fill")
                .foregroundStyle(s.error == nil ? Color.secondary : Color.orange)
            Text(name(s)).lineLimit(1)
            if let error = s.error {
                // Always the age: a phone has no tooltip, and cached readings must show their time.
                Text("\(error) · data \(ageText(s.snapshot?.fetchedAt))").foregroundStyle(.secondary).lineLimit(1)
            } else if showsAge {
                Text(ageText(s.snapshot?.fetchedAt)).foregroundStyle(.secondary).lineLimit(1)
            }
        }
        .fixedSize()
        .help("Read \(ageText(s.snapshot?.fetchedAt))")
    }

    private func line(showsAge: Bool) -> some View {
        HStack(spacing: 12) {
            ForEach(statuses, id: \.address) { item($0, showsAge: showsAge) }
        }
    }

    var body: some View {
        // More hosts than fit keep their names and lose their ages first.
        ViewThatFits(in: .horizontal) {
            line(showsAge: true)
            line(showsAge: false)
            ScrollView(.horizontal, showsIndicators: false) { line(showsAge: false) }
        }
        .font(.caption)
    }
}

