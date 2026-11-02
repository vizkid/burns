#if os(macOS)
import AppKit
import Foundation
import SwiftUI
import Testing
@testable import BurnsKit

/// Renders the views from saved smithersd answers, to look at them without a device. Runs only
/// with BURNS_RENDER_DIR set to a folder of `<host>-usage.json` and `<host>-usage-readings.json`;
/// the PNGs land in the same folder.
@MainActor
struct RenderTests {
    /// The money panel, from a saved `GET /usage/cost` answer in `cost.json`.
    @Test(.enabled(if: ProcessInfo.processInfo.environment["BURNS_RENDER_DIR"] != nil))
    func renderCost() throws {
        let dir = URL(fileURLWithPath: ProcessInfo.processInfo.environment["BURNS_RENDER_DIR"]!)
        let answer = try Wire.decoder.decode(CostAnswer.self, from: Data(contentsOf: dir.appendingPathComponent("cost.json")))
        let start = Date(timeIntervalSinceNow: -7 * 86_400), limit = Date(timeIntervalSinceNow: -6 * 3600)
        for (name, account) in [("claude-viz", AccountID(provider: "claude", name: "viz")), ("codex-viz", AccountID(provider: "codex", name: "viz"))] {
            var s = CostSummary.build([HostCost(host: "a", answer: answer), HostCost(host: "Other", answer: nil)], account: account, windowStart: start, limitAt: limit)
            if name == "claude-viz" { s.windowBilledMinor = 3_850 } // Spend, to see the large number with money in it.
            for scheme in [ColorScheme.dark, .light] {
                try save(AnyView(CostView(summary: s, period: "This window").frame(width: 352).padding(14).background(scheme == .dark ? Color.black : Color.white)),
                         scheme: scheme, size: CGSize(width: 380, height: 290), to: dir.appendingPathComponent("cost-\(name)-\(scheme == .dark ? "dark" : "light").png"))
            }
        }
    }

    @Test(.enabled(if: ProcessInfo.processInfo.environment["BURNS_RENDER_DIR"] != nil))
    func renderSavedAnswers() throws {
        let dir = URL(fileURLWithPath: ProcessInfo.processInfo.environment["BURNS_RENDER_DIR"]!)
        let names = try FileManager.default.contentsOfDirectory(atPath: dir.path)
            .filter { $0.hasSuffix("-usage.json") }.sorted()
        let snaps = try names.map { name -> HostSnapshot in
            let host = String(name.dropLast("-usage.json".count))
            let usage = try Data(contentsOf: dir.appendingPathComponent(name))
            let readings = try Data(contentsOf: dir.appendingPathComponent("\(host)-usage-readings.json"))
            return HostSnapshot(address: host, fetchedAt: Date(), usage: try Wire.decoder.decode(UsageAnswer.self, from: usage),
                                readings: try Wire.decoder.decode(ReadingsAnswer.self, from: readings).readings)
        }
        let first = UsageModel(snapshots: snaps, selected: nil)
        let choices: [AccountID?] = [nil] + first.rings.map(\.id)
        for choice in choices {
            let model = UsageModel(snapshots: snaps, selected: choice)
            let tag = choice.map { "\($0.provider)-\($0.name)" } ?? "default"
            for scheme in [ColorScheme.light, .dark] {
                let suffix = scheme == .dark ? "-dark" : ""
                try save(window(model), scheme: scheme, size: CGSize(width: 380, height: 260), to: dir.appendingPathComponent("window-\(tag)\(suffix).png"))
                try save(widget(model, diameter: 26, names: false), scheme: scheme, size: CGSize(width: 170, height: 170), to: dir.appendingPathComponent("small-\(tag)\(suffix).png"))
                try save(widget(model, diameter: 34, names: true), scheme: scheme, size: CGSize(width: 364, height: 170), to: dir.appendingPathComponent("medium-\(tag)\(suffix).png"))
            }
        }
    }

    func window(_ model: UsageModel) -> some View {
        UsageView(model: model, diameter: 40) { ring, selected in RingCell(ring: ring, selected: selected, diameter: 40, showsName: true) }
            .padding(14)
    }

    func widget(_ model: UsageModel, diameter: CGFloat, names: Bool) -> some View {
        UsageView(model: model, diameter: diameter, compact: true) { ring, selected in
            RingCell(ring: ring, selected: selected, diameter: diameter, showsName: names)
        }
        .padding(12)
        .background(.fill.tertiary, in: .rect(cornerRadius: 22))
    }

    func save(_ view: some View, scheme: ColorScheme, size: CGSize, to url: URL) throws {
        let back = scheme == .dark ? Color(white: 0.12) : Color(white: 0.97)
        let r = ImageRenderer(content: view.frame(width: size.width, height: size.height).background(back).environment(\.colorScheme, scheme))
        r.scale = 2
        guard let image = r.cgImage else { throw CocoaError(.fileWriteUnknown) }
        let rep = NSBitmapImageRep(cgImage: image)
        try rep.representation(using: .png, properties: [:])!.write(to: url)
    }
}
#endif
