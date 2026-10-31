import CoreGraphics

/// The iPhone screen's usage view fills the height the host list leaves (docs/SPEC.md, "Apps and
/// widgets"), so the burndown grows on a tall phone instead of leaving empty space below.
public enum ScreenFit {
    /// The usage view's least height: the ring row and a readable burndown. Below it the screen
    /// scrolls instead.
    public static let minimum: CGFloat = 320
    /// The screen's padding above and below its content, and the space between the usage view and
    /// the host list, as `HomeView` lays them out.
    public static let padding: CGFloat = 16
    public static let spacing: CGFloat = 16

    /// `visible` is the scroll view's height inside its safe area and content insets; `below` is the
    /// host list's height.
    public static func usageHeight(visible: CGFloat, below: CGFloat) -> CGFloat {
        max(minimum, visible - below - spacing - 2 * padding)
    }
}
