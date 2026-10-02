import AppKit
import GhosttyKit
import XCTest
@testable import muse

/// Drives a real surface in an offscreen window. The hosting app has run
/// `ghostty_init` but, under XCTest, opens no terminal of its own; the
/// surface here runs `/bin/cat`, which waits on its pty until freed.
final class SurfaceViewTests: XCTestCase {
    private var ghostty: Ghostty.App!
    private var window: NSWindow!
    private var view: Ghostty.SurfaceView!

    override func setUpWithError() throws {
        let overrides = Ghostty.ConfigOverrides(command: URL(fileURLWithPath: "/bin/cat"))
        ghostty = Ghostty.App { Ghostty.Config(userConfig: nil, overrides: overrides) }
        let app = try XCTUnwrap(ghostty.app)
        window = NSWindow(
            contentRect: NSRect(x: -10_000, y: -10_000, width: 800, height: 500),
            styleMask: [.titled, .resizable],
            backing: .buffered,
            defer: false
        )
        window.isReleasedWhenClosed = false
        view = Ghostty.SurfaceView(app: app)
        window.contentView = view
    }

    override func tearDownWithError() throws {
        window.contentView = nil
        view = nil
        window.close()
        window = nil
        ghostty = nil
    }

    private func surfaceSize() throws -> ghostty_surface_size_s {
        ghostty_surface_size(try XCTUnwrap(view.surface))
    }

    func testTheSurfaceAcceptsDroppedFilesURLsAndText() throws {
        XCTAssertTrue(Set(view.registeredDraggedTypes).isSuperset(of: Ghostty.SurfaceView.dropTypes))

        let pasteboard = NSPasteboard(name: .init("muse-drop-\(UUID().uuidString)"))
        defer { pasteboard.releaseGlobally() }
        pasteboard.clearContents()
        pasteboard.writeObjects([
            URL(fileURLWithPath: "/tmp/Screen Shot (1).png") as NSURL,
            URL(fileURLWithPath: "/tmp/b.txt") as NSURL,
        ])
        XCTAssertEqual(Ghostty.SurfaceView.dropText(from: pasteboard), "/tmp/Screen\\ Shot\\ \\(1\\).png /tmp/b.txt")

        pasteboard.clearContents()
        pasteboard.setString("https://example.com/x", forType: .string)
        XCTAssertEqual(Ghostty.SurfaceView.dropText(from: pasteboard), "https://example.com/x")

        pasteboard.clearContents()
        XCTAssertNil(Ghostty.SurfaceView.dropText(from: pasteboard))
    }

    /// A new default zoom reaches a zoomed surface only through the reset
    /// that follows the reload, which is how Preferences applies it.
    func testReloadedFontSizeAppliesToAZoomedSurfaceOnReset() throws {
        // The setUp surface goes before the app it belongs to.
        window.contentView = nil
        view = nil
        var fontSize: Float? = nil
        let overrides = Ghostty.ConfigOverrides(command: URL(fileURLWithPath: "/bin/cat"))
        ghostty = Ghostty.App {
            var current = overrides
            current.fontSize = fontSize
            return Ghostty.Config(userConfig: nil, overrides: current)
        }
        view = Ghostty.SurfaceView(app: try XCTUnwrap(ghostty.app))
        window.contentView = view
        let initial = try surfaceSize().cell_width_px

        XCTAssertTrue(view.performBindingAction("increase_font_size:20"))
        let zoomed = try surfaceSize().cell_width_px
        XCTAssertGreaterThan(zoomed, initial)

        fontSize = 20
        XCTAssertTrue(ghostty.reloadConfig())
        XCTAssertEqual(ghostty.config.get("font-size", as: Float.self, initial: 0), 20)
        XCTAssertEqual(try surfaceSize().cell_width_px, zoomed, "a reload keeps a manual zoom")

        XCTAssertTrue(view.performBindingAction("reset_font_size"))
        let reset = try surfaceSize().cell_width_px
        XCTAssertGreaterThan(reset, initial)
        XCTAssertLessThan(reset, zoomed)
    }

