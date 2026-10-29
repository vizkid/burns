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
