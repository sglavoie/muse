import AppKit
import GhosttyKit
import XCTest
@testable import muse

final class ExitPolicyTests: XCTestCase {
    func testShellStatusSplitsExitCodesFromSignals() {
        XCTAssertEqual(MuseExitStatus(shellStatus: 0), .exited(0))
        XCTAssertEqual(MuseExitStatus(shellStatus: 2), .exited(2))
        XCTAssertEqual(MuseExitStatus(shellStatus: 128), .exited(128))
        XCTAssertEqual(MuseExitStatus(shellStatus: 137), .signaled(9))
        XCTAssertEqual(MuseExitStatus(shellStatus: 129), .signaled(1))
        XCTAssertEqual(MuseExitStatus(shellStatus: nil), .unknown)
        XCTAssertEqual(MuseExitStatus(shellStatus: 300), .unknown)
        XCTAssertEqual(MuseExitStatus(shellStatus: -1), .unknown)
    }

    func testStatusFileIsReadTrimmedAndMissingIsUnknown() throws {
        let url = FileManager.default.temporaryDirectory.appendingPathComponent("muse-status-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: url) }
        XCTAssertEqual(MuseExitStatus(contentsOf: url), .unknown)
        try "3\n".write(to: url, atomically: true, encoding: .utf8)
        XCTAssertEqual(MuseExitStatus(contentsOf: url), .exited(3))
        try "garbage".write(to: url, atomically: true, encoding: .utf8)
        XCTAssertEqual(MuseExitStatus(contentsOf: url), .unknown)
    }

    func testCleanExitFollowsTheBehaviorAndEveryFailureStaysOnScreen() {
        XCTAssertEqual(MuseExitAction.action(for: .exited(0), behavior: .relaunchOnShow), .hideAndRelaunchOnShow)
        XCTAssertEqual(MuseExitAction.action(for: .exited(0), behavior: .quitApp), .quitApp)
        for behavior in MuseExitBehavior.allCases {
            XCTAssertEqual(MuseExitAction.action(for: .exited(1), behavior: behavior),
                           .showFailure("muse exited with status 1."))
            XCTAssertEqual(MuseExitAction.action(for: .unknown, behavior: behavior),
                           .showFailure(MuseExitStatus.unknown.summary))
        }
        guard case .showFailure(let summary) = MuseExitAction.action(for: .signaled(9), behavior: .quitApp) else {
            return XCTFail("a signal is a failure")
        }
        XCTAssertTrue(summary.hasPrefix("muse was terminated by signal 9 (Killed"), summary)
    }

    /// Status 3 is muse's "another instance holds the lock", told apart from
    /// any other failure whatever the behavior.
    func testAlreadyRunningNamesTheHolder() {
        for behavior in MuseExitBehavior.allCases {
            XCTAssertEqual(MuseExitAction.action(for: .exited(3), behavior: behavior, holderPID: { 4242 }),
                           .alreadyRunning(pid: 4242))
        }
        XCTAssertEqual(MuseExitAction.alreadyRunning(pid: 4242).bannerMessage,
                       "muse is already running in a terminal (PID 4242). Quit it there, then Relaunch.")
        XCTAssertEqual(MuseExitAction.alreadyRunning(pid: nil).bannerMessage,
                       "muse is already running in a terminal. Quit it there, then Relaunch.")
        XCTAssertEqual(MuseExitAction.showFailure("muse exited with status 1.").bannerMessage,
                       "muse exited with status 1. Its last output is shown above.")
        XCTAssertNil(MuseExitAction.hideAndRelaunchOnShow.bannerMessage)
    }

    /// The lock path follows muse's rule: XDG_RUNTIME_DIR when absolute, else
    /// TMPDIR with the uid.
    func testInstanceLockIsReadWhereMuseWritesIt() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("muse-lock-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: dir) }
        XCTAssertEqual(MuseInstanceLock.path(environment: ["XDG_RUNTIME_DIR": dir.path]).path,
                       dir.appendingPathComponent("muse/muse.pid").path)
        XCTAssertEqual(MuseInstanceLock.path(environment: ["XDG_RUNTIME_DIR": "relative", "TMPDIR": dir.path]).path,
                       dir.appendingPathComponent("muse-\(getuid())/muse.pid").path)
        XCTAssertEqual(MuseInstanceLock.path(environment: [:]).path, "/tmp/muse-\(getuid())/muse.pid")

        let env = ["XDG_RUNTIME_DIR": dir.path]
        XCTAssertNil(MuseInstanceLock.holderPID(environment: env))
        let file = MuseInstanceLock.path(environment: env)
        try FileManager.default.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
        try "31337\n".write(to: file, atomically: true, encoding: .utf8)
        XCTAssertEqual(MuseInstanceLock.holderPID(environment: env), 31337)
    }

    func testBehaviorDefaultsToQuitAndRoundTrips() throws {
        let suite = "muse-tests-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        XCTAssertEqual(MuseExitBehavior.load(from: defaults), .quitApp)
        defaults.set("nonsense", forKey: MuseExitBehavior.defaultsKey)
        XCTAssertEqual(MuseExitBehavior.load(from: defaults), .quitApp)
        MuseExitBehavior.relaunchOnShow.save(to: defaults)
        XCTAssertEqual(MuseExitBehavior.load(from: defaults), .relaunchOnShow)
    }
}

final class MainMenuTests: XCTestCase {
    private func titles(_ menu: NSMenu?) -> [String] {
        (menu?.items ?? []).filter { !$0.isSeparatorItem }.map(\.title)
    }

