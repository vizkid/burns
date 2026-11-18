import Foundation
import SwiftUI

/// A theme from the Rex app's design tokens, as the Theme picker lists it (docs/SPEC.md, "Pane
/// themes"). Launch reads the full palette itself; the picker needs only the terminal's two main
/// colours as swatches.
public struct RexTheme: Identifiable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let background: String
    public let foreground: String

    static let bundlePath = "Contents/Resources/SuperUI_SuperDesignTokens.bundle/Contents/Resources/themes.json"
    static let apps = ["Rex.app", "Rex Beta.app"]

    private struct Tone: Decodable { let red, green, blue: Double }
    private struct Entry: Decodable {
        let id: String
        let name: String?
        let semantic: [String: Tone]?
    }

    /// The themes in a themes file, sorted by name. A theme without terminal colours is left out.
    public static func read(_ data: Data) throws -> [RexTheme] {
        try JSONDecoder().decode([Entry].self, from: data).compactMap { e in
            guard let bg = e.semantic?["terminalBackground"], let fg = e.semantic?["terminalForeground"] else { return nil }
            return RexTheme(id: e.id, name: e.name ?? e.id, background: hex(bg), foreground: hex(fg))
        }
        .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }

    /// The first Rex app's themes in these folders, or none.
    public static func installed(in dirs: [URL]) -> [RexTheme] {
        for dir in dirs {
            for app in apps {
                let url = dir.appendingPathComponent(app).appendingPathComponent(bundlePath)
                if let data = try? Data(contentsOf: url), let themes = try? read(data) { return themes }
            }
        }
        return []
    }

    /// This Mac's themes, read once: the file is a few megabytes.
    public static let onThisMac: [RexTheme] = installed(in: [
        URL(fileURLWithPath: "/Applications"),
        FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Applications"),
    ])

    /// The same conversion launch makes: round(clamp(c) × 255).
    private static func hex(_ t: Tone) -> String {
        let b = { (c: Double) in Int((min(1, max(0, c)) * 255).rounded()) }
        return String(format: "#%02x%02x%02x", b(t.red), b(t.green), b(t.blue))
    }
}

extension Color {
    /// A colour from `#rrggbb`, in Display P3 as Rex reads it.
    init(rexHex: String) {
        let v = UInt32(rexHex.dropFirst(), radix: 16) ?? 0
        self.init(.displayP3, red: Double(v >> 16 & 0xff) / 255, green: Double(v >> 8 & 0xff) / 255, blue: Double(v & 0xff) / 255)
    }
}

/// The Theme row's menu: each theme's name with its background and foreground, and None.
struct ThemePicker: View {
    let current: String?
    let themes: [RexTheme]
    let pick: (String?) -> Void

    var body: some View {
        if themes.isEmpty {
            Text(current ?? "None").foregroundStyle(.secondary)
        } else {
            Menu {
                Button("None") { pick(nil) }
                Divider()
                ForEach(themes) { t in
                    Button { pick(t.id) } label: {
                        Label { Text(t.name) } icon: { image(t) }
                    }
                }
            } label: {
                if let t = themes.first(where: { $0.id == current }) {
                    HStack(spacing: 6) { swatch(t); Text(t.name) }
                } else {
                    Text(current ?? "None")
                }
            }
            .fixedSize()
        }
    }

    /// The swatch as an image: a Mac menu item draws an image icon and drops any other view.
    @MainActor func image(_ t: RexTheme) -> Image {
        let renderer = ImageRenderer(content: swatch(t))
        renderer.scale = 2
        #if os(macOS)
        return renderer.nsImage.map { Image(nsImage: $0) } ?? Image(systemName: "square.fill")
        #else
        return renderer.uiImage.map { Image(uiImage: $0) } ?? Image(systemName: "square.fill")
        #endif
    }

    /// A background square with a foreground dot, as the terminal would show them.
    func swatch(_ t: RexTheme) -> some View {
        RoundedRectangle(cornerRadius: 3).fill(Color(rexHex: t.background))
            .frame(width: 16, height: 16)
            .overlay { Circle().fill(Color(rexHex: t.foreground)).frame(width: 6, height: 6) }
            .overlay { RoundedRectangle(cornerRadius: 3).strokeBorder(.secondary.opacity(0.4), lineWidth: 0.5) }
    }
}
