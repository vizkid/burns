import SwiftUI

/// The colours the rings and the burndown use. Ported from Penfold's spend palette: used in the
/// paid yellow, over pace amber, out red, the time arc a blue-grey.
public struct BurnsPalette: Sendable {
    public var used: Color
    public var spare: Color
    public var over: Color
    public var critical: Color
    public var track: Color

    /// Amber and red are fixed sRGB values, not `.orange` and `.red`: the Mac's menu bar window
    /// blends system colours with what is behind it, which turned amber pale yellow and red salmon
    /// beside the fixed yellow (Viz, 2026-10-29).
    public static func standard(increasedContrast: Bool = false) -> BurnsPalette {
        BurnsPalette(used: Color(red: 0.96, green: 0.74, blue: 0.20),
                     spare: increasedContrast ? .secondary : Color(red: 0.616, green: 0.698, blue: 0.769).opacity(0.42),
                     over: Color(red: 1.0, green: 0.55, blue: 0.10), critical: Color(red: 0.93, green: 0.23, blue: 0.20),
                     track: Color.secondary.opacity(increasedContrast ? 0.45 : 0.25))
    }

    /// The usage arc's and the chart line's colour: yellow on or under pace, amber over it, red when
    /// it runs out early. The pace colour goes on usage, as on Penfold's Mac bars, not on the time
    /// arc: a time arc in amber over a yellow usage arc of nearly the same length hid the yellow and
    /// read as a second shade of it (Viz, 2026-10-29).
    public func usage(_ verdict: SpendPace.Verdict?) -> Color {
        switch verdict {
        case .critical?: return critical
        case .over?: return over
        case .onPace?, nil: return used
        }
    }
}

/// A provider's one-colour mark in the middle of a ring, tinted by the caller. Penfold's vectors:
/// Claude's spark from Anthropic's press kit, and Codex's cloud from the MIT-licensed LobeHub
/// drawing (@lobehub/icons, codex-color). Used to name the provider, as Rex and Penfold do.
public struct ProviderSymbol: View {
    let provider: String
    public init(provider: String) { self.provider = provider }

    /// Codex's solid cloud fills its image; Claude's thin spark does not. At one size the cloud
    /// looks much larger, so it draws at Penfold's 0.68.
    var scale: CGFloat { provider == "codex" ? 0.68 : 1 }

    public var body: some View {
        GeometryReader { geo in
            Image(provider == "codex" ? "CodexGlyph" : "ClaudeGlyph", bundle: .module)
                .renderingMode(.template).resizable().interpolation(.high).scaledToFit()
                .frame(width: geo.size.width * scale, height: geo.size.height * scale)
                .frame(width: geo.size.width, height: geo.size.height)
        }
    }
}

/// Two rings with a mark in the middle (ported from Penfold's `SpendRings`): outer five-hour,
/// inner seven-day, each with a usage arc in the pace colour and a neutral time arc. Usage is
/// always on top: under pace the time arc shows past it, and over pace the usage colour already
/// says so. The translucent time arc drawn over usage would tint it. An unknown window draws only
/// its empty track.
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
            mark.frame(width: max(8, inner - 2 * stroke - 2), height: max(8, inner - 2 * stroke - 2))
        }
        .frame(width: diameter, height: diameter)
        .accessibilityHidden(true)
    }

    private func ring(_ w: SpendTileModel.Window, diameter: CGFloat) -> some View {
        ZStack {
            Circle().stroke(palette.track, lineWidth: stroke)
            arc(w.elapsed ?? 0, palette.spare, diameter)
            arc(w.fill, palette.usage(w.verdict), diameter)
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
            if ring.credits {
                // No windows, so no track either: an empty track would read as unknown quota.
                Image("AzureMark", bundle: .module).renderingMode(.original).resizable().interpolation(.high).scaledToFit()
                    .padding(diameter * 0.18)
                    .frame(width: diameter, height: diameter)
                    .opacity(selected ? 1 : 0.75)
            } else {
                SpendRings(model: ring.tile, palette: palette, diameter: diameter) {
                    ProviderSymbol(provider: ring.id.provider).foregroundStyle(selected ? Color.primary : Color.secondary)
                }
            }
            if showsName {
                Text(ring.id.name).font(.caption2).lineLimit(1).foregroundStyle(selected ? .primary : .secondary)
            }
            Capsule().fill(selected ? Color.primary : Color.clear).frame(width: diameter * 0.4, height: 2)
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(ring.credits ? "\(ring.tile.account): Microsoft credits" : ring.tile.accessibilityLabel)
        .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
    }
}