    private func submenu(_ main: NSMenu, _ title: String) -> NSMenu? {
        main.items.first { $0.submenu?.title == title }?.submenu
    }

    func testAppMenuAndClipboardOnlyEditMenu() {
        let main = MainMenu.make()
        let app = titles(main.items.first?.submenu)
        for title in ["About muse", "Preferences…", "Relaunch muse", "Hide muse", "Quit muse"] {
            XCTAssertTrue(app.contains(title), "\(title) missing from \(app)")
        }
        XCTAssertEqual(titles(submenu(main, "Edit")), ["Copy", "Paste", "Select All"])
        XCTAssertEqual(main.items.compactMap { $0.submenu?.title }, ["", "Edit", "View", "Window", "Help"])
    }

    /// The zoom keys Ghostty already answers are named, so they can be found.
    func testViewMenuNamesTheZoomKeys() throws {
        let view = try XCTUnwrap(submenu(MainMenu.make(), "View"))
        XCTAssertEqual(titles(view), ["Actual Size", "Zoom In", "Zoom Out"])
        XCTAssertEqual(view.items.map(\.keyEquivalent), ["0", "=", "-"])
        XCTAssertEqual(view.items.map(\.action), [
            #selector(Ghostty.SurfaceView.resetZoom(_:)),
            #selector(Ghostty.SurfaceView.zoomIn(_:)),
            #selector(Ghostty.SurfaceView.zoomOut(_:)),
        ])
    }

    func testHelpMenuOpensTheReadme() throws {
        let help = try XCTUnwrap(submenu(MainMenu.make(), "Help"))
        XCTAssertEqual(titles(help), ["muse README"])
        XCTAssertEqual(MainMenu.readmeURL.host, "github.com")
        XCTAssertEqual(MainMenu.readmeURL.path, "/sglavoie/muse")
    }

    /// Cmd+, belongs to Preferences, never to Ghostty's open-config binding.
    func testPreferencesOwnsCommandComma() throws {
        let app = try XCTUnwrap(MainMenu.make().items.first?.submenu)
        let item = try XCTUnwrap(app.items.first { $0.title == "Preferences…" })
        XCTAssertEqual(item.keyEquivalent, ",")
        XCTAssertEqual(item.action, MainMenu.showPreferences)
    }

    func testAboutCreditsNameTheGhosttyVersionAndNotice() {
        XCTAssertFalse(Ghostty.versionDescription.hasPrefix("unknown"), Ghostty.versionDescription)
        let credits = MainMenu.aboutCredits(
            museVersion: "v1.2.3", ghosttyVersion: Ghostty.versionDescription, notice: "MIT License"
        ).string
        XCTAssertTrue(credits.contains("muse v1.2.3"))
        XCTAssertTrue(credits.contains("Ghostty \(Ghostty.versionDescription)"))
        XCTAssertFalse(MainMenu.aboutCredits(museVersion: nil, ghosttyVersion: "1", notice: nil).string.contains("muse"))
        XCTAssertTrue(credits.contains("MIT License"))
    }

