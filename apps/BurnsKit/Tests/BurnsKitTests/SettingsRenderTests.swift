#if os(macOS)
import AppKit
import SwiftUI
import Testing
@testable import BurnsKit

/// Renders the account screens to look at them. Forms are AppKit underneath, which ImageRenderer
/// cannot draw, so these go through a hosting view. Runs only with BURNS_RENDER_DIR set; a
/// `<host>-accounts.json` there supplies the accounts.
@MainActor
struct SettingsRenderTests {
    @Test(.enabled(if: ProcessInfo.processInfo.environment["BURNS_RENDER_DIR"] != nil))
    func renderAccountScreens() throws {
        let dir = URL(fileURLWithPath: ProcessInfo.processInfo.environment["BURNS_RENDER_DIR"]!)
        let name = try FileManager.default.contentsOfDirectory(atPath: dir.path).first { $0.hasSuffix("-accounts.json") }
        let accounts = try JSONDecoder().decode(AccountsFile.self, from: Data(contentsOf: dir.appendingPathComponent(name ?? ""))).accounts
        let setUp = Dictionary(uniqueKeysWithValues: accounts.map { ($0.id, ["host-1", "host-2", "host-3"]) })
        let list = Binding.constant(accounts)
        let first = accounts[0].name
        var changed = accounts
        changed.removeLast()
        for (scheme, tag) in [(NSAppearance.Name.darkAqua, "dark"), (.aqua, "light")] {
            try save(AccountsPane(accounts: list, setUp: setUp, selected: first), scheme, 640, 480, dir, "settings-accounts-\(tag).png")
            try save(EmojiGrid { _ in }, scheme, 320, 380, dir, "settings-emoji-\(tag).png")
            try save(NavigationStack { AccountsForm(accounts: list, setUp: setUp) }, scheme, 390, 340, dir, "settings-phone-\(tag).png")
            try save(AddAccount(accounts: list), scheme, 420, 300, dir, "settings-add-\(tag).png")
            try save(NavigationStack { AccountReview(host: "host-1", central: changed, onHost: accounts, keepHost: {}, keepList: {}) },
                     scheme, 460, 340, dir, "settings-review-\(tag).png")
        }
    }

    func save(_ view: some View, _ name: NSAppearance.Name, _ w: CGFloat, _ h: CGFloat, _ dir: URL, _ file: String) throws {
        let host = NSHostingView(rootView: view.frame(width: w, height: h))
        let rect = NSRect(x: 0, y: 0, width: w, height: h)
        host.frame = rect
        host.appearance = NSAppearance(named: name)
        let window = NSWindow(contentRect: rect, styleMask: [.borderless], backing: .buffered, defer: false)
        window.contentView = host
        window.appearance = NSAppearance(named: name)
        window.orderFrontRegardless()
        RunLoop.main.run(until: Date().addingTimeInterval(0.6))
        host.layoutSubtreeIfNeeded()
        guard let rep = host.bitmapImageRepForCachingDisplay(in: rect) else { throw CocoaError(.fileWriteUnknown) }
        host.cacheDisplay(in: rect, to: rep)
        window.orderOut(nil)
        try rep.representation(using: .png, properties: [:])!.write(to: dir.appendingPathComponent(file))
    }
}
#endif
