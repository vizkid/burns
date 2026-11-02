import SwiftUI

/// Adds a provider entry, adopting an existing home or naming a new one.
public struct AddAccount: View {
    @Binding var accounts: [CentralAccount]
    @Environment(\.dismiss) private var dismiss
    @State private var provider = "claude"
    @State private var name = ""
    @State private var home = ""
    @State private var azure = false

    public init(accounts: Binding<[CentralAccount]>) { _accounts = accounts }

    /// Nil when the typed home cannot be used.
    var homeUsed: String? {
        guard let clean = AccountGroup.cleanHome(home) else { return nil }
        return clean.isEmpty ? ".\(provider)-\(name)" : clean
    }
    var duplicate: Bool { accounts.contains { ($0.provider == provider && $0.name == name) || $0.home == homeUsed } }
    var valid: Bool { AccountGroup.validName(name) && homeUsed != nil && !duplicate }

    public var body: some View {
        NavigationStack {
            Form {
                Picker("Provider", selection: $provider) {
                    Text("Claude").tag("claude")
                    Text("Codex").tag("codex")
                }
                TextField("Name", text: $name, prompt: Text("work"))
                TextField("Home", text: $home, prompt: Text(".\(provider)-\(name.isEmpty ? "work" : name)"))
                if provider == "codex" { Toggle("Azure credits", isOn: $azure) }
                if duplicate { Text("This account or home is already in the list.").font(.footnote).foregroundStyle(.secondary) }
                if homeUsed == nil { Text("A home is absolute, or relative to your home folder.").font(.footnote).foregroundStyle(.secondary) }
            }
            .formStyle(.grouped)
            .navigationTitle("Add Account")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Add") {
                        if let homeUsed {
                            accounts.append(CentralAccount(provider: provider, name: name, home: homeUsed,
                                                           kind: provider == "codex" && azure ? "azure" : "subscription"))
                        }
                        dismiss()
                    }
                    .disabled(!valid)
                }
            }
        }
        .frame(minWidth: 380, minHeight: 260)
    }
}

/// A host whose file differs and was not overwritten: what it has that the list does not, and the
/// two ways out.
public struct AccountReview: View {
    let host: String
    let central: [CentralAccount]
    let onHost: [CentralAccount]
    let keepHost: () -> Void
    let keepList: () -> Void

    public init(host: String, central: [CentralAccount], onHost: [CentralAccount], keepHost: @escaping () -> Void, keepList: @escaping () -> Void) {
        self.host = host; self.central = central; self.onHost = onHost; self.keepHost = keepHost; self.keepList = keepList
    }

    var onlyHost: [CentralAccount] { onHost.filter { !central.contains($0) } }
    var onlyList: [CentralAccount] { central.filter { !onHost.contains($0) } }

    public var body: some View {
        Form {
            Section("Only on \(host)") { rows(onlyHost) }
            Section("Only in Burns") { rows(onlyList) }
            Section {
                Button("Use \(host)'s accounts") { keepHost() }
                Button("Overwrite \(host)", role: .destructive) { keepList() }
            } footer: {
                Text("Using the host's accounts updates every other host too.").font(.footnote).foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
        .navigationTitle("Review \(host)")
    }

    @ViewBuilder func rows(_ list: [CentralAccount]) -> some View {
        if list.isEmpty {
            Text("Nothing").foregroundStyle(.secondary)
        } else {
            ForEach(list) { LabeledContent($0.id.label, value: $0.detail) }
        }
    }
}
