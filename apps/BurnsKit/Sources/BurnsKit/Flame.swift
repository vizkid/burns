#if os(macOS)
import AppKit

/// Burns' flame (docs/SPEC.md, "Icons"): a flat outline traced from the 3D flame in the app icon,
/// `apps/Shared/AppIcon.icon`. Not the SF Symbol `flame.fill`: Apple's terms keep SF Symbols out
/// of app icons, and the menu bar should show the same mark as the icon.
public enum BurnsFlame {
    /// The menu bar's height for the flame, in points. SF Symbols at the menu bar's default size
    /// stand about this tall.
    public static let menuBarHeight: CGFloat = 16

    /// The flame for the menu bar. Without a tint it is a template image, so macOS draws it in the
    /// menu bar's own colour. With a tint, such as amber over pace, it keeps that colour.
    public static func menuBarImage(tint: NSColor? = nil) -> NSImage? {
        guard let source = Bundle.module.image(forResource: "Flame") else { return nil }
        let size = NSSize(width: menuBarHeight * source.size.width / source.size.height, height: menuBarHeight)
        let image = NSImage(size: size, flipped: false) { rect in
            source.draw(in: rect)
            if let tint {
                tint.set()
                rect.fill(using: .sourceAtop)
            }
            return true
        }
        image.isTemplate = tint == nil
        image.accessibilityDescription = "Burns"
        return image
    }
}
#endif
