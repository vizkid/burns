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
    /// hosts no longer listed leave the cache.
    public func refresh(hosts: [String], fetch: @escaping @Sendable (String) async throws -> HostSnapshot) async -> [HostStatus] {
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
                return HostStatus(address: host, snapshot: cached[host], error: Self.describe(error))
            case nil:
                return HostStatus(address: host, snapshot: cached[host], error: "not fetched")
            }
        }
        save(kept)
        return status
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

    public init() {}

    public func fetch(_ address: String) async throws -> HostSnapshot {
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = Self.timeout
        config.timeoutIntervalForResource = Self.timeout * 2
        config.waitsForConnectivity = false
        let session = URLSession(configuration: config)
        defer { session.finishTasksAndInvalidate() }
        async let usage = get(session, address, "/usage", as: UsageAnswer.self)
        async let readings = get(session, address, "/usage/readings", as: ReadingsAnswer.self)
        return HostSnapshot(address: address, fetchedAt: Date(), usage: try await usage, readings: try await readings.readings)
    }

    func get<T: Decodable>(_ session: URLSession, _ address: String, _ path: String, as: T.Type) async throws -> T {
        guard let url = URL(string: "http://\(address):\(Self.port)\(path)") else { throw URLError(.badURL) }
        let (data, response) = try await session.data(from: url)
        if let http = response as? HTTPURLResponse, http.statusCode != 200 { throw Refused(status: http.statusCode) }
        return try Wire.decoder.decode(T.self, from: data)
    }
}
