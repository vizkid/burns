import BurnsKit
import Foundation

/// Runs `rex`. Each argument passes as it is, with no shell.
enum Rex {
    private static let finder = Finder()

    /// Output, or an error's text. A launch has no `limit`: stopping `rex` partway could leave a tab
    /// that the panel then reports as failed.
    static func run(_ args: [String], limit: TimeInterval? = nil) async -> Result<Data, CommandFailure> {
        guard let path = await finder.path() else { return .failure(CommandFailure(text: "rex was not found")) }
        return await Command.run(path, args, limit: limit)
    }

    static func json<T: Decodable>(_ type: T.Type, _ args: [String]) async -> T? {
        guard case .success(let data) = await run(args, limit: 5) else { return nil }
        return try? JSONDecoder().decode(type, from: data)
    }

    @MainActor private static var listing: Task<RexHosts?, Never>?

    /// `rex hosts list`. Callers at one time share one process: a window opening asks twice.
    @MainActor static func hosts() async -> RexHosts? {
        if let listing { return await listing.value }
        let task = Task { await json(RexHosts.self, ["hosts", "list", "--json"]) }
        listing = task
        let answer = await task.value
        listing = nil
        return answer
    }

    /// Finds `rex` through a login shell, once. The app starts without the shell's PATH. Only a
    /// success is kept: a slow start must not read as "not found" until the app restarts.
    private actor Finder {
        private var found: String?
        private var searching: Task<String?, Never>?
        private var nextTry = Date.distantPast
        private static let retryAfter: TimeInterval = 30

        func path() async -> String? {
            if let found { return found }
            if let searching { return await searching.value }
            guard Date() >= nextTry else { return nil }
            let task = Task { await Command.locate("rex") }
            searching = task
            let answer = await task.value
            searching = nil
            found = answer
            if answer == nil { nextTry = Date().addingTimeInterval(Self.retryAfter) }
            return answer
        }
    }
}
