import SwiftUI

/// The icons an account may take: an SF Symbol for the apps, paired with a glyph for Rex, whose
/// labels and titles are text and draw a symbol as an empty box (docs/SPEC.md, "Account icons in
/// the apps"). Each glyph is one character with no emoji form.
public enum SymbolCatalogue {
    public struct Item: Identifiable, Equatable, Sendable {
        public let symbol: String
        public let glyph: String
        public let name: String
        public var id: String { symbol }
    }

    /// What an account with no icon shows.
    public static let standard = Item(symbol: "circle.fill", glyph: "●", name: "circle")

    public static let all: [Item] = [
        standard,
        Item(symbol: "circle", glyph: "○", name: "ring"),
        Item(symbol: "circle.lefthalf.filled", glyph: "◐", name: "left half circle"),
        Item(symbol: "circle.righthalf.filled", glyph: "◑", name: "right half circle"),
        Item(symbol: "circle.tophalf.filled", glyph: "◓", name: "top half circle"),
        Item(symbol: "circle.bottomhalf.filled", glyph: "◒", name: "bottom half circle"),
        Item(symbol: "smallcircle.filled.circle", glyph: "◉", name: "target"),
        Item(symbol: "square.fill", glyph: "■", name: "square"),
        Item(symbol: "square", glyph: "□", name: "open square"),
        Item(symbol: "square.lefthalf.filled", glyph: "◧", name: "left half square"),
        Item(symbol: "square.righthalf.filled", glyph: "◨", name: "right half square"),
        Item(symbol: "diamond.fill", glyph: "◆", name: "diamond"),
        Item(symbol: "diamond", glyph: "◇", name: "open diamond"),
        Item(symbol: "triangle.fill", glyph: "▲", name: "triangle"),
        Item(symbol: "triangle", glyph: "△", name: "open triangle"),
        Item(symbol: "arrowtriangle.down.fill", glyph: "▼", name: "down triangle"),
        Item(symbol: "star.fill", glyph: "★", name: "star"),
        Item(symbol: "star", glyph: "☆", name: "open star"),
        Item(symbol: "hexagon.fill", glyph: "⬢", name: "hexagon"),
        Item(symbol: "hexagon", glyph: "⬡", name: "open hexagon"),
        Item(symbol: "pentagon.fill", glyph: "⬟", name: "pentagon"),
        Item(symbol: "sparkle", glyph: "✦", name: "sparkle"),
        Item(symbol: "asterisk", glyph: "✱", name: "asterisk"),
        Item(symbol: "plus", glyph: "✚", name: "plus"),
        Item(symbol: "xmark", glyph: "✕", name: "cross"),
        Item(symbol: "checkmark", glyph: "✓", name: "check"),
    ]

    /// Sets both halves of an icon, or clears both for the default.
    public static func apply(_ item: Item?, to account: inout CentralAccount) {
        account.symbol = item?.symbol
        account.emoji = item?.glyph
    }
}

/// An account's icon as the apps draw it: its symbol in its swatch colour, or an older account's
/// emoji as text.
public struct AccountIcon: View {
    let group: AccountGroup
    public init(_ group: AccountGroup) { self.group = group }

    public var body: some View {
        if let symbol = group.symbol {
            Image(systemName: symbol).foregroundStyle(group.swatch.color)
        } else {
            Text(group.emoji)
        }
    }
}

/// A button showing the account's icon, which opens a grid of the catalogue.
struct SymbolPicker: View {
    let group: AccountGroup
    let pick: (SymbolCatalogue.Item?) -> Void
    @State private var showing = false

    var body: some View {
        Button { showing = true } label: { AccountIcon(group).font(.title3) }
            .accessibilityLabel("Icon \(group.symbol ?? group.emoji)")
            .popover(isPresented: $showing) {
                SymbolGrid(swatch: group.swatch) { picked in
                    pick(picked)
                    showing = false
                }
            }
    }
}

/// The grid in the picker. Nil means "use the default".
struct SymbolGrid: View {
    let swatch: Swatch
    let pick: (SymbolCatalogue.Item?) -> Void

    var body: some View {
        VStack(spacing: 8) {
            ScrollView {
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 40))], spacing: 6) {
                    ForEach(SymbolCatalogue.all) { item in
                        Button { pick(item) } label: {
                            Image(systemName: item.symbol).font(.title2).foregroundStyle(swatch.color)
                                .frame(minWidth: 40, minHeight: 40)
                                .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                        .help(item.name)
                        .accessibilityLabel(item.name)
                    }
                }
            }
            Button("Use Default") { pick(nil) }
        }
        .padding(12)
        .frame(minWidth: 280, idealWidth: 320, minHeight: 240, idealHeight: 300)
    }
}
