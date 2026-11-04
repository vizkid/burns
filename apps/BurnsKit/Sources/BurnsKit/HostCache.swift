import Foundation

/// One host's last answer: its usage and its readings, with when it was fetched.
public struct HostSnapshot: Codable, Equatable, Sendable {
    /// The host's address as settings list it, such as `host.tailnet.ts.net`.
    public var address: String
    public var fetchedAt: Date
    public var usage: UsageAnswer
    public var readings: [Reading]

    public init(address: String, fetchedAt: Date, usage: UsageAnswer, readings: [Reading]) {
        self.address = address; self.fetchedAt = fetchedAt; self.usage = usage; self.readings = readings
    }
}

/// What a refresh learned about one host. A host that did not answer keeps its cached snapshot,
/// with the error that says why it is not fresh.
public struct HostStatus: Equatable, Sendable {
    public var address: String
    public var snapshot: HostSnapshot?
    public var error: String?
    public init(address: String, snapshot: HostSnapshot?, error: String?) { self.address = address; self.snapshot = snapshot; self.error = error }
}

/// Each host's last answer, kept in the app group so the widgets and the app share it
/// (docs/SPEC.md, "Apps and widgets").
public final class HostCache: @unchecked Sendable {
    public static let key = "burns.hostSnapshots"
    let defaults: UserDefaults
    private let lock = NSLock()
    /// The newest refresh, so the next one starts after it.
    private var newest: Task<[HostStatus], Never>?

    public init(defaults: UserDefaults) { self.defaults = defaults }

    public func load() -> [String: HostSnapshot] {
        guard let data = defaults.data(forKey: Self.key) else { return [:] }
        return (try? JSONDecoder().decode([String: HostSnapshot].self, from: data)) ?? [:]
    }

    public func save(_ snapshots: [String: HostSnapshot]) {
        // Foundation's own date encoding keeps every digit of the hosts' times.
        if let data = try? JSONEncoder().encode(snapshots) { defaults.set(data, forKey: Self.key) }
    }

    /// Fetches every host at once and stores the answers. The result is in `hosts` order, and
    /// hosts no longer listed leave the cache. Refreshes run one after another, each from the last
    /// one's result: an older, slower refresh must not finish after a newer one.
    public func refresh(hosts: [String], fetch: @escaping @Sendable (String) async throws -> HostSnapshot) async -> [HostStatus] {
        let run = lock.withLock {
            let before = newest
            let run = Task { [self] in
                _ = await before?.value
                return await pass(hosts: hosts, fetch: fetch)
            }
            newest = run
            return run
        }
        return await run.value
    }

    private func pass(hosts: [String], fetch: @escaping @Sendable (String) async throws -> HostSnapshot) async -> [HostStatus] {
        let cached = load()
        let results = await withTaskGroup(of: (String, Result<HostSnapshot, Error>).self) { group in
            for host in hosts {
                group.addTask {
                    do { return (host, .success(try await fetch(host))) } catch { return (host, .failure(error)) }
                }
            }
            var out: [String: Result<HostSnapshot, Error>] = [:]
            for await (host, result) in group { out[host] = result }
            return out
        }
        var kept: [String: HostSnapshot] = [:]
        let status = hosts.map { host -> HostStatus in
            switch results[host] {
            case .success(let snap)?:
                kept[host] = snap
                return HostStatus(address: host, snapshot: snap, error: nil)
            case .failure(let error)?:
                kept[host] = cached[host]
                return HostStatus(address: host, snapshot: cached[host], error: Self.wasCancelled(error) ? nil : Self.describe(error))
            case nil:
                return HostStatus(address: host, snapshot: cached[host], error: "not fetched")
            }
        }
        // Another process, a widget, shares this key: a newer answer it stored stays.
        for (host, theirs) in load() where kept[host] != nil && theirs.fetchedAt > kept[host]?.fetchedAt ?? .distantPast { kept[host] = theirs }
        save(kept)
        return status
    }

    /// A cancelled read says nothing about the host.
    static func wasCancelled(_ error: Error) -> Bool {
        error is CancellationError || (error as? URLError)?.code == .cancelled
    }

    static func describe(_ error: Error) -> String {
        if let e = error as? URLError {
            switch e.code {
            case .timedOut: return "timed out"
            case .cannotFindHost, .dnsLookupFailed: return "host not found"
            case .cannotConnectToHost, .networkConnectionLost, .notConnectedToInternet: return "unreachable"
            default: break
            }
        }
        if let e = error as? HostFetcher.Refused { return "answered \(e.status)" }
        return "failed"
    }
}

/// Reads one smithersd over the tailnet: `/usage` and `/usage/readings` on port 9766.
public struct HostFetcher: Sendable {
    public struct Refused: Error { public var status: Int }
    public static let port = 9766
    public static let timeout: TimeInterval = 4

    /// One session for every request, so connections are reused between polls.
    public static let session: URLSession = {
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = timeout
        config.timeoutIntervalForResource = timeout * 2
        config.waitsForConnectivity = false
        return URLSession(configuration: config)
    }()

    private let session: URLSession

    public init(session: URLSession = HostFetcher.session) { self.session = session }

    public func fetch(_ address: String) async throws -> HostSnapshot {
        async let usage = get(address, "/usage", as: UsageAnswer.self)
        async let readings = get(address, "/usage/readings", as: ReadingsAnswer.self)
        return HostSnapshot(address: address, fetchedAt: Date(), usage: try await usage, readings: try await readings.readings)
    }

    /// Read when the window opens, not on the poll: the rings need quota every minute, and panes only
    /// while someone is looking.
    public func panes(_ address: String) async throws -> PanesAnswer {
        try await get(address, "/usage/panes", as: PanesAnswer.self)
    }

    /// Read when the window opens, as panes are: money changes slowly, and an answer holds a week of hours.
    public func cost(_ address: String) async throws -> CostAnswer {
        try await get(address, "/usage/cost", as: CostAnswer.self)
    }

    /// What each account can run on the host. Empty when the host did not answer, which callers
    /// treat as unknown.
    public func models(_ address: String) async -> [AccountID: ModelEntry] {
        let list = (try? await get(address, "/models", as: ModelsAnswer.self))?.accounts ?? []
        return Dictionary(list.map { (AccountID(provider: $0.provider, name: $0.account), $0) }) { first, _ in first }
    }

    /// `GET /dirs`: the host's folders whose path starts with `prefix`, relative to its home folder.
    /// Nil when the host did not answer, or runs a smithersd without the route.
    public func dirs(_ address: String, prefix: String) async -> DirsAnswer? {
        var q = URLComponents()
        q.queryItems = [URLQueryItem(name: "prefix", value: prefix)]
        return try? await get(address, "/dirs?" + (q.percentEncodedQuery ?? ""), as: DirsAnswer.self)
    }

    func get<T: Decodable>(_ address: String, _ path: String, as: T.Type) async throws -> T {
        guard let url = URL(string: "http://\(address):\(Self.port)\(path)") else { throw URLError(.badURL) }
        let (data, response) = try await session.data(from: url)
        if let http = response as? HTTPURLResponse, http.statusCode != 200 { throw Refused(status: http.statusCode) }
        return try Wire.decoder.decode(T.self, from: data)
    }
}
