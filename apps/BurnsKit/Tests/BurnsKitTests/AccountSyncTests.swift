import Foundation
import Testing
@testable import BurnsKit

private func account(_ provider: String, _ name: String, home: String? = nil) -> CentralAccount {
    CentralAccount(provider: provider, name: name, home: home ?? ".\(provider)-\(name)", kind: "subscription")
}

/// docs/SPEC.md, "Accounts in the apps": the central list wins, but never silently.
struct AccountSyncRuleTests {
    let alpha = account("claude", "alpha"), beta = account("codex", "beta")

    @Test func aHostThatMatchesNeedsNothing() {
        #expect(AccountSync.decide(central: [alpha], host: [alpha], hostHash: "h", written: nil) == .inStep)
    }

    @Test func aHostUnchangedSinceTheLastWriteTakesTheList() {
        #expect(AccountSync.decide(central: [alpha, beta], host: [alpha], hostHash: "h", written: "h") == .write)
    }

    /// Nothing on the host would be lost, so there is nothing to review.
    @Test func aHostHoldingOnlyCentralAccountsTakesTheList() {
        #expect(AccountSync.decide(central: [alpha, beta], host: [beta], hostHash: "h", written: nil) == .write)
    }

    @Test func aHandEditedHostIsReviewedNotOverwritten() {
        #expect(AccountSync.decide(central: [alpha], host: [alpha, beta], hostHash: "new", written: "old") == .review)
    }

    @Test func aChangedHomeOnAHostIsReviewed() {
        let moved = account("claude", "alpha", home: ".moved")
        #expect(AccountSync.decide(central: [alpha], host: [moved], hostHash: "new", written: nil) == .review)
    }

    /// An empty central list must never wipe a host.
    @Test func anEmptyCentralListNeverWrites() {
        #expect(AccountSync.decide(central: [], host: [alpha], hostHash: "h", written: "h") == .review)
    }

    @Test func aHeldHostSaysItWasNotOverwritten() {
        #expect(AccountSync.State.review([]).words == "Accounts differ, not overwritten")
        #expect(AccountSync.State.refused(422).words == "Accounts refused (422)")
    }

    @Test func importMergesHostsInFirstSeenOrder() {
        let result = AccountSync.importing([[alpha], [alpha, beta]])
        #expect(result.accounts == [alpha, beta])
        #expect(result.conflicts.isEmpty)
    }

    @Test func importKeepsTheFirstHomeAndReportsTheOther() {
        let moved = account("claude", "alpha", home: ".moved")
        let result = AccountSync.importing([[alpha], [moved]])
        #expect(result.accounts == [alpha])
        #expect(result.conflicts == [.init(id: alpha.id, homes: [alpha.home, ".moved"])])
    }
}

/// A host's accounts file and the requests made of it.
private final class FakeHosts: AccountHosts, @unchecked Sendable {
    var files: [String: Data] = [:]
    var unreachable: Set<String> = []
    var puts: [String] = []
    var refuse: Int?

    func get(_ host: String) async throws -> (data: Data, etag: String) {
        if unreachable.contains(host) { throw URLError(.cannotConnectToHost) }
        return (files[host] ?? Data(), Self.tag(files[host] ?? Data()))
    }

    func put(_ host: String, data: Data, etag: String) async throws -> String {
        if let refuse { throw HostFetcher.Refused(status: refuse) }
        puts.append(host)
        files[host] = data
        return Self.tag(data)
    }

    static func tag(_ d: Data) -> String { "\"\(d.count)-\(d.hashValue)\"" }
}

private final class FakeValues: KeyValues, @unchecked Sendable {
    var values: [String: Data] = [:]
    func data(forKey key: String) -> Data? { values[key] }
    func set(_ value: Data?, forKey key: String) { values[key] = value }
    func removeObject(forKey key: String) { values[key] = nil }
}

private func file(_ accounts: CentralAccount...) -> Data { AccountsFile(accounts: accounts).encoded() }

@MainActor
struct AccountSyncRunTests {
    let alpha = account("claude", "alpha"), beta = account("codex", "beta")

    @Test func firstUseImportsAndWritesNothingWhenHostsAgree() async {
        let hosts = FakeHosts(), values = FakeValues()
        hosts.files = ["a": file(alpha, beta), "b": file(alpha, beta)]
        let store = CentralStore(values: values)
        let out = await AccountSync.run(hosts: ["a", "b"], store: store, client: hosts)
        #expect(store.load() == [alpha, beta])
        #expect(hosts.puts.isEmpty)
        #expect(out.map(\.state) == [.inStep, .inStep])
    }

    @Test func firstUseWithNoAnswerStoresNothing() async {
        let hosts = FakeHosts(), store = CentralStore(values: FakeValues())
        hosts.unreachable = ["a"]
        let out = await AccountSync.run(hosts: ["a"], store: store, client: hosts)
        #expect(store.load() == nil)
        #expect(out.map(\.state) == [.unreachable])
    }

    /// An edit reaches the hosts, and an offline host catches up on a later run.
    @Test func anEditReachesHostsAndAnOfflineHostCatchesUp() async {
        let hosts = FakeHosts(), store = CentralStore(values: FakeValues())
        hosts.files = ["a": file(alpha), "b": file(alpha)]
        _ = await AccountSync.run(hosts: ["a", "b"], store: store, client: hosts)
        store.save([alpha, beta])
        hosts.unreachable = ["b"]
        let first = await AccountSync.run(hosts: ["a", "b"], store: store, client: hosts)
        #expect(first.map(\.state) == [.written, .unreachable])
        hosts.unreachable = []
        let second = await AccountSync.run(hosts: ["a", "b"], store: store, client: hosts)
        #expect(second.map(\.state) == [.inStep, .written])
    }

    @Test func aHandEditIsHeldForReview() async {
        let hosts = FakeHosts(), store = CentralStore(values: FakeValues())
        hosts.files = ["a": file(alpha)]
        _ = await AccountSync.run(hosts: ["a"], store: store, client: hosts)
        hosts.files["a"] = file(alpha, beta)
        let out = await AccountSync.run(hosts: ["a"], store: store, client: hosts)
        #expect(out.map(\.state) == [.review([alpha, beta])])
        #expect(hosts.puts.isEmpty)
    }

    @Test func aRefusedWriteIsReported() async {
        let hosts = FakeHosts(), store = CentralStore(values: FakeValues())
        hosts.files = ["a": file(alpha)]
        _ = await AccountSync.run(hosts: ["a"], store: store, client: hosts)
        store.save([alpha, beta])
        hosts.refuse = 422
        let out = await AccountSync.run(hosts: ["a"], store: store, client: hosts)
        #expect(out.map(\.state) == [.refused(422)])
    }

    @Test func theStoreKeepsOrderAndForgetsRemovedAccounts() {
        let values = FakeValues(), store = CentralStore(values: values)
        store.save([beta, alpha])
        #expect(store.load() == [beta, alpha])
        store.save([alpha])
        #expect(store.load() == [alpha])
        #expect(values.values.keys.filter { $0.hasPrefix("account.") }.count == 1)
    }
}
