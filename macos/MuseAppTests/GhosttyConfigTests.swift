import GhosttyKit
import XCTest
@testable import muse

/// The hosting app has already run `ghostty_init`, so configs load here.
final class GhosttyConfigTests: XCTestCase {
    private var dir: URL!

    override func setUpWithError() throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("muse-config-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: dir)
    }

    private func writeUserConfig(_ text: String) throws -> URL {
        let url = dir.appendingPathComponent("ghostty.conf")
        try text.write(to: url, atomically: true, encoding: .utf8)
        return url
    }

    private let overrides = Ghostty.ConfigOverrides(
        command: URL(fileURLWithPath: "/usr/local/bin/muse"),
        environment: ["PATH": "/opt/bin:/usr/bin", "HOME": "/Users/someone"]
    )

    func testUserConfigPathIsUnderMuseAppConfigDir() {
        let home = URL(fileURLWithPath: "/Users/someone")
        XCTAssertEqual(Ghostty.Config.userConfigURL(home: home).path, "/Users/someone/.config/muse-app/ghostty.conf")
    }

    func testOverrideLinesForceCommandAndCloseBehavior() {
        XCTAssertEqual(overrides.lines, [
            "command = direct:/usr/local/bin/muse",
            "wait-after-command = false",
            "confirm-close-surface = false",
            "env = HOME=/Users/someone",
            "env = PATH=/opt/bin:/usr/bin",
        ])
    }

