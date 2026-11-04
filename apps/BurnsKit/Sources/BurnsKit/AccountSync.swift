import Foundation

/// A host's accounts file over HTTP.
public protocol AccountHosts: Sendable {
    func get(_ host: String) async throws -> (data: Data, etag: String)
    func put(_ host: String, data: Data, etag: String) async throws -> String
}

/// Copies the central list to the hosts (docs/SPEC.md, "Accounts in the apps").
public enum AccountSync {
    public enum Decision: Equatable, Sendable { case inStep, write, review }

    /// What one run did about one host.
    public enum State: Equatable, Sendable {
        case inStep, written, unreachable, unreadable
        /// iCloud has not delivered the whole list, so the host was left alone.
        case waiting
        /// The host's file differs and writing could lose something: held, with what the host has.
        case review([CentralAccount])
        case refused(Int)
    }

    public struct HostSync: Equatable, Sendable {
        public var host: String
        public var state: State
    }

    public struct Conflict: Equatable, Sendable {
        public var id: AccountID
        public var homes: [String]
    }

    /// The central list wins, but never silently: a host is written only when that loses nothing.
    /// That is when the file is what was last written, or holds only accounts the list already has.
    /// An empty list never writes, so a store that came back empty cannot wipe a host. Fields the
    /// list has not seen are held even when the hash matches: a host in step before the app knew
    /// `launch` holds a matching hash, and writing would delete its credential reference.
    public static func decide(central: [CentralAccount], host: [CentralAccount], hostHash: String, written: String?) -> Decision {
        if central == host { return .inStep }
        if central.isEmpty || holdsUnseenFields(central: central, host: host) { return .review }
        if hostHash == written || host.allSatisfy(central.contains) { return .write }
        return .review
    }

    /// An account on both sides whose host copy has fields that the list's copy does not match.
    /// An account gone from the list is a removal, and its fields go with it.
    static func holdsUnseenFields(central: [CentralAccount], host: [CentralAccount]) -> Bool {
        host.contains { h in !h.extra.isEmpty && central.contains { $0.id == h.id && $0.extra != h.extra } }
    }

    /// Adds to each listed account the fields its host copies have and it lacks, when every host
    /// that has a field agrees on its value. The apps never edit those fields, so the list missing
    /// one means it was added on a host, by hand or by an agent. Taking it loses nothing, and
    /// otherwise every host stays held: overwrite would drop the field, and import would undo the
    /// list's own edits. Hosts that disagree keep the field out, and stay held for the operator.
    static func adopting(_ central: [CentralAccount], from hosts: [[CentralAccount]]) -> [CentralAccount] {
        central.map { account in
            var found: [String: JSONValue] = [:], disputed: Set<String> = []
            for copy in hosts.joined() where copy.id == account.id {
                for (field, value) in copy.extra where account.extra[field] == nil {
                    if let seen = found[field], seen != value { disputed.insert(field) }
                    found[field] = found[field] ?? value
                }
            }
            var out = account
            for (field, value) in found where !disputed.contains(field) { out.extra[field] = value }
            return out
        }
    }

    /// First use: every host's accounts, matched by provider and name, in the order first seen. The
    /// first home wins here. The host with the other one then differs, so it comes up for review.
    public static func importing(_ hosts: [[CentralAccount]]) -> (accounts: [CentralAccount], conflicts: [Conflict]) {
        var accounts: [CentralAccount] = [], conflicts: [Conflict] = []
        for account in hosts.joined() {
            guard let i = accounts.firstIndex(where: { $0.id == account.id }) else {
                accounts.append(account)
                continue
            }
            guard accounts[i].home != account.home else { continue }
            if let c = conflicts.firstIndex(where: { $0.id == account.id }) {
                if !conflicts[c].homes.contains(account.home) { conflicts[c].homes.append(account.home) }
            } else {
                conflicts.append(Conflict(id: account.id, homes: [accounts[i].home, account.home]))
            }
        }
        return (accounts, conflicts)
    }

    /// One pass over the hosts, all at one time. A host that cannot be reached catches up on a later
    /// pass. `settled` is false while iCloud may still be delivering: a new device must not take the
    /// hosts' accounts over a list it has not downloaded, and a part of a list must not be copied.
    public static func run(hosts: [String], store: CentralStore, client: some AccountHosts, settled: Bool = true) async -> [HostSync] {
        let read = await reads(hosts, client)
        let stored: [CentralAccount]
        switch store.stored() {
        case .ready(let list): stored = list
        case .arriving:
            guard settled, let list = store.settle() else { return hosts.map { HostSync(host: $0, state: .waiting) } }
            stored = list
        case .never:
            guard settled else { return hosts.map { HostSync(host: $0, state: .waiting) } }
            let answered = hosts.compactMap { host in (try? read[host]?.get())?.accounts }
            guard !answered.isEmpty else { return hosts.map { HostSync(host: $0, state: failure(read[$0])) } }
            stored = importing(answered).accounts
            store.save(stored)
        }
        let central = adopting(stored, from: hosts.compactMap { (try? read[$0]?.get())?.accounts })
        if central != stored { store.save(central) }
        let states = await withTaskGroup(of: (String, State).self) { group in
            for host in hosts {
                let answer = read[host]
                group.addTask {
                    guard case .success(let file)? = answer else { return (host, failure(answer)) }
                    return (host, await apply(central, file, host, store, client))
                }
            }
            return await group.reduce(into: [String: State]()) { $0[$1.0] = $1.1 }
        }
        return hosts.map { HostSync(host: $0, state: states[$0] ?? .unreachable) }
    }

