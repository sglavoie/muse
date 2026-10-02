import AppKit
import XCTest
@testable import muse

final class AppSkeletonTests: XCTestCase {
    func testHostBundleIdentity() {
        XCTAssertEqual(Bundle.main.bundleIdentifier, "io.github.sglavoie.muse")
    }

    func testAppIconShipsInTheBundle() throws {
        let info = Bundle.main.infoDictionary ?? [:]
        XCTAssertEqual(info["CFBundleIconName"] as? String, "AppIcon")
        let file = try XCTUnwrap(info["CFBundleIconFile"] as? String)
        XCTAssertNotNil(Bundle.main.url(forResource: file, withExtension: file.hasSuffix(".icns") ? nil : "icns"))
    }

    func testStatusIconIsTheTemplateAsset() throws {
        let asset = try XCTUnwrap(NSImage(named: "StatusIcon"))
        XCTAssertEqual(asset.size, NSSize(width: 18, height: 18))
        let icon = StatusItemController.icon()
        XCTAssertTrue(icon.isTemplate)
        XCTAssertEqual(icon.size, NSSize(width: 18, height: 18))
        XCTAssertEqual(icon.tiffRepresentation, asset.tiffRepresentation, "the SF Symbol fallback was used")
    }

    func testMusicLibraryFoldersHaveUsageDescriptions() {
        let info = Bundle.main.infoDictionary ?? [:]
        XCTAssertEqual(info["LSApplicationCategoryType"] as? String, "public.app-category.music")
        for key in [
            "NSDocumentsFolderUsageDescription", "NSDesktopFolderUsageDescription",
            "NSDownloadsFolderUsageDescription", "NSRemovableVolumesUsageDescription",
            "NSNetworkVolumesUsageDescription",
        ] {
            XCTAssertFalse((info[key] as? String ?? "").isEmpty, "\(key) is missing")
        }
    }
}
