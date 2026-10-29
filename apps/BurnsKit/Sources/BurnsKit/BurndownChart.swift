import Charts
import SwiftUI

/// The combined burndown (docs/SPEC.md, "Showing usage"). Line style alone tells actual (solid),
/// steady (dashed) and projected (dotted) apart, so the chart still reads when StandBy tints it red
/// at night. The seven-day line is heavier than the five-hour line.
public struct BurndownChart: View {
    let burndown: Burndown
    let palette: BurnsPalette
    let compact: Bool

    public init(burndown: Burndown, palette: BurnsPalette = .standard(), compact: Bool = false) {
        self.burndown = burndown; self.palette = palette; self.compact = compact
    }

    struct Segment: Identifiable {
        let id: String
        let points: [Point]
        let style: StrokeStyle
        let color: Color
    }

    var segments: [Segment] {
        burndown.lines.flatMap { line -> [Segment] in
            let width: CGFloat = line.minutes >= 1440 ? (compact ? 2 : 2.5) : (compact ? 1.2 : 1.5)
            let tone = palette.usage(line.verdict)
            // On the shared axis every window's steady burn is the same line: draw it once.
            let steady = line.minutes == burndown.lines.first?.minutes
                ? [Segment(id: "steady", points: line.steady, style: StrokeStyle(lineWidth: max(1, width - 1), dash: [4, 3]), color: .secondary)] : []
            return steady + [
                Segment(id: "\(line.minutes)-actual", points: line.actual, style: StrokeStyle(lineWidth: width, lineCap: .round), color: palette.used),
                Segment(id: "\(line.minutes)-projection", points: line.projection,
                        style: StrokeStyle(lineWidth: width, lineCap: .round, dash: [1, 3]), color: tone),
            ]
        }
    }

    public var body: some View {
        Chart {
            ForEach(Array(burndown.grid.enumerated()), id: \.offset) { _, slot in
                RectangleMark(xStart: .value("x", slot.x0), xEnd: .value("x", slot.x1), yStart: .value("left", 0), yEnd: .value("left", 100))
                    .foregroundStyle(Color.secondary.opacity(slot.elapsed ? 0.12 : 0.04))
                if slot.x0 > 0 {
                    RuleMark(x: .value("x", slot.x0)).foregroundStyle(Color.secondary.opacity(0.25)).lineStyle(StrokeStyle(lineWidth: 0.5))
                }
            }
            ForEach(segments) { segment in
                ForEach(Array(segment.points.enumerated()), id: \.offset) { _, p in
                    LineMark(x: .value("x", p.x), y: .value("left", p.left), series: .value("line", segment.id))
                        .foregroundStyle(segment.color)
                        .lineStyle(segment.style)
                        .interpolationMethod(.linear)
                }
            }
        }
        .chartXScale(domain: 0...1)
        .chartYScale(domain: 0...100)
        .chartXAxis(.hidden)
        .chartYAxis {
            if !compact {
                AxisMarks(position: .leading, values: [0, 50, 100]) { v in
                    AxisGridLine()
                    AxisValueLabel { Text("\(v.as(Int.self) ?? 0)%").font(.caption2) }
                }
            }
        }
        .chartLegend(.hidden)
        .accessibilityLabel(burndown.headline)
    }
}

/// The ring row above the selected account's burndown: the menu bar window, the iPhone screen and
/// the medium widgets. `cell` wraps each ring in the caller's kind of button.
public struct UsageView<Cell: View>: View {
    let model: UsageModel
    let diameter: CGFloat
    let compact: Bool
    let cell: (UsageModel.Ring, Bool) -> Cell

    public init(model: UsageModel, diameter: CGFloat, compact: Bool = false, @ViewBuilder cell: @escaping (UsageModel.Ring, Bool) -> Cell) {
        self.model = model; self.diameter = diameter; self.compact = compact; self.cell = cell
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: compact ? 4 : 8) {
            if model.rings.isEmpty {
                Text("No usage yet. Add a host in settings.").font(.footnote).foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                HStack(spacing: compact ? 2 : 10) {
                    ForEach(model.rings) { ring in cell(ring, ring.id == model.selected) }
                }
                .frame(maxWidth: .infinity)
                if let b = model.burndown {
                    // Compact widgets are too narrow for both on one line: the headline goes under.
                    let layout = compact ? AnyLayout(VStackLayout(alignment: .leading, spacing: 0)) : AnyLayout(HStackLayout(alignment: .firstTextBaseline))
                    layout {
                        Text(b.account.label).font(compact ? .caption2.weight(.semibold) : .subheadline.weight(.semibold))
                        if !compact { Spacer(minLength: 4) }
                        Text(b.headline).font(compact ? .caption2 : .caption).foregroundStyle(.secondary).lineLimit(1).minimumScaleFactor(0.7)
                    }
                    if b.lines.isEmpty {
                        Text(b.headline == "Microsoft credits" ? "Runs on Microsoft credits. No quota windows to draw." : "No live quota window to draw.")
                            .font(compact ? .caption2 : .footnote).foregroundStyle(.secondary)
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                    } else {
                        BurndownChart(burndown: b, compact: compact)
                    }
                }
            }
        }
    }
}