    func testSurfaceIsCreatedAndHostsItself() throws {
        let surface = try XCTUnwrap(view.surface)
        let host = Ghostty.surfaceHost(from: ghostty_surface_userdata(surface))
        XCTAssertTrue(host === view)
    }

    func testFramebufferTracksTheViewInBackingPixels() throws {
        let backing = view.convertToBacking(view.bounds.size)
        let size = try surfaceSize()
        XCTAssertEqual(size.width_px, UInt32(backing.width))
        XCTAssertEqual(size.height_px, UInt32(backing.height))
        XCTAssertGreaterThan(size.columns, 0)
        XCTAssertGreaterThan(size.rows, 0)
    }

    func testResizeChangesTheGridBothWays() throws {
        let initial = try surfaceSize()

        window.setContentSize(NSSize(width: 400, height: 250))
        let smaller = try surfaceSize()
        XCTAssertLessThan(smaller.columns, initial.columns)
        XCTAssertLessThan(smaller.rows, initial.rows)
        XCTAssertEqual(smaller.width_px, UInt32(view.convertToBacking(view.bounds.size).width))

        window.setContentSize(NSSize(width: 800, height: 500))
        let restored = try surfaceSize()
        XCTAssertEqual(restored.columns, initial.columns)
        XCTAssertEqual(restored.rows, initial.rows)
    }

    func testBackingChangeKeepsTheSameFramebuffer() throws {
        let before = try surfaceSize()
        view.viewDidChangeBackingProperties()
        let after = try surfaceSize()
        XCTAssertEqual(after.width_px, before.width_px)
        XCTAssertEqual(after.height_px, before.height_px)
        XCTAssertEqual(view.layer?.contentsScale, window.backingScaleFactor)
    }

    func testRendererInstallsItsOwnLayer() {
        XCTAssertTrue(view.wantsLayer)
        XCTAssertNotNil(view.layer)
    }

    func testFocusFollowsFirstResponderInTheKeyWindow() {
        // An offscreen window never becomes key, so the surface stays unfocused
        // even as first responder; losing first responder keeps it so.
        XCTAssertTrue(window.makeFirstResponder(view))
        XCTAssertEqual(view.focused, window.isKeyWindow)
        view.focusDidChange(true)
        XCTAssertTrue(view.focused)
        XCTAssertTrue(window.makeFirstResponder(nil))
        XCTAssertFalse(view.focused)
    }

    func testTitleChangesAreReportedOnce() {
        var titles: [String] = []
        view.onTitleChange = { titles.append($0) }
        view.setTitle("muse")
        view.setTitle("muse")
        view.setTitle("library")
        XCTAssertEqual(titles, ["muse", "library"])
        XCTAssertEqual(view.title, "library")
    }

    func testPixelSizeRoundsDownAndNeverReachesZero() {
        XCTAssertTrue(Ghostty.SurfaceView.pixelSize(CGSize(width: 1601.7, height: 999.2)) == (1601, 999))
        XCTAssertTrue(Ghostty.SurfaceView.pixelSize(.zero) == (1, 1))
        XCTAssertTrue(Ghostty.SurfaceView.pixelSize(CGSize(width: -4, height: 0.5)) == (1, 1))
    }

    func testColorSchemeFollowsTheAppearance() throws {
        let dark = try XCTUnwrap(NSAppearance(named: .darkAqua))
        let light = try XCTUnwrap(NSAppearance(named: .aqua))
        XCTAssertEqual(Ghostty.colorScheme(for: dark), GHOSTTY_COLOR_SCHEME_DARK)
        XCTAssertEqual(Ghostty.colorScheme(for: light), GHOSTTY_COLOR_SCHEME_LIGHT)
    }

    func testDisplayIDComesFromTheScreenNumber() throws {
        let screen = try XCTUnwrap(NSScreen.main)
        XCTAssertEqual(Ghostty.SurfaceView.displayID(of: screen), CGMainDisplayID())
    }
}
