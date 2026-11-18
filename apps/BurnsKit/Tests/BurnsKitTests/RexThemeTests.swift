import Foundation
import Testing
@testable import BurnsKit

/// docs/SPEC.md, "Pane themes": the Theme picker lists the Rex app's themes.
struct RexThemeTests {
    static let file = """
    [{"id":"zeta-dusk","name":"Zeta Dusk","appearance":"dark","semantic":{
       "terminalBackground":{"red":0.05,"green":0.0863,"blue":0.0941},
       "terminalForeground":{"red":0.7725,"green":0.8471,"blue":0.8471}}},
     {"id":"alpha-milk","name":"Alpha Milk","appearance":"light","semantic":{
       "terminalBackground":{"red":1.2,"green":1,"blue":1},
       "terminalForeground":{"red":0,"green":0,"blue":-0.1}}},
     {"id":"broken","name":"Broken","semantic":{}}]
    """

    @Test func readsThemesByNameWithTheirTerminalColours() throws {
        let themes = try RexTheme.read(Data(Self.file.utf8))
        #expect(themes.map(\.id) == ["alpha-milk", "zeta-dusk"])
        #expect(themes[1].name == "Zeta Dusk")
        #expect(themes[1].background == "#0d1618" && themes[1].foreground == "#c5d8d8")
        #expect(themes[0].background == "#ffffff" && themes[0].foreground == "#000000")
    }

    @Test func aMissingAppMeansNoThemes() {
        #expect(RexTheme.installed(in: [URL(fileURLWithPath: "/nonexistent")]).isEmpty)
    }

    @Test func theThemeIsAKnownField() throws {
        var a = CentralAccount(provider: "claude", name: "alpha", home: ".c", kind: "subscription")
        a.theme = "zeta-dusk"
        let back = try JSONDecoder().decode(CentralAccount.self, from: JSONEncoder().encode(a))
        #expect(back.theme == "zeta-dusk" && back.extra["theme"] == nil)
        #expect(AccountGroup.build([back])[0].theme == "zeta-dusk")
    }
}
