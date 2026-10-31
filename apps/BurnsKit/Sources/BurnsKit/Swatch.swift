import SwiftUI
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif

/// An account name's colour (docs/SPEC.md, "Accounts in the apps"): a fixed pair, one value for dark
/// backgrounds and a darker one for light. Purple and teal are Penfold's hand-tuned pairs. There is
/// no yellow, amber or red: those are the pace colours.
public enum Swatch: String, CaseIterable, Codable, Sendable {
    /// List order is the order names take them. Teal, the one with no matching circle emoji, comes
    /// late, so most people's accounts get an emoji that matches their colour.
    case purple, blue, green, pink, teal, grey

    public var dark: UInt32 {
        switch self {
        case .purple: 0xAF87FF
        case .teal: 0x00CCCC
        case .blue: 0x5AA9FF
        case .green: 0x5FD38A
        case .pink: 0xFF7FBF
        case .grey: 0xA3ACB8
        }
    }

    public var light: UInt32 {
        switch self {
        case .purple: 0x7540B8
        case .teal: 0x007F7F
        case .blue: 0x1F6FD0
        case .green: 0x1E8A4C
        case .pink: 0xC2337F
        case .grey: 0x5C6570
        }
    }

    /// The pair as one colour that follows the view's appearance.
    public var color: Color {
        let dark = dark, light = light
        #if canImport(UIKit)
        return Color(uiColor: UIColor { $0.userInterfaceStyle == .dark ? Self.ui(dark) : Self.ui(light) })
        #else
        return Color(nsColor: NSColor(name: nil) { appearance in
            appearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua ? Self.ui(dark) : Self.ui(light)
        })
        #endif
    }

    #if canImport(UIKit)
    private static func ui(_ rgb: UInt32) -> UIColor { UIColor(red: part(rgb, 16), green: part(rgb, 8), blue: part(rgb, 0), alpha: 1) }
    #else
    private static func ui(_ rgb: UInt32) -> NSColor { NSColor(srgbRed: part(rgb, 16), green: part(rgb, 8), blue: part(rgb, 0), alpha: 1) }
    #endif

    private static func part(_ rgb: UInt32, _ shift: UInt32) -> CGFloat { CGFloat((rgb >> shift) & 0xff) / 255 }

    /// A ring's glyph: full strength when selected, 55% when not (Viz, 2026-10-31).
    public static func opacity(selected: Bool) -> Double { selected ? 1 : 0.55 }

    /// Each name in list order takes the next unused swatch. Past the last swatch the list starts
    /// again: a repeated colour is better than a ring with none.
    public static func assign(_ names: [String]) -> [String: Swatch] {
        var out: [String: Swatch] = [:]
        for name in names where out[name] == nil {
            out[name] = allCases[out.count % allCases.count]
        }
        return out
    }
}
