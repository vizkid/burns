// swift-tools-version:5.9
// BurnsKit: the code Burns for Mac, Burns for iOS and their widgets share (docs/SPEC.md,
// "Apps and widgets"): the smithersd wire types, the host merge, pace, Enter's pick, the
// burndown series, the per-host cache, and the views.
import PackageDescription

let package = Package(
    name: "BurnsKit",
    platforms: [.macOS(.v14), .iOS(.v17)],
    products: [.library(name: "BurnsKit", targets: ["BurnsKit"])],
    targets: [
        .target(name: "BurnsKit", path: "Sources/BurnsKit", resources: [.process("Resources")]),
        .testTarget(name: "BurnsKitTests", dependencies: ["BurnsKit"], path: "Tests/BurnsKitTests"),
    ],
    swiftLanguageVersions: [.v5]
)