    /// Replaces a held host's file with the list, after the operator chose that in the review.
    public static func overwrite(host: String, store: CentralStore, client: some AccountHosts) async -> State {
        guard let central = store.load(), !central.isEmpty else { return .waiting }
        switch await fetch(host, client) {
        case .success(let file): return await write(central, file.etag, host, store, client)
        case let failed: return failure(failed)
        }
    }

    private static func reads(_ hosts: [String], _ client: some AccountHosts) async -> [String: Result<(accounts: [CentralAccount], etag: String), Failure>] {
        await withTaskGroup(of: (String, Result<(accounts: [CentralAccount], etag: String), Failure>).self) { group in
            for host in hosts { group.addTask { (host, await fetch(host, client)) } }
            return await group.reduce(into: [:]) { $0[$1.0] = $1.1 }
        }
    }

    enum Failure: Error { case unreachable, unreadable, refused(Int) }

    private static func fetch(_ host: String, _ client: some AccountHosts) async -> Result<(accounts: [CentralAccount], etag: String), Failure> {
        do {
            let (data, etag) = try await client.get(host)
            guard let file = try? JSONDecoder().decode(AccountsFile.self, from: data) else { return .failure(.unreadable) }
            return .success((file.accounts, etag))
        } catch let e as HostFetcher.Refused {
            return .failure(.refused(e.status))
        } catch {
            return .failure(.unreachable)
        }
    }

    private static func failure(_ r: Result<(accounts: [CentralAccount], etag: String), Failure>?) -> State {
        switch r {
        case .failure(.unreadable)?: .unreadable
        case .failure(.refused(let status))?: .refused(status)
        default: .unreachable
        }
    }

    private static func apply(_ central: [CentralAccount], _ file: (accounts: [CentralAccount], etag: String), _ host: String,
                              _ store: CentralStore, _ client: some AccountHosts) async -> State {
        let written = store.written(host: host)
        switch decide(central: central, host: file.accounts, hostHash: file.etag, written: written) {
        case .inStep:
            if written != file.etag { store.setWritten(file.etag, host: host) } // iCloud limits its write rate.
            return .inStep
        case .review:
            return .review(file.accounts)
        case .write:
            return await write(central, file.etag, host, store, client)
        }
    }

    private static func write(_ central: [CentralAccount], _ etag: String, _ host: String, _ store: CentralStore, _ client: some AccountHosts) async -> State {
        do {
            store.setWritten(try await client.put(host, data: AccountsFile(accounts: central).encoded(), etag: etag), host: host)
            return .written
        } catch let e as HostFetcher.Refused {
            return .refused(e.status)
        } catch {
            return .unreachable
        }
    }
}

extension AccountSync.State {
    /// Settings' line for a host.
    public var words: String {
        switch self {
        case .inStep: "Accounts in step"
        case .written: "Accounts updated"
        case .unreachable: "Accounts not checked"
        case .waiting: "Accounts waiting for iCloud"
        case .unreadable: "Accounts file unreadable"
        case .review: "Accounts differ, not overwritten"
        case .refused(let status): "Accounts refused (\(status))"
        }
    }
}

/// Reads and writes one smithersd's `/accounts` over the tailnet.
public struct HostAccounts: AccountHosts {
    public init() {}

    private func request(_ host: String) throws -> URLRequest {
        guard let url = URL(string: "http://\(host):\(HostFetcher.port)/accounts") else { throw URLError(.badURL) }
        var request = URLRequest(url: url, timeoutInterval: HostFetcher.timeout)
        request.cachePolicy = .reloadIgnoringLocalCacheData
        return request
    }

    private func send(_ request: URLRequest) async throws -> (Data, String) {
        let (data, response) = try await HostFetcher.session.data(for: request)
        guard let http = response as? HTTPURLResponse else { throw URLError(.badServerResponse) }
        guard http.statusCode == 200 else { throw HostFetcher.Refused(status: http.statusCode) }
        return (data, http.value(forHTTPHeaderField: "ETag") ?? "")
    }

    public func get(_ host: String) async throws -> (data: Data, etag: String) {
        let (data, etag) = try await send(try request(host))
        return (data, etag)
    }

    public func put(_ host: String, data: Data, etag: String) async throws -> String {
        var request = try request(host)
        request.httpMethod = "PUT"
        request.httpBody = data
        request.setValue(etag, forHTTPHeaderField: "If-Match")
        return try await send(request).1
    }
}
