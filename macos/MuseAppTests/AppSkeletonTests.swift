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
}
