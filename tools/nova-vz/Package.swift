// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "nova-vz",
    platforms: [.macOS(.v14)],
    targets: [
        .executableTarget(name: "nova-vz", path: "Sources")
    ]
)
