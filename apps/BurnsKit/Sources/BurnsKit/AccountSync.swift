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
    /// An empty list never writes, so a store that came back empty cannot wipe a host.
    public static func decide(central: [CentralAccount], host: [CentralAccount], hostHash: String, written: String?) -> Decision {
        if central == host { return .inStep }
        if central.isEmpty { return .review }
        if hostHash == written || host.allSatisfy(central.contains) { return .write }
        return .review
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

    /// One pass over the hosts. A host that cannot be reached catches up on a later pass.
    /// `mayImport` is false until iCloud has had time to deliver: a new device must not take the
    /// hosts' accounts over a list it has not downloaded yet.
    public static func run(hosts: [String], store: CentralStore, client: some AccountHosts, mayImport: Bool = true) async -> [HostSync] {
        var read: [String: Result<(accounts: [CentralAccount], etag: String), Failure>] = [:]
        for host in hosts { read[host] = await fetch(host, client) }
        var central = store.load()
        if central == nil {
            guard mayImport else { return hosts.map { HostSync(host: $0, state: .unreachable) } }
            let answered = hosts.compactMap { host in (try? read[host]?.get())?.accounts }
            guard !answered.isEmpty else { return hosts.map { HostSync(host: $0, state: failure(read[$0])) } }
            let list = importing(answered).accounts
            store.save(list)
            central = list
        }
        var out: [HostSync] = []
        for host in hosts {
            guard case .success(let file)? = read[host], let central else {
                out.append(HostSync(host: host, state: failure(read[host])))
                continue
            }
            out.append(HostSync(host: host, state: await apply(central, file, host, store, client)))
        }
        return out
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
        switch decide(central: central, host: file.accounts, hostHash: file.etag, written: store.written(host: host)) {
        case .inStep:
            store.setWritten(file.etag, host: host)
            return .inStep
        case .review:
            return .review(file.accounts)
        case .write:
            do {
                store.setWritten(try await client.put(host, data: AccountsFile(accounts: central).encoded(), etag: file.etag), host: host)
                return .written
            } catch let e as HostFetcher.Refused {
                return .refused(e.status)
            } catch {
                return .unreachable
            }
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
        let (data, response) = try await URLSession(configuration: .ephemeral).data(for: request)
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
