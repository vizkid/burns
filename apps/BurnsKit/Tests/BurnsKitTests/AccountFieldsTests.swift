import Foundation
import Testing
@testable import BurnsKit

/// A launch override with a cop reference, spelled as the accounts file spells it. Synthetic.
private let withLaunch = Data(#"""
{"accounts":[{"provider":"codex","name":"delta","home":".codex-delta","kind":"azure",
 "launch":{"model":"gpt-x","credential":{"account":"vault","ref":"op://v/i/f","env":"API_KEY"}},
 "note":{"list":[1,true,null,"x"]}}]}
"""#.utf8)

private func decode(_ data: Data) throws -> [CentralAccount] {
    try JSONDecoder().decode(AccountsFile.self, from: data).accounts
}

private func json(_ data: Data) throws -> NSDictionary {
    try JSONSerialization.jsonObject(with: data) as? NSDictionary ?? [:]
}

/// F1: the app writes back what it read, so a field it does not know is never deleted.
struct AccountFieldsTests {
    let bare = CentralAccount(provider: "codex", name: "delta", home: ".codex-delta", kind: "azure")

    @Test func aWriteKeepsLaunchAndUnknownFields() throws {
        var accounts = try decode(withLaunch)
        accounts[0].colour = "teal"
        let written = try json(AccountsFile(accounts: accounts).encoded())
        let account = try #require((written["accounts"] as? [NSDictionary])?.first)
        let launch = try #require(account["launch"] as? NSDictionary)
        #expect(launch["model"] as? String == "gpt-x")
        #expect((launch["credential"] as? NSDictionary)?["ref"] as? String == "op://v/i/f")
        #expect(account["note"] as? NSDictionary == ["list": [1, true, NSNull(), "x"]])
        #expect(account["colour"] as? String == "teal")
    }

    @Test func theStoreKeepsLaunch() throws {
        let store = CentralStore(values: FakeValues())
        let accounts = try decode(withLaunch)
        store.save(accounts)
        #expect(store.load() == accounts)
    }

    /// A file that always round-trips must not read as different from itself.
    @Test func aFileWithNoExtrasEncodesAsBefore() throws {
        let text = String(decoding: AccountsFile(accounts: [bare]).encoded(), as: UTF8.self)
        #expect(!text.contains("launch") && !text.contains("extra"))
    }

    @Test func launchTheListHasNotSeenIsNotInStep() throws {
        let host = try decode(withLaunch)
        #expect(AccountSync.decide(central: [bare], host: host, hostHash: "h", written: nil) == .review)
    }

    /// Hosts in step before the app knew `launch` hold a matching hash, and must still not lose it.
    @Test func aMatchingHashDoesNotLicenseLosingLaunch() throws {
        let host = try decode(withLaunch)
        #expect(AccountSync.decide(central: [bare], host: host, hostHash: "h", written: "h") == .review)
    }

    @Test func aListThatHasTheLaunchIsInStep() throws {
        let host = try decode(withLaunch)
        #expect(AccountSync.decide(central: host, host: host, hostHash: "h", written: nil) == .inStep)
    }

    @Test func aListWithLaunchReachesAHostWithout() throws {
        let central = try decode(withLaunch)
        #expect(AccountSync.decide(central: central, host: [bare], hostHash: "h", written: "h") == .write)
    }

    @Test func takingTheHostBringsItsLaunch() throws {
        let host = try decode(withLaunch)
        #expect(AccountSync.importing([host, [bare]]).accounts == host)
    }

    /// A field added on a host by hand joins the list, so no host waits for review over it.
    @Test func aFieldOnlyTheHostsHaveJoinsTheList() async throws {
        let hosts = FakeHosts(), store = CentralStore(values: FakeValues())
        hosts.files = ["a": withLaunch, "b": withLaunch]
        store.save([bare])
        #expect(await AccountSync.run(hosts: ["a", "b"], store: store, client: hosts).map(\.state) == [.inStep, .inStep])
        #expect(store.load() == (try decode(withLaunch)))
        #expect(hosts.puts.isEmpty)
    }

    /// 2026-11-04: `credit_price` was added to every host by hand, and then an emoji was changed
    /// in the app. Every host was held, and both ways out lost something: import put back the old
    /// emoji, and overwrite dropped the price. Now the price joins the list, so overwrite keeps both.
    @Test func anEditAfterAHandAddedFieldKeepsBoth() async throws {
        let hosts = FakeHosts(), store = CentralStore(values: FakeValues())
        hosts.files = ["a": AccountsFile(accounts: [bare]).encoded()]
        _ = await AccountSync.run(hosts: ["a"], store: store, client: hosts)
        hosts.files["a"] = withLaunch
        var edited = bare
        edited.emoji = "🚀"
        store.save([edited])
        let held = await AccountSync.run(hosts: ["a"], store: store, client: hosts)
        #expect(held.map(\.state) == [.review(try decode(withLaunch))], "the host's own edit may be the newer one")
        #expect(await AccountSync.overwrite(host: "a", store: store, client: hosts) == .written)
        let written = try #require(try decode(hosts.files["a"] ?? Data()).first)
        #expect(written.emoji == "🚀")
        #expect(written.extra == (try decode(withLaunch))[0].extra)
    }

    /// Hosts that disagree on a field's value keep it out of the list, and are held for review.
    /// Overwrite is then the operator's choice, and it drops the field.
    @Test func hostsThatDisagreeOnAFieldAreHeld() async throws {
        let hosts = FakeHosts(), store = CentralStore(values: FakeValues())
        let other = Data(String(decoding: withLaunch, as: UTF8.self).replacingOccurrences(of: "gpt-x", with: "gpt-y").utf8)
        hosts.files = ["a": withLaunch, "b": other]
        store.save([bare])
        let out = await AccountSync.run(hosts: ["a", "b"], store: store, client: hosts)
        #expect(out.map(\.state) == [.review(try decode(withLaunch)), .review(try decode(other))])
        #expect(store.load()?.first?.extra["launch"] == nil)
        #expect(store.load()?.first?.extra["note"] != nil, "a field the hosts agree on still joins")
        #expect(hosts.puts.isEmpty)
        #expect(await AccountSync.overwrite(host: "a", store: store, client: hosts) == .written)
        #expect(try decode(hosts.files["a"] ?? Data()).first?.extra["launch"] == nil)
    }

    /// The review names the field, never its value: a credential is a reference, but stays out.
    @Test func theReviewNamesFieldsNotValues() throws {
        let account = try #require(try decode(withLaunch).first)
        #expect(account.detail == ".codex-delta, launch, note")
        #expect(bare.detail == ".codex-delta")
    }
}

/// F8: iCloud delivers keys one at a time, so a list can be half here.
struct CentralStoreTests {
    let alpha = CentralAccount(provider: "claude", name: "alpha", home: ".claude-alpha", kind: "subscription")
    let beta = CentralAccount(provider: "codex", name: "beta", home: ".codex-beta", kind: "subscription")

    @Test func nothingStoredIsFirstUse() {
        #expect(CentralStore(values: FakeValues()).stored() == .never)
    }

    @Test func anEmptyListIsAList() {
        let store = CentralStore(values: FakeValues())
        store.save([])
        #expect(store.stored() == .ready([]))
    }

    @Test func aMissingAccountKeyIsNotReady() {
        let values = FakeValues(), store = CentralStore(values: values)
        store.save([alpha, beta])
        values.removeObject(forKey: CentralStore.key(beta.id))
        #expect(store.stored() == .arriving)
        #expect(store.load() == nil)
    }

    /// The order arrived and an older account did not change yet: the digest shows the mismatch.
    @Test func anOlderAccountBesideANewerDigestIsNotReady() throws {
        let values = FakeValues(), store = CentralStore(values: values)
        var recoloured = alpha
        store.save([alpha, beta])
        let old = try #require(values.data(forKey: CentralStore.key(alpha.id)))
        recoloured.colour = "teal"
        store.save([recoloured, beta])
        values.set(old, forKey: CentralStore.key(alpha.id))
        #expect(store.stored() == .arriving)
    }

    @Test func aPartListNeverWrites() async {
        let hosts = FakeHosts(), values = FakeValues(), store = CentralStore(values: values)
        hosts.files = ["a": AccountsFile(accounts: [alpha, beta]).encoded()]
        store.save([alpha, beta])
        _ = await AccountSync.run(hosts: ["a"], store: store, client: hosts)
        values.removeObject(forKey: CentralStore.key(beta.id))
        let out = await AccountSync.run(hosts: ["a"], store: store, client: hosts)
        #expect(out.map(\.state) == [.waiting])
        #expect(hosts.puts.isEmpty)
    }

    /// Two devices that edit different accounts leave a digest neither list matches. Once iCloud
    /// has gone quiet, the accounts that are all here are the list.
    @Test func aSettledMismatchHeals() async {
        let hosts = FakeHosts(), values = FakeValues(), store = CentralStore(values: values)
        hosts.files = ["a": AccountsFile(accounts: [alpha, beta]).encoded()]
        store.save([alpha, beta])
        values.set(Data("other".utf8), forKey: CentralStore.digestKey)
        #expect(await AccountSync.run(hosts: ["a"], store: store, client: hosts, settled: false).map(\.state) == [.waiting])
        #expect(await AccountSync.run(hosts: ["a"], store: store, client: hosts, settled: true).map(\.state) == [.inStep])
        #expect(store.stored() == .ready([alpha, beta]))
    }

    /// A key that never arrives is never settled away: that account could be real.
    @Test func aMissingKeyDoesNotSettle() async {
        let hosts = FakeHosts(), values = FakeValues(), store = CentralStore(values: values)
        store.save([alpha, beta])
        values.removeObject(forKey: CentralStore.key(beta.id))
        hosts.files = ["a": AccountsFile(accounts: [alpha]).encoded()]
        #expect(await AccountSync.run(hosts: ["a"], store: store, client: hosts, settled: true).map(\.state) == [.waiting])
        #expect(hosts.puts.isEmpty)
    }

    /// F23: iCloud's write rate is limited, and an unchanged key must not wake other devices.
    @Test func aPassInStepWritesNothingToICloud() async {
        let hosts = FakeHosts(), values = FakeValues(), store = CentralStore(values: values)
        hosts.files = ["a": AccountsFile(accounts: [alpha]).encoded()]
        store.save([alpha])
        _ = await AccountSync.run(hosts: ["a"], store: store, client: hosts)
        let before = values.written.count
        _ = await AccountSync.run(hosts: ["a"], store: store, client: hosts)
        #expect(values.written.count == before)
    }

    /// F37: a sleeping host must not hold up the others.
    @Test func hostsAreReadAtOneTime() async {
        let hosts = FakeHosts(), store = CentralStore(values: FakeValues())
        hosts.delay = .milliseconds(50)
        hosts.files = ["a": AccountsFile(accounts: [alpha]).encoded(), "b": AccountsFile(accounts: [alpha]).encoded()]
        store.save([alpha])
        _ = await AccountSync.run(hosts: ["a", "b"], store: store, client: hosts)
        #expect(hosts.peak == 2)
    }

    @Test func hostsAreWrittenAtOneTimeInHostOrder() async {
        let hosts = FakeHosts(), store = CentralStore(values: FakeValues())
        hosts.delay = .milliseconds(50)
        let old = AccountsFile(accounts: [beta]).encoded()
        hosts.files = ["a": old, "b": old, "c": AccountsFile(accounts: [alpha]).encoded()]
        store.save([alpha, beta])
        let out = await AccountSync.run(hosts: ["a", "b", "c"], store: store, client: hosts)
        #expect(hosts.peak == 3)
        #expect(out.map(\.host) == ["a", "b", "c"] && out.map(\.state) == [.written, .written, .written])
    }
}
