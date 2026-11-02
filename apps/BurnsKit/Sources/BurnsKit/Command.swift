#if os(macOS)
import Foundation

public struct CommandFailure: Error, Equatable {
    public let text: String
    public init(text: String) { self.text = text }
}

/// Runs a program with no shell. Output is read while it runs: a pipe holds only 64 KiB, so a
/// program with more to say would block on writing and never exit.
public enum Command {
    /// A process that closes its pipes lags its exit by this much at most. A child that inherited
    /// them and lives on must not hold the answer back.
    static let drainLimit: TimeInterval = 1

    /// Output, or the error's text. A `limit` stops the program after that many seconds.
    public static func run(_ path: String, _ args: [String], limit: TimeInterval? = nil) async -> Result<Data, CommandFailure> {
        await withCheckedContinuation { done in Running(path, args).start(limit: limit) { done.resume(returning: $0) } }
    }

    private final class Running: @unchecked Sendable {
        private let process = Process()
        private let out = Pipe(), err = Pipe()
        private let lock = NSLock()
        private var output = Data(), errors = Data()

        init(_ path: String, _ args: [String]) {
            process.executableURL = URL(fileURLWithPath: path)
            process.arguments = args
            process.standardInput = FileHandle.nullDevice
            process.standardOutput = out
            process.standardError = err
        }

        private func read(_ pipe: Pipe, _ drained: DispatchGroup, into append: @escaping @Sendable (Data) -> Void) {
            drained.enter()
            pipe.fileHandleForReading.readabilityHandler = { handle in
                let data = handle.availableData
                if data.isEmpty {
                    handle.readabilityHandler = nil
                    drained.leave()
                } else {
                    append(data)
                }
            }
        }

        private func stopReading() {
            out.fileHandleForReading.readabilityHandler = nil
            err.fileHandleForReading.readabilityHandler = nil
        }

        private func result() -> Result<Data, CommandFailure> {
            lock.withLock {
                process.terminationStatus == 0 ? .success(output) : .failure(CommandFailure(text: String(decoding: errors, as: UTF8.self)))
            }
        }

        func start(limit: TimeInterval?, finish: @escaping @Sendable (Result<Data, CommandFailure>) -> Void) {
            let drained = DispatchGroup()
            read(out, drained) { [self] data in lock.withLock { output.append(data) } }
            read(err, drained) { [self] data in lock.withLock { errors.append(data) } }
            process.terminationHandler = { [self] _ in
                DispatchQueue.global().async { [self] in
                    _ = drained.wait(timeout: .now() + Command.drainLimit)
                    stopReading()
                    finish(result())
                }
            }
            do {
                try process.run()
            } catch {
                stopReading()
                finish(.failure(CommandFailure(text: "\(error)")))
                return
            }
            if let limit {
                DispatchQueue.global().asyncAfter(deadline: .now() + limit) { [self] in
                    if process.isRunning { process.terminate() }
                }
            }
        }
    }

    /// Asks a login shell where a program is: the app starts without the shell's PATH. The marker
    /// picks the answer out of whatever the shell's start files print around it.
    public static func locate(_ program: String) async -> String? {
        let script = "print -r -- \"\(marker)$(whence -p \(program))\""
        guard case .success(let data) = await run("/bin/zsh", ["-ilc", script], limit: searchLimit) else { return nil }
        return located(data)
    }

    static let marker = "burns-found:"
    static let searchLimit: TimeInterval = 10

    static func located(_ output: Data) -> String? {
        let found = String(decoding: output, as: UTF8.self).split(whereSeparator: \.isNewline)
            .last { $0.hasPrefix(marker) }?.dropFirst(marker.count).trimmingCharacters(in: .whitespaces)
        return found?.isEmpty == false ? found : nil
    }
}
#endif
