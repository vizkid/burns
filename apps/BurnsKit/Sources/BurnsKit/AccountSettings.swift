import SwiftUI

/// An account name and its providers: colour and emoji belong to the name, not the provider.
public struct AccountGroup: Identifiable, Equatable, Sendable {
    public var name: String
    public var entries: [CentralAccount]
    public var swatch: Swatch
    public var emoji: String
    public var id: String { name }

    /// Names in list order. A name with no colour takes the next unused swatch, and one with no
    /// emoji follows its swatch.
    public static func build(_ accounts: [CentralAccount]) -> [AccountGroup] {
        var names: [String] = []
        for a in accounts where !names.contains(a.name) { names.append(a.name) }
        var free = Swatch.allCases.filter { s in !accounts.contains { $0.colour == s.rawValue } }
        return names.map { name in
            let entries = accounts.filter { $0.name == name }
            let chosen = entries.compactMap { $0.colour.flatMap(Swatch.init(rawValue:)) }.first
            let swatch = chosen ?? (free.isEmpty ? Swatch.allCases[names.firstIndex(of: name)! % Swatch.allCases.count] : free.removeFirst())
            return AccountGroup(name: name, entries: entries, swatch: swatch,
                                emoji: entries.compactMap(\.emoji).first ?? swatch.circle)
        }
    }

    /// True when a new account can be added: the rule `smithersd` applies, checked before sending.
    public static func validName(_ name: String) -> Bool {
        name.range(of: "^[a-z][a-z0-9-]{0,31}$", options: .regularExpression) != nil
    }
}

extension Swatch {
    /// The emoji an account shows until one is chosen (Viz, 2026-10-31). Circles have no teal, so
    /// teal takes orange: six accounts still get six emoji.
    public var circle: String {
        switch self {
        case .purple: "🟣"
        case .teal: "🟠"
        case .blue: "🔵"
        case .green: "🟢"
        case .pink: "🔴"
        case .grey: "⚪"
        }
    }
}

/// Settings' Accounts page, modelled on System Settings: a grouped form, a row per name, and a page
/// for each name.
public struct AccountsForm: View {
    @Binding var accounts: [CentralAccount]
    let setUp: [AccountID: [String]]
    @State private var adding = false

    public init(accounts: Binding<[CentralAccount]>, setUp: [AccountID: [String]]) {
        _accounts = accounts; self.setUp = setUp
    }

