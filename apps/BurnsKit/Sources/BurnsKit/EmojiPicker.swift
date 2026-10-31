import SwiftUI

/// The emoji a picker offers, read from the Unicode data Swift ships, so there is no list to
/// maintain and search works on Unicode's names. Sequences such as flags and skin tones are left
/// out: an account's emoji is one mark in a Rex tab.
enum EmojiCatalogue {
    struct Item: Identifiable, Equatable, Sendable {
        let emoji: String
        let name: String
        var id: String { emoji }
    }

    /// The circles first: they are the defaults, and they read best at tab size.
    static let circles = ["🔴", "🟠", "🟡", "🟢", "🔵", "🟣", "🟤", "⚫", "⚪"]

    static let all: [Item] = {
        let ranges: [ClosedRange<UInt32>] = [0x1F300...0x1FAFF, 0x2600...0x27BF, 0x2B00...0x2BFF]
        let scalars = ranges.joined().compactMap(Unicode.Scalar.init).filter {
            $0.properties.isEmojiPresentation && !$0.properties.isEmojiModifier
        }
        let items = scalars.map { Item(emoji: String($0), name: ($0.properties.name ?? "").lowercased()) }
        let first = circles.compactMap { c in items.first { $0.emoji == c } }
        return first + items.filter { !circles.contains($0.emoji) }
    }()

    static func matching(_ query: String) -> [Item] {
        let words = query.lowercased().split(separator: " ")
        return words.isEmpty ? all : all.filter { item in words.allSatisfy { item.name.contains($0) } }
    }
}

/// A button showing the account's emoji, which opens a searchable grid. Apple gives no emoji picker
/// view: the Mac's palette needs a focused text field, and iOS ties its emoji to the keyboard.
struct EmojiPicker: View {
    @Binding var emoji: String
    @State private var showing = false

    var body: some View {
        Button { showing = true } label: { Text(emoji).font(.title3) }
            .accessibilityLabel("Emoji \(emoji)")
            .popover(isPresented: $showing) {
                EmojiGrid { picked in
                    emoji = picked
                    showing = false
                }
            }
    }
}

/// The grid in the picker. An empty string means "use the default".
struct EmojiGrid: View {
    let pick: (String) -> Void
    @State private var query = ""

    var body: some View {
        let items = EmojiCatalogue.matching(query)
        VStack(spacing: 8) {
            TextField("Search", text: $query, prompt: Text("Search emoji"))
                .textFieldStyle(.roundedBorder)
                .autocorrectionDisabled()
            if items.isEmpty {
                ContentUnavailableView.search(text: query)
            } else {
                ScrollView {
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 36))], spacing: 4) {
                        ForEach(items) { item in
                            Button(item.emoji) { pick(item.emoji) }
                                .buttonStyle(.plain).font(.title2)
                                .frame(minWidth: 36, minHeight: 36)
                                .contentShape(Rectangle())
                                .accessibilityLabel(item.name)
                        }
                    }
                }
            }
            Button("Use Default") { pick("") }
        }
        .padding(12)
        .frame(minWidth: 300, idealWidth: 320, minHeight: 340, idealHeight: 380)
    }
}
