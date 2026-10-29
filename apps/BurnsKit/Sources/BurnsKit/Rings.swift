import SwiftUI

/// The colours the rings and the burndown use. Ported from Penfold's spend palette: used in the
/// paid yellow, over pace amber, out red, the unused part a blue-grey.
public struct BurnsPalette: Sendable {
    public var used: Color
    public var spare: Color
    public var over: Color
    public var critical: Color
    public var track: Color

    public static func standard(increasedContrast: Bool = false) -> BurnsPalette {
        BurnsPalette(used: Color(red: 0.96, green: 0.74, blue: 0.20),
                     spare: increasedContrast ? .secondary : Color(red: 0.616, green: 0.698, blue: 0.769).opacity(0.42),
                     over: .orange, critical: .red,
                     track: Color.secondary.opacity(increasedContrast ? 0.45 : 0.25))
    }

    /// A verdict's colour: quiet on or under pace, amber over it, red when it runs out early.
    public func tone(_ verdict: SpendPace.Verdict?) -> Color {
        switch verdict {
        case .critical?: return critical
        case .over?: return over
        case .onPace?, nil: return spare
        }
    }
}

/// A provider's mark in the middle of a ring. SF Symbols, so the public repository carries no
/// vendor logos: a sparkle for Claude, a cloud for Codex.
public struct ProviderSymbol: View {
    let provider: String
    public init(provider: String) { self.provider = provider }
    public var body: some View {
        Image(systemName: provider == "codex" ? "cloud.fill" : "sparkle")
            .resizable().scaledToFit()
    }
}

/// Two rings with a mark in the middle (ported from Penfold's `SpendRings`): outer five-hour,
/// inner seven-day, each with a usage arc and a time arc in the verdict's colour. An unknown
/// window draws only its empty track.
public struct SpendRings<Mark: View>: View {
    let model: SpendTileModel
    let palette: BurnsPalette
    let diameter: CGFloat
    let mark: Mark

    public init(model: SpendTileModel, palette: BurnsPalette, diameter: CGFloat, @ViewBuilder mark: () -> Mark) {
        self.model = model; self.palette = palette; self.diameter = diameter; self.mark = mark()
    }

    private var stroke: CGFloat { max(3, diameter * 0.09) }
    private var inner: CGFloat { diameter - 2 * stroke - 2 }

    public var body: some View {
        ZStack {
            ring(model.fiveHour, diameter: diameter)
            ring(model.sevenDay, diameter: inner)
            mark.frame(width: max(8, inner - 2 * stroke - 6), height: max(8, inner - 2 * stroke - 6))
        }
        .frame(width: diameter, height: diameter)
        .accessibilityHidden(true)
    }

    private func ring(_ w: SpendTileModel.Window, diameter: CGFloat) -> some View {
        let elapsed = w.elapsed ?? 0
        let timeOnTop = elapsed < w.fill
        return ZStack {
            Circle().stroke(palette.track, lineWidth: stroke)
            if timeOnTop {
                arc(w.fill, palette.used, diameter)
                arc(elapsed, palette.tone(w.verdict), diameter)
            } else {
                arc(elapsed, palette.tone(w.verdict), diameter)
                arc(w.fill, palette.used, diameter)
            }
        }
        .frame(width: diameter, height: diameter)
    }

    private func arc(_ fraction: Double, _ color: Color, _ diameter: CGFloat) -> some View {
        Circle().trim(from: 0, to: fraction)
            .stroke(color, style: StrokeStyle(lineWidth: stroke, lineCap: .round))
            .rotationEffect(.degrees(-90))
            .frame(width: diameter, height: diameter)
    }
}

/// One ring in the row: the rings, the provider mark, and a mark under the selected one. The
/// caller wraps it in a button: an action in the apps, an App Intent in the widgets.
public struct RingCell: View {
    let ring: UsageModel.Ring
    let selected: Bool
    let diameter: CGFloat
    let palette: BurnsPalette
    let showsName: Bool

    public init(ring: UsageModel.Ring, selected: Bool, diameter: CGFloat, palette: BurnsPalette = .standard(), showsName: Bool = false) {
        self.ring = ring; self.selected = selected; self.diameter = diameter; self.palette = palette; self.showsName = showsName
    }

    public var body: some View {
        VStack(spacing: 2) {
            SpendRings(model: ring.tile, palette: palette, diameter: diameter) {
                ProviderSymbol(provider: ring.id.provider).foregroundStyle(selected ? Color.primary : Color.secondary)
            }
            if showsName {
                Text(ring.id.name).font(.caption2).lineLimit(1).foregroundStyle(selected ? .primary : .secondary)
            }
            Capsule().fill(selected ? Color.primary : Color.clear).frame(width: diameter * 0.4, height: 2)
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(ring.tile.accessibilityLabel)
        .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
    }
}
