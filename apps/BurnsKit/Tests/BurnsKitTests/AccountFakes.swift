import Foundation
@testable import BurnsKit

/// A host's accounts file and the requests made of it. Locked: a pass reads hosts in parallel.
final class FakeHosts: AccountHosts, @unchecked Sendable {
    private let lock = NSLock()
    private var state = (files: [String: Data](), unreachable: Set<String>(), puts: [String](), refuse: Int?.none, active: 0, peak: 0)
    /// Each request waits this long, so that overlapping requests can be seen.
    var delay: Duration?

    var files: [String: Data] { get { lock.withLock { state.files } } set { lock.withLock { state.files = newValue } } }
    var unreachable: Set<String> { get { lock.withLock { state.unreachable } } set { lock.withLock { state.unreachable = newValue } } }
    var puts: [String] { lock.withLock { state.puts } }
    var refuse: Int? { get { lock.withLock { state.refuse } } set { lock.withLock { state.refuse = newValue } } }
    /// The most requests that ran at one time.
    var peak: Int { lock.withLock { state.peak } }

    private func enter() async {
        lock.withLock { state.active += 1; state.peak = max(state.peak, state.active) }
        if let delay { try? await Task.sleep(for: delay) }
        lock.withLock { state.active -= 1 }
    }

    func get(_ host: String) async throws -> (data: Data, etag: String) {
        await enter()
        if unreachable.contains(host) { throw URLError(.cannotConnectToHost) }
        let data = files[host] ?? Data()
        return (data, Self.tag(data))
    }

    func put(_ host: String, data: Data, etag: String) async throws -> String {
        await enter()
        if let refuse { throw HostFetcher.Refused(status: refuse) }
        lock.withLock { state.puts.append(host); state.files[host] = data }
        return Self.tag(data)
    }

    static func tag(_ d: Data) -> String { "\"\(d.count)-\(d.hashValue)\"" }
}

final class FakeValues: KeyValues, @unchecked Sendable {
    private let lock = NSLock()
    private var stored: [String: Data] = [:]
    private var writes: [String] = []

    var values: [String: Data] { get { lock.withLock { stored } } set { lock.withLock { stored = newValue } } }
    /// Every key written, in order.
    var written: [String] { lock.withLock { writes } }

    func data(forKey key: String) -> Data? { lock.withLock { stored[key] } }
    func set(_ value: Data?, forKey key: String) { lock.withLock { stored[key] = value; writes.append(key) } }
    func removeObject(forKey key: String) { lock.withLock { stored[key] = nil } }
}