    func testCommandWithSpacesIsShellQuoted() {
        let url = URL(fileURLWithPath: "/Users/some one/it's/muse")
        XCTAssertEqual(Ghostty.ConfigOverrides.commandValue(for: url), #"shell:'/Users/some one/it'\''s/muse'"#)
    }

    func testArgumentsFollowTheCommandAndAnySpaceQuotesEveryWord() {
        let launcher = URL(fileURLWithPath: "/Applications/muse.app/Contents/Resources/muse-launch")
        XCTAssertEqual(
            Ghostty.ConfigOverrides.commandValue(for: launcher, arguments: ["/tmp/s", "/usr/local/bin/muse"]),
            "direct:/Applications/muse.app/Contents/Resources/muse-launch /tmp/s /usr/local/bin/muse"
        )
        XCTAssertEqual(
            Ghostty.ConfigOverrides.commandValue(for: launcher, arguments: ["/tmp/s", "/Users/a b/muse"]),
            "shell:'/Applications/muse.app/Contents/Resources/muse-launch' '/tmp/s' '/Users/a b/muse'"
        )
    }

    /// Ghostty runs a `shell:` command as `bash -c "exec -l <value>"`.
    func testShellFormRunsUnderGhosttysExecPrefix() throws {
        let value = Ghostty.ConfigOverrides.commandValue(
            for: URL(fileURLWithPath: "/bin/echo"), arguments: ["it's", "a b"]
        )
        let body = try XCTUnwrap(value.split(separator: ":", maxSplits: 1).last)
        let bash = Process()
        bash.executableURL = URL(fileURLWithPath: "/bin/bash")
        bash.arguments = ["--noprofile", "--norc", "-c", "exec -l \(body)"]
        let out = Pipe()
        bash.standardOutput = out
        try bash.run()
        bash.waitUntilExit()
        XCTAssertEqual(bash.terminationStatus, 0)
        XCTAssertEqual(String(decoding: out.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self), "it's a b\n")
    }

    func testWaitAfterCommandIsAnOverride() {
        var o = overrides
        o.waitAfterCommand = true
        XCTAssertTrue(o.lines.contains("wait-after-command = true"))
        let config = Ghostty.Config(userConfig: dir.appendingPathComponent("missing.conf"), overrides: o)
        XCTAssertEqual(config.bool("wait-after-command"), true)
    }

    func testMultilineEnvironmentValuesAreDropped() {
        let o = Ghostty.ConfigOverrides(command: URL(fileURLWithPath: "/muse"), environment: ["X": "a\ncommand = /bin/sh"])
        XCTAssertFalse(o.lines.contains { $0.hasPrefix("env = X") })
        XCTAssertEqual(o.lines.filter { $0.hasPrefix("command") }.count, 1)
    }

    func testDefaultsApplyWithoutAUserFile() {
        let config = Ghostty.Config(userConfig: dir.appendingPathComponent("missing.conf"), overrides: overrides)
        XCTAssertTrue(config.loaded)
        XCTAssertEqual(config.diagnostics, [])
        XCTAssertEqual(config.bool("wait-after-command"), false)
        XCTAssertEqual(config.string("confirm-close-surface"), "false")
    }

    func testUserFileLoadsAfterDefaultsAndBeforeOverrides() throws {
        let user = try writeUserConfig("""
            wait-after-command = true
            confirm-close-surface = always
            font-size = 17
            """)
        let config = Ghostty.Config(userConfig: user, overrides: overrides)
        XCTAssertTrue(config.loaded)
        XCTAssertEqual(config.diagnostics, [])
        // The user file beats the defaults...
        XCTAssertEqual(config.get("font-size", as: Float.self, initial: 0), 17)
        // ...and the forced overrides beat the user file.
        XCTAssertEqual(config.bool("wait-after-command"), false)
        XCTAssertEqual(config.string("confirm-close-surface"), "false")
    }

    func testDefaultFontSizeOverridesTheUserFileOnlyWhenSet() throws {
        let user = try writeUserConfig("font-size = 17\n")
        var sized = overrides
        sized.fontSize = 15
        XCTAssertTrue(sized.lines.contains("font-size = 15"))
        XCTAssertFalse(overrides.lines.contains { $0.hasPrefix("font-size") })
        sized.fontSize = 12.5
        XCTAssertTrue(sized.lines.contains("font-size = 12.5"))

        let config = Ghostty.Config(userConfig: user, overrides: sized)
        XCTAssertEqual(config.diagnostics, [])
        XCTAssertEqual(config.get("font-size", as: Float.self, initial: 0), 12.5)
        XCTAssertEqual(
            Ghostty.Config(userConfig: user, overrides: overrides).get("font-size", as: Float.self, initial: 0), 17)
    }

    func testUserConfigDiagnosticsAreReported() throws {
        let user = try writeUserConfig("not-a-real-ghostty-key = 1\n")
        let config = Ghostty.Config(userConfig: user, overrides: overrides)
        XCTAssertTrue(config.loaded)
        XCTAssertFalse(config.diagnostics.isEmpty)
    }

    /// A standard theme resolves from the resources muse.app bundles, with no
    /// Ghostty install or user theme behind it.
    func testBundledThemeSuppliesItsColors() throws {
        if ProcessInfo.processInfo.environment["GHOSTTY_RESOURCES_DIR"] != nil {
            throw XCTSkip("GHOSTTY_RESOURCES_DIR points libghostty away from the bundle")
        }
        let resources = try XCTUnwrap(Bundle.main.resourceURL)
        XCTAssertTrue(FileManager.default.fileExists(
            atPath: resources.appendingPathComponent("terminfo/78/xterm-ghostty").path))
        XCTAssertTrue(FileManager.default.fileExists(
            atPath: resources.appendingPathComponent("ghostty/themes/Catppuccin Mocha").path))

        let user = try writeUserConfig("theme = Catppuccin Mocha\n")
        let config = Ghostty.Config(userConfig: user, overrides: overrides)
        XCTAssertEqual(config.diagnostics, [])
        let background = try XCTUnwrap(config.get("background", as: ghostty_config_color_s.self, initial: .init()))
        XCTAssertEqual([background.r, background.g, background.b], [0x1E, 0x1E, 0x2E])
        let foreground = try XCTUnwrap(config.get("foreground", as: ghostty_config_color_s.self, initial: .init()))
        XCTAssertEqual([foreground.r, foreground.g, foreground.b], [0xCD, 0xD6, 0xF4])
    }

    func testAppCreatesGhosttyAppObject() {
        let app = Ghostty.App { Ghostty.Config(userConfig: nil, overrides: self.overrides) }
        XCTAssertEqual(app.readiness, .ready)
        XCTAssertNotNil(app.app)
    }

    func testTabSplitQuickTerminalAndNewWindowActionsAreIgnored() {
        let ignored = [
            GHOSTTY_ACTION_NEW_WINDOW,
            GHOSTTY_ACTION_NEW_TAB,
            GHOSTTY_ACTION_CLOSE_TAB,
            GHOSTTY_ACTION_GOTO_TAB,
            GHOSTTY_ACTION_MOVE_TAB,
            GHOSTTY_ACTION_NEW_SPLIT,
            GHOSTTY_ACTION_GOTO_SPLIT,
            GHOSTTY_ACTION_RESIZE_SPLIT,
            GHOSTTY_ACTION_EQUALIZE_SPLITS,
            GHOSTTY_ACTION_TOGGLE_SPLIT_ZOOM,
            GHOSTTY_ACTION_TOGGLE_QUICK_TERMINAL,
            GHOSTTY_ACTION_TOGGLE_WINDOW_DECORATIONS,
        ]
        for tag in ignored {
            XCTAssertEqual(Ghostty.disposition(of: tag), .ignored, "action \(tag.rawValue)")
        }
        XCTAssertEqual(Ghostty.disposition(of: GHOSTTY_ACTION_QUIT), .app)
        XCTAssertEqual(Ghostty.disposition(of: GHOSTTY_ACTION_SET_TITLE), .surface)
    }
}