    public var body: some View {
        Form {
            Section {
                ForEach(AccountGroup.build(accounts)) { group in
                    NavigationLink(value: group.name) { row(group) }
                }
                Button("Add Account…") { adding = true }
            } footer: {
                Text("Accounts apply to every host. A home is created when an account first launches on a host.")
                    .font(.footnote).foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
        .navigationTitle("Accounts")
        .navigationDestination(for: String.self) { name in AccountPage(name: name, accounts: $accounts, setUp: setUp) }
        .sheet(isPresented: $adding) { AddAccount(accounts: $accounts) }
    }

    func row(_ group: AccountGroup) -> some View {
        LabeledContent {
            Text(group.entries.map { $0.id.label.split(separator: " ")[0] }.joined(separator: ", "))
        } label: {
            HStack(spacing: 8) { Text(group.emoji); Text(group.name) }
        }
    }
}

/// One name: its colour, emoji, and each provider's homes.
public struct AccountPage: View {
    let name: String
    @Binding var accounts: [CentralAccount]
    let setUp: [AccountID: [String]]
    @State private var confirming = false

    public init(name: String, accounts: Binding<[CentralAccount]>, setUp: [AccountID: [String]]) {
        self.name = name; _accounts = accounts; self.setUp = setUp
    }

    var group: AccountGroup? { AccountGroup.build(accounts).first { $0.name == name } }

    public var body: some View {
        Form {
            if let group {
                Section("Appearance") {
                    LabeledContent("Colour") { swatches(group) }
                    LabeledContent("Emoji") { EmojiPicker(emoji: emoji(group)) }
                }
                ForEach(group.entries) { entry in
                    Section(entry.id.label) {
                        LabeledContent("Home", value: entry.home)
                        if !entry.extraHomes.isEmpty { LabeledContent("Extra homes", value: entry.extraHomes.joined(separator: ", ")) }
                        LabeledContent("Kind", value: entry.kind == "azure" ? "Azure" : "Subscription")
                        LabeledContent("Set up on", value: (setUp[entry.id] ?? []).joined(separator: ", ").nonEmpty ?? "No hosts yet")
                    }
                }
                Section {
                    Button("Remove \(name)", role: .destructive) { confirming = true }
                        .confirmationDialog("Remove \(name) from every host?", isPresented: $confirming) {
                            Button("Remove", role: .destructive) { accounts.removeAll { $0.name == name } }
                        } message: {
                            Text("Its homes and logins stay on each host. Burns stops tracking them.")
                        }
                }
            }
        }
        .formStyle(.grouped)
        .navigationTitle(name)
    }

    func swatches(_ group: AccountGroup) -> some View {
        HStack(spacing: 8) {
            ForEach(Swatch.allCases, id: \.self) { s in
                Button { update { $0.colour = s.rawValue } } label: {
                    Circle().fill(s.color).frame(width: 20, height: 20)
                        .overlay {
                            if s == group.swatch { Image(systemName: "checkmark").font(.caption2.bold()).foregroundStyle(.white) }
                        }
                }
                .buttonStyle(.plain)
                .accessibilityLabel(s.rawValue)
                .accessibilityAddTraits(s == group.swatch ? .isSelected : [])
            }
        }
    }

    /// Empty returns to the swatch's circle.
    func emoji(_ group: AccountGroup) -> Binding<String> {
        Binding(get: { group.emoji }, set: { new in
            update { $0.emoji = new.isEmpty || new == group.swatch.circle ? nil : new }
        })
    }

    func update(_ change: (inout CentralAccount) -> Void) {
        for i in accounts.indices where accounts[i].name == name { change(&accounts[i]) }
    }
}

extension String {
    var nonEmpty: String? { isEmpty ? nil : self }
}

#if os(macOS)
/// The Mac's Accounts tab, modelled on Mail's: the list beside the selected account's page, with no
/// navigation inside the settings window.
public struct AccountsPane: View {
    @Binding var accounts: [CentralAccount]
    let setUp: [AccountID: [String]]
    @State private var selected: String?
    @State private var adding = false

    public init(accounts: Binding<[CentralAccount]>, setUp: [AccountID: [String]], selected: String? = nil) {
        _accounts = accounts; self.setUp = setUp; _selected = State(initialValue: selected)
    }

    public var body: some View {
        let groups = AccountGroup.build(accounts)
        HStack(spacing: 0) {
            VStack(spacing: 0) {
                List(groups, selection: $selected) { group in
                    Label { Text(group.name) } icon: { Text(group.emoji) }.tag(group.name)
                }
                .listStyle(.inset)
                Divider()
                HStack {
                    Button { adding = true } label: { Image(systemName: "plus") }
                        .buttonStyle(.borderless)
                        .help("Add Account").accessibilityLabel("Add Account")
                    Spacer()
                }
                .padding(6)
            }
            .frame(width: 160)
            Divider()
            if let name = selected ?? groups.first?.name, groups.contains(where: { $0.name == name }) {
                AccountPage(name: name, accounts: $accounts, setUp: setUp).id(name)
            } else {
                ContentUnavailableView("No Account Selected", systemImage: "person.2")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .sheet(isPresented: $adding) { AddAccount(accounts: $accounts) }
    }
}
#endif

/// Adds a provider entry, adopting an existing home or naming a new one.
public struct AddAccount: View {
    @Binding var accounts: [CentralAccount]
    @Environment(\.dismiss) private var dismiss
    @State private var provider = "claude"
    @State private var name = ""
    @State private var home = ""
    @State private var azure = false

    public init(accounts: Binding<[CentralAccount]>) { _accounts = accounts }

    var homeUsed: String { home.isEmpty ? ".\(provider)-\(name)" : home }
    var duplicate: Bool { accounts.contains { ($0.provider == provider && $0.name == name) || $0.home == homeUsed } }
    var valid: Bool { AccountGroup.validName(name) && !duplicate }

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
            }
            .formStyle(.grouped)
            .navigationTitle("Add Account")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Add") {
                        accounts.append(CentralAccount(provider: provider, name: name, home: homeUsed,
                                                       kind: provider == "codex" && azure ? "azure" : "subscription"))
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
            ForEach(list) { LabeledContent($0.id.label, value: $0.home) }
        }
    }
}
