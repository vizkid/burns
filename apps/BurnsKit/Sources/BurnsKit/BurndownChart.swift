import Charts
import SwiftUI

/// The burndown (docs/SPEC.md, "Showing usage"): quota left as an area in the pace colour, edged by
/// a solid line. The steady burn and the projection share one dash and are grey (Viz, 2026-10-29);
/// the steady line is thin and faint, the projection as heavy as the actual line, so weight alone
/// still tells them apart when StandBy tints the chart red at night.
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
        var curve: InterpolationMethod = .linear
    }

    /// The actual line as drawn: one monotone curve, which never turns back or overshoots a point.
    var actual: [Point] { Self.plotted(burndown.line?.actual ?? [], minGap: 0.01) }

    /// Readings closer than `minGap` of the window merge into the later one: a near-vertical step
    /// makes a monotone curve overshoot far past the chart. The span before the first reading, where
    /// nothing was read, gets points along its straight line, so the curve does not bow there.
    static func plotted(_ actual: [Point], minGap: Double) -> [Point] {
        guard actual.count > 2, let start = actual.first else { return actual }
        var read: [Point] = []
        for p in actual.dropFirst() {
            if let last = read.last, p.x - last.x < minGap { read[read.count - 1] = p } else { read.append(p) }
        }
        guard let first = read.first else { return actual }
        let lead = (0..<4).map { i -> Point in
            let f = Double(i) / 4
            return Point(x: start.x + (first.x - start.x) * f, left: start.left + (first.left - start.left) * f)
        }
        return lead + read
    }

    var width: CGFloat { compact ? 1.5 : 2 }
    /// Round caps grow each dash by the line width, so the gap is wider than it looks here.
    let dash: [CGFloat] = [3, 5]
    var tone: Color { palette.usage(burndown.line?.verdict) }

    var segments: [Segment] {
        guard let line = burndown.line else { return [] }
        return [
            Segment(id: "steady", points: line.steady, style: StrokeStyle(lineWidth: 1, lineCap: .round, dash: dash), color: Color.secondary.opacity(0.5)),
            Segment(id: "actual", points: actual, style: StrokeStyle(lineWidth: width, lineCap: .round, lineJoin: .round), color: tone, curve: .monotone),
            Segment(id: "projection", points: line.projection, style: StrokeStyle(lineWidth: width, lineCap: .round, dash: dash), color: .secondary),
        ]
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
            ForEach(Array(actual.enumerated()), id: \.offset) { _, p in
                AreaMark(x: .value("x", p.x), y: .value("left", p.left))
                    .foregroundStyle(LinearGradient(colors: [tone.opacity(0.45), tone.opacity(0.1)], startPoint: .top, endPoint: .bottom))
                    .interpolationMethod(.monotone)
            }
            ForEach(segments) { segment in
                ForEach(Array(segment.points.enumerated()), id: \.offset) { _, p in
                    LineMark(x: .value("x", p.x), y: .value("left", p.left), series: .value("line", segment.id))
                        .foregroundStyle(segment.color)
                        .lineStyle(segment.style)
                        // Monotone through readings: a smooth curve that never turns back or
                        // overshoots one, so it adds no usage that was not read.
                        .interpolationMethod(segment.curve)
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
                    if b.line == nil {
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
