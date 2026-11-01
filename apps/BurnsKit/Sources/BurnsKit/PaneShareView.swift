import SwiftUI

/// The selected account's live panes under the burndown: a segmented bar over one line per pane.
/// The legend is always there, so a pane is never named by shade alone. Hovering either a segment
/// or a line marks both: the smallest segments are hard to match to a line by shade. A phone has
/// no hover, so a tap marks them and shows what the subagents used.
public struct PaneShareView: View {
    let shares: PaneShares?
    let account: AccountID?
    let tint: Color
    @State private var hovered: String?

    public init(shares: PaneShares?, account: AccountID?, tint: Color) {
        self.shares = shares; self.account = account; self.tint = tint
    }

    private static let gap: CGFloat = 2

    func color(rank: Int) -> Color {
        PaneShares.strength(rank: rank).map { tint.opacity($0) } ?? Color.secondary.opacity(0.35)
    }

    /// The marked pane, by id: a rank would land on another pane after a ring change or a new read.
    private func marked(_ panes: [PaneShare]) -> String? {
        panes.contains { $0.id == hovered } ? hovered : nil
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Live panes\(account.map { ": share of \($0.label)" } ?? "")").font(.subheadline.weight(.semibold))
            if let shares {
                if !shares.panes.isEmpty {
                    bar(shares.panes)
                    // No spacing: rows touch, so the hover never drops between them.
                    VStack(alignment: .leading, spacing: 0) {
                        ForEach(Array(shares.panes.enumerated()), id: \.element.id) { rank, pane in
                            row(pane, rank: rank, marked: marked(shares.panes) == pane.id)
                        }
                    }
                } else if shares.unread.isEmpty {
                    Text("No live panes on this account.").font(.footnote).foregroundStyle(.secondary)
                }
                if !shares.unread.isEmpty {
                    Text("Not read: \(shares.unread.joined(separator: ", ")). Shares leave those hosts out.")
                        .font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                }
            } else {
                Text("Reading panes").font(.footnote).foregroundStyle(.secondary)
            }
        }
    }

    private func bar(_ panes: [PaneShare]) -> some View {
        GeometryReader { geo in
            let room = Double(geo.size.width - Self.gap * CGFloat(panes.count - 1))
            let widths = PaneShares.widths(panes.map(\.share), room: room, minimum: 3)
            let mark = marked(panes)
            // The gap is padding inside each segment's hover area, so the hover never drops crossing it.
            HStack(spacing: 0) {
                ForEach(Array(panes.enumerated()), id: \.element.id) { rank, pane in
                    RoundedRectangle(cornerRadius: 3).fill(mark == pane.id ? tint : color(rank: rank))
                        .opacity(mark == nil || mark == pane.id ? 1 : 0.4) // Full colour, or a pale rank would not stand out.
                        .frame(width: widths[rank])
                        .padding(.trailing, rank == panes.count - 1 ? 0 : Self.gap)
                        .contentShape(Rectangle())
                        .modifier(PaneMark(id: pane.id, hovered: $hovered))
                }
            }
        }
        .frame(height: 16)
        .accessibilityHidden(true) // The rows below say the same in words.
    }

    private func row(_ pane: PaneShare, rank: Int, marked: Bool) -> some View {
        VStack(alignment: .leading, spacing: 1) {
            HStack(spacing: 6) {
                RoundedRectangle(cornerRadius: 2).fill(color(rank: rank)).frame(width: 8, height: 8)
                Image(systemName: pane.icon).font(.caption).foregroundStyle(.secondary).frame(width: 18).help(pane.host)
                Text(pane.title).font(.callout).lineLimit(1).truncationMode(.middle)
                Spacer(minLength: 6)
                Text(pane.modelLine).font(.caption).foregroundStyle(.secondary).lineLimit(1).layoutPriority(1)
                Text(PaneShares.percent(pane.share)).font(.callout).monospacedDigit().frame(width: 34, alignment: .trailing)
            }
            #if os(iOS)
            if marked, let used = pane.subagentModels {
                Text(used).font(.caption2).foregroundStyle(.secondary).padding(.leading, 38)
            }
            #endif
        }
        .padding(.horizontal, 4)
        .padding(.vertical, 2.5)
        .background(Color.primary.opacity(marked ? 0.1 : 0), in: RoundedRectangle(cornerRadius: 4))
        .padding(.horizontal, -4)
        .contentShape(Rectangle())
        .modifier(PaneMark(id: pane.id, hovered: $hovered))
        .help(pane.subagentModels ?? "")
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(pane.title) on \(pane.host), \(pane.modelLine), \(PaneShares.percent(pane.share))")
        .accessibilityHint(pane.subagentModels ?? "")
    }
}

/// Hover marks a pane on the Mac, and a tap toggles it on a phone. Tapping is left out on the Mac,
/// where a click would clear the mark under a pointer that is still inside.
private struct PaneMark: ViewModifier {
    let id: String
    @Binding var hovered: String?

    func body(content: Content) -> some View {
        #if os(iOS)
        content.onTapGesture { hovered = hovered == id ? nil : id }
        #else
        content.onHover { inside in
            if inside { hovered = id } else if hovered == id { hovered = nil }
        }
        #endif
    }
}