    func testTheBundleShipsTheLauncherAndNotice() throws {
        let launcher = try XCTUnwrap(Bundle.main.url(forResource: "muse-launch", withExtension: nil))
        XCTAssertTrue(FileManager.default.isExecutableFile(atPath: launcher.path))
        let notice = try XCTUnwrap(Bundle.main.url(forResource: "NOTICE-ghostty", withExtension: nil))
        XCTAssertTrue(try String(contentsOf: notice, encoding: .utf8).contains("MIT License"))
    }
}

/// A real surface running a stand-in for muse through the bundled launcher.
final class TerminalWindowControllerTests: XCTestCase {
    private var dir: URL!
    private var ghostty: Ghostty.App!
    private var controller: TerminalWindowController!

    override func setUpWithError() throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("muse-lifecycle-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    }

    override func tearDownWithError() throws {
        // A stand-in left ignoring the hangup would hold teardown forever.
        if let pid = controller?.launcherPID, let group = TerminalWindowController.processGroup(of: pid) {
            killpg(group, SIGKILL)
        }
        controller?.teardown()
        controller?.window.orderOut(nil)
        controller = nil
        ghostty = nil
        try? FileManager.default.removeItem(at: dir)
    }

    /// The stand-in prints READY and its pid, then waits for a line on the
    /// pty: `exit N` exits with N, anything else is echoed back.
    private static let echoingFake = """
        #!/bin/sh
        printf 'READY:%s:\\n' "$$"
        while read -r line; do
          case "$line" in
            exit*) exit "${line#exit }" ;;
            bg*) printf '\\033]11;%s\\033\\\\' "${line#bg }" ;;
            *) printf 'GOT:%s:\\n' "$line" ;;
          esac
        done
        """

    /// A muse that will not drain: it ignores the hangup, as does the child
    /// standing in for mpv, so only a SIGKILL of the group ends them.
    private static let stubbornFake = """
        #!/bin/sh
        trap '' HUP
        sleep 600 &
        printf 'READY:%s:CHILD:%s:\\n' "$$" "$!"
        while :; do sleep 0.1; done
        """

    private func start(script body: String = echoingFake, holderPID: @escaping () -> Int32? = { nil }) throws {
        let script = dir.appendingPathComponent("fake-muse")
        try body.write(to: script, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: script.path)

        let launcher = try XCTUnwrap(Bundle.main.url(forResource: "muse-launch", withExtension: nil))
        let statusFile = dir.appendingPathComponent("status")
        let overrides = Ghostty.ConfigOverrides(
            command: launcher, arguments: [statusFile.path, script.path], waitAfterCommand: true
        )
        ghostty = Ghostty.App { Ghostty.Config(userConfig: nil, overrides: overrides) }
        let delegate = Delegate()
        ghostty.delegate = delegate
        self.delegate = delegate
        controller = TerminalWindowController(
            ghostty: ghostty, statusFile: statusFile, exitBehavior: { .relaunchOnShow }, holderPID: holderPID,
            frameAutosaveName: nil, hidesApp: false
        )
        delegate.controller = controller
        controller.window.setFrameOrigin(NSPoint(x: -10_000, y: -10_000))
        controller.show()
        try waitForScreen { $0.contains("READY:") }
    }

    private var delegate: Delegate?

    /// Stands in for AppDelegate's routing of the two exit signals.
    private final class Delegate: GhosttyAppDelegate {
        weak var controller: TerminalWindowController?

        func ghosttyApp(_ app: Ghostty.App, perform action: ghostty_action_s, target: ghostty_target_s) -> Bool {
            switch action.tag {
            case GHOSTTY_ACTION_SHOW_CHILD_EXITED:
                controller?.childExited(surface: target.target.surface)
            case GHOSTTY_ACTION_COLOR_CHANGE:
                let change = action.action.color_change
                DispatchQueue.main.async { [weak self] in self?.controller?.colorChanged(change) }
            default:
                return false
            }
            return true
        }

        func ghosttyApp(_ app: Ghostty.App, closeSurface host: GhosttySurfaceHost?, needsConfirm: Bool) {
            controller?.closeRequested(host: host)
        }
    }

    private func screenText() -> String {
        guard let surface = controller.surfaceView?.surface else { return "" }
        var text = ghostty_text_s()
        let selection = ghostty_selection_s(
            top_left: ghostty_point_s(tag: GHOSTTY_POINT_SCREEN, coord: GHOSTTY_POINT_COORD_TOP_LEFT, x: 0, y: 0),
            bottom_right: ghostty_point_s(tag: GHOSTTY_POINT_SCREEN, coord: GHOSTTY_POINT_COORD_BOTTOM_RIGHT, x: 0, y: 0),
            rectangle: false
        )
        guard ghostty_surface_read_text(surface, selection, &text) else { return "" }
        defer { ghostty_surface_free_text(surface, &text) }
        return String(cString: text.text)
    }

    private struct Timeout: Error {}

    private func wait(timeout: TimeInterval = 10, _ what: String, until done: () -> Bool) throws {
        let deadline = Date().addingTimeInterval(timeout)
        while !done() {
            if Date() > deadline {
                XCTFail("timed out waiting for \(what); the screen reads: \(screenText())")
                throw Timeout()
            }
            ghostty.appTick()
            RunLoop.current.run(until: Date().addingTimeInterval(0.02))
        }
    }

    private func waitForScreen(_ done: (String) -> Bool) throws {
        try wait("the screen to match") { done(screenText()) }
    }

    private func type(_ line: String) {
        guard let surface = controller.surfaceView?.surface else { return }
        let text = line + "\r"
        ghostty_surface_text(surface, text, UInt(text.utf8.count))
    }

    private func fakePID() throws -> pid_t {
        let screen = screenText()
        let parts = try XCTUnwrap(screen.components(separatedBy: "READY:").last?.split(separator: ":").first)
        return try XCTUnwrap(pid_t(parts))
    }

    /// muse announces its theme background with OSC 11; the title bar follows,
    /// with title text that reads on it.
    func testTheWindowTakesTheBackgroundTheProgramSets() throws {
        try start()
        XCTAssertTrue(controller.window.titlebarAppearsTransparent)

        type("bg #f6f6f0")
        try wait("a light background") { controller.window.backgroundColor.usingColorSpace(.sRGB)?.redComponent ?? 0 > 0.9 }
        XCTAssertEqual(controller.window.appearance?.name, .aqua)

        type("bg #1a1b26")
        try wait("a dark background") { controller.window.backgroundColor.usingColorSpace(.sRGB)?.redComponent ?? 1 < 0.2 }
        XCTAssertEqual(controller.window.appearance?.name, .darkAqua)
    }

    func testHideAndShowKeepTheSameSession() throws {
        try start()
        let surface = controller.surfaceView
        XCTAssertFalse(controller.windowShouldClose(controller.window))
        XCTAssertFalse(controller.window.isVisible)
        type("still here")
        controller.show()
        XCTAssertTrue(controller.window.isVisible)
        XCTAssertTrue(controller.surfaceView === surface)
        try waitForScreen { $0.contains("GOT:still here:") }
    }

    /// Cmd+W is Ghostty's close_surface binding; its callback must hide, not
    /// read as an exit.
    func testCloseBindingHidesWithoutEndingTheSession() throws {
        try start()
        let surfaceView = try XCTUnwrap(controller.surfaceView)
        let action = "close_surface"
        XCTAssertTrue(ghostty_surface_binding_action(surfaceView.surface, action, UInt(action.utf8.count)))
        try wait("the close binding to hide the window") { !controller.window.isVisible }
        XCTAssertTrue(controller.surfaceView === surfaceView)
        XCTAssertNil(controller.exitAction)
        controller.show()
        type("after close")
        try waitForScreen { $0.contains("GOT:after close:") }
    }

    func testCleanExitHidesAndTheNextShowStartsFresh() throws {
        try start()
        let firstPID = try fakePID()
        type("exit 0")
        try wait("the clean exit to hide the window") { controller.surfaceView == nil }
        XCTAssertFalse(controller.window.isVisible)
        controller.show()
        XCTAssertNotNil(controller.surfaceView)
        try waitForScreen { $0.contains("READY:") }
        XCTAssertNotEqual(try fakePID(), firstPID)
    }

    func testNonZeroExitKeepsTheOutputUnderARelaunchBanner() throws {
        try start()
        type("before the crash")
        try waitForScreen { $0.contains("GOT:before the crash:") }
        type("exit 4")
        try wait("the failure banner") { controller.isShowingFailure }
        XCTAssertEqual(controller.exitAction, .showFailure("muse exited with status 4."))
        XCTAssertTrue(controller.window.isVisible)
        XCTAssertTrue(screenText().contains("GOT:before the crash:"))

        // A key on the dead surface asks Ghostty to close it; the banner stays.
        controller.closeRequested(host: controller.surfaceView)
        XCTAssertTrue(controller.isShowingFailure)

        let surfaceBefore = controller.surfaceView
        let returnKey = try XCTUnwrap(NSEvent.keyEvent(
            with: .keyDown, location: .zero, modifierFlags: [], timestamp: ProcessInfo.processInfo.systemUptime,
            windowNumber: controller.window.windowNumber, context: nil,
            characters: "\r", charactersIgnoringModifiers: "\r", isARepeat: false, keyCode: 36
        ))
        NSApp.sendEvent(returnKey)
        XCTAssertFalse(controller.isShowingFailure)
        XCTAssertFalse(controller.surfaceView === surfaceBefore)
        XCTAssertNil(controller.exitAction)
        try waitForScreen { $0.contains("READY:") && !$0.contains("before the crash") }
    }

    func testOnlyAPlainReturnOrEnterRelaunches() {
        XCTAssertTrue(TerminalWindowController.isRelaunchKey("\r", []))
        XCTAssertTrue(TerminalWindowController.isRelaunchKey("\u{3}", [.numericPad, .function]))
        XCTAssertFalse(TerminalWindowController.isRelaunchKey("\r", [.command]))
        XCTAssertFalse(TerminalWindowController.isRelaunchKey("q", []))
    }

    func testKillingMuseShowsTheRelaunchAffordance() throws {
        try start()
        kill(try fakePID(), SIGKILL)
        try wait("the failure banner") { controller.isShowingFailure }
        guard case .showFailure(let summary) = controller.exitAction else {
            return XCTFail("expected a failure, got \(String(describing: controller.exitAction))")
        }
        XCTAssertTrue(summary.contains("signal 9"), summary)
    }

    func testExitThreeSaysAnotherMuseIsRunning() throws {
        try start(holderPID: { 4242 })
        type("exit 3")
        try wait("the already-running banner") { controller.isShowingFailure }
        XCTAssertEqual(controller.exitAction, .alreadyRunning(pid: 4242))
        XCTAssertTrue(controller.window.isVisible)
    }

    func testTheLauncherRecordsItsPidInMusesProcessGroup() throws {
        try start()
        let launcher = try XCTUnwrap(controller.launcherPID)
        XCTAssertEqual(getpgid(launcher), getpgid(try fakePID()))
        XCTAssertNotNil(TerminalWindowController.processGroup(of: launcher))
        XCTAssertNil(TerminalWindowController.processGroup(of: getpid()))
    }

    private func isAlive(_ pid: pid_t) -> Bool {
        kill(pid, 0) == 0
    }

    private func drain(timeout: TimeInterval) throws -> TimeInterval {
        var done = false
        let started = Date()
        controller.drainForQuit(link: nil, timeout: timeout) { done = true }
        try wait(timeout: timeout + 10, "the drain to finish") { done }
        return Date().timeIntervalSince(started)
    }

    /// A muse that honors the hangup ends well inside the wait; the launcher
    /// survives it to record the exit, and that exit decides nothing.
    func testQuitDrainEndsAMuseThatHonorsTheHangup() throws {
        try start()
        let pid = try fakePID()
        let elapsed = try drain(timeout: 5)
        XCTAssertLessThan(elapsed, 4)
        XCTAssertFalse(isAlive(pid))
        XCTAssertNil(controller.surfaceView)
        XCTAssertNil(controller.exitAction)
        XCTAssertFalse(controller.isShowingFailure)
    }

    /// A muse ignoring the hangup is SIGKILLed with its whole process group
    /// once the wait runs out, so its mpv cannot survive it.
    func testQuitDrainKillsTheProcessGroupAfterTheTimeout() throws {
        try start(script: Self.stubbornFake)
        try waitForScreen { $0.contains("CHILD:") }
        let pid = try fakePID()
        let child = try XCTUnwrap(screenText().components(separatedBy: "CHILD:").last?.split(separator: ":").first
            .flatMap { pid_t($0) })
        let elapsed = try drain(timeout: 1)
        XCTAssertGreaterThanOrEqual(elapsed, 1)
        try wait("the group to die") { !isAlive(pid) && !isAlive(child) }
        XCTAssertNil(controller.surfaceView)
    }
}
