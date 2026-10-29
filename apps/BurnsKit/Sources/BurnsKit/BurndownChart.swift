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
    let dash: [CGFloat] = [2, 5]
    var tone: Color { palette.usage(burndown.line?.verdict) }

    var projectionArea: [Point] {
        guard let p = burndown.line?.projection, let first = p.first else { return [] }
        return [Point(x: max(0, first.x - 0.005), left: first.left)] + p
    }

    var segments: [Segment] {
        [Segment(id: "actual", points: actual, style: StrokeStyle(lineWidth: width, lineCap: .round, lineJoin: .round), color: tone, curve: .monotone)]
    }

    /// Where a dashed line starting at `start` must begin its pattern so its dashes fall on those of
    /// a line with the same pattern starting at `origin`. On pace the projection lies on the steady
    /// line, and two patterns out of step there read as one jagged, beaded line.
    static func dashPhase(origin: CGPoint, start: CGPoint, period: CGFloat) -> CGFloat {
        guard period > 0 else { return 0 }
        return hypot(start.x - origin.x, start.y - origin.y).truncatingRemainder(dividingBy: period)
    }

    /// The steady burn and the projection, drawn over the plot in screen points: Swift Charts gives
    /// no dash phase, and the two patterns must line up where the lines meet.
    func dashedLines(_ proxy: ChartProxy, _ geo: GeometryProxy) -> some View {
        let plot = proxy.plotFrame.map { geo[$0] } ?? .zero
        func screen(_ p: Point) -> CGPoint? {
            guard let x = proxy.position(forX: p.x), let y = proxy.position(forY: p.left) else { return nil }
            return CGPoint(x: plot.minX + x, y: plot.minY + y)
        }
        func path(_ points: [Point]) -> Path {
            Path { path in path.addLines(points.compactMap(screen)) }
        }
        let steady = burndown.line?.steady ?? [], projection = burndown.line?.projection ?? []
        let origin = steady.first.flatMap(screen) ?? .zero
        let phase = projection.first.flatMap(screen).map { Self.dashPhase(origin: origin, start: $0, period: dash.reduce(0, +)) } ?? 0
        return ZStack {
            path(steady).stroke(Color.secondary.opacity(0.5), style: StrokeStyle(lineWidth: 1, lineCap: .round, dash: dash))
            path(projection).stroke(Color.secondary, style: StrokeStyle(lineWidth: width, lineCap: .round, dash: dash, dashPhase: phase))
        }
        .allowsHitTesting(false)
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
            // The quota the projection says would be left, faintly: the future part of the area. Drawn
            // first and started a little early, under the actual area, so the two leave no seam.
            ForEach(Array(projectionArea.enumerated()), id: \.offset) { _, p in
                AreaMark(x: .value("x", p.x), y: .value("left", p.left), series: .value("area", "projection"))
                    .foregroundStyle(Color.secondary.opacity(0.12))
            }
            ForEach(Array(actual.enumerated()), id: \.offset) { _, p in
                AreaMark(x: .value("x", p.x), y: .value("left", p.left), series: .value("area", "actual"))
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
        .chartOverlay { proxy in GeometryReader { geo in dashedLines(proxy, geo) } }
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
