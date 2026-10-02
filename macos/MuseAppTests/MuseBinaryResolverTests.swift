import AppKit
import XCTest
@testable import muse

final class MuseBinaryResolverTests: XCTestCase {
    private var root: URL!
    private var home: URL!
    private var resources: URL!
    private var pathA: URL!
    private var pathB: URL!

    override func setUpWithError() throws {
        root = FileManager.default.temporaryDirectory.appendingPathComponent("muse-resolve-\(UUID().uuidString)")
        home = root.appendingPathComponent("home")
        resources = root.appendingPathComponent("muse.app/Contents/Resources")
        pathA = root.appendingPathComponent("path-a")
        pathB = root.appendingPathComponent("path-b")
        for dir in [home!, resources!, pathA!, pathB!] {
            try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        }
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: root)
    }

    @discardableResult
    private func makeBinary(_ url: URL, executable: Bool = true) throws -> URL {
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try "#!/bin/sh\n".write(to: url, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: executable ? 0o755 : 0o644], ofItemAtPath: url.path)
        return url
    }

    private func resolver(
        override: String? = nil, goBin: String? = nil, goPath: String? = nil, built: [URL: Date] = [:]
    ) -> MuseBinaryResolver {
        MuseBinaryResolver(
            overridePath: override,
            bundleResourcesURL: resources,
            homeDirectory: home,
            searchPath: "\(pathA.path)::\(pathB.path)",
            goBin: goBin,
            goPath: goPath,
            buildTime: { path in built[URL(fileURLWithPath: path)] }
        )
    }

    private var bundled: URL { resources.appendingPathComponent("bin/muse") }
    private var goInstalled: URL { home.appendingPathComponent("go/bin/muse") }

    func testOverrideWinsOverEverything() throws {
        let custom = try makeBinary(root.appendingPathComponent("custom/muse"))
        try makeBinary(bundled)
        try makeBinary(goInstalled)
        try makeBinary(pathA.appendingPathComponent("muse"))
        XCTAssertEqual(try resolver(override: custom.path).resolve().get(),
                       .init(url: custom, source: .userDefaultsOverride))
    }

    func testOverrideExpandsTilde() throws {
        try makeBinary(goInstalled)
        XCTAssertEqual(try resolver(override: "~/go/bin/muse").resolve().get(),
                       .init(url: goInstalled, source: .userDefaultsOverride))
    }

    func testBlankOverrideIsUnset() throws {
        try makeBinary(bundled)
        XCTAssertEqual(try resolver(override: "  ").resolve().get().source, .bundled)
    }

    func testInvalidOverrideFailsClosed() throws {
        try makeBinary(bundled)
        let missing = root.appendingPathComponent("nope/muse").path
        guard case .failure(let error) = resolver(override: missing).resolve() else {
            return XCTFail("an unusable override must not fall back")
        }
        XCTAssertEqual(error.invalidOverride, missing)
        XCTAssertTrue(error.message.contains(missing))
        XCTAssertTrue(error.message.contains("defaults delete io.github.sglavoie.muse museBinaryPath"))
    }

    func testBundledBeatsGoInstallAndPath() throws {
        try makeBinary(bundled)
        try makeBinary(goInstalled)
        try makeBinary(pathA.appendingPathComponent("muse"))
        XCTAssertEqual(try resolver().resolve().get(), .init(url: bundled, source: .bundled))
    }

    func testNewerGoInstallBeatsOlderBundled() throws {
        try makeBinary(bundled)
        try makeBinary(goInstalled)
        let old = Date(timeIntervalSince1970: 1_000)
        let new = Date(timeIntervalSince1970: 2_000)

        XCTAssertEqual(try resolver(built: [bundled: old, goInstalled: new]).resolve().get(),
                       .init(url: goInstalled, source: .newerGoInstall))
        XCTAssertEqual(try resolver(built: [bundled: new, goInstalled: old]).resolve().get().source, .bundled)
        XCTAssertEqual(try resolver(built: [bundled: old, goInstalled: old]).resolve().get().source, .bundled)
        // An unknown build time on either side keeps the bundled copy.
        XCTAssertEqual(try resolver(built: [goInstalled: new]).resolve().get().source, .bundled)
        XCTAssertEqual(try resolver(built: [bundled: old]).resolve().get().source, .bundled)
    }

    func testOverrideBeatsNewerGoInstall() throws {
        let custom = try makeBinary(root.appendingPathComponent("custom/muse"))
        try makeBinary(bundled)
        try makeBinary(goInstalled)
        let r = resolver(override: custom.path, built: [bundled: .distantPast, goInstalled: .distantFuture])
        XCTAssertEqual(try r.resolve().get().source, .userDefaultsOverride)
    }

    func testBuildInfoParsing() {
        let v = MuseVersion.parse("version=v1.2.3 revision=7516e86a time=2026-10-01T02:45:16Z\n")
        XCTAssertEqual(v?.version, "v1.2.3")
        XCTAssertEqual(v?.built, ISO8601DateFormatter().date(from: "2026-10-01T02:45:16Z"))
        XCTAssertNotNil(MuseVersion.parse("version=dev revision= time=2026-10-01T02:45:16.5Z")?.built)
        XCTAssertNil(MuseVersion.parse("version=dev revision= time=")?.built)
        XCTAssertNil(MuseVersion.parse("Usage: muse [command]"))
    }

    func testHelpProbeFindsTheControlFlagLine() {
        let help = """
        Usage: muse [command]

        Flags:
          -h, --help     Show this help and exit
          --control PATH Serve a JSON control socket at PATH for a host app
        """
        XCTAssertTrue(MuseVersion.helpLists(flag: "--control", in: help))
        XCTAssertFalse(MuseVersion.helpLists(flag: "--control", in: "use --control to serve a socket"))
    }

    /// A stand-in muse answering build-info and --help as the real one does.
    func testVersionAndFlagsAreReadFromTheBinary() throws {
        let fake = root.appendingPathComponent("fake/muse")
        try FileManager.default.createDirectory(at: fake.deletingLastPathComponent(), withIntermediateDirectories: true)
        try """
        #!/bin/sh
        case "$1" in
          build-info) echo 'version=v9 revision=abc time=2026-10-01T02:45:16Z' ;;
          --help) printf 'Flags:\\n  --control PATH  Serve a socket\\n' ;;
          *) exit 2 ;;
        esac
        """.write(to: fake, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: fake.path)
        XCTAssertEqual(MuseVersion.read(binary: fake.path)?.version, "v9")
        XCTAssertTrue(MuseVersion.supports(flag: "--control", binary: fake.path))
        XCTAssertFalse(MuseVersion.supports(flag: "--notify", binary: fake.path))
    }

    func testGoInstallPathFollowsGoBinThenGoPath() {
        XCTAssertEqual(resolver().goInstallPath, home.appendingPathComponent("go/bin/muse").path)
        XCTAssertEqual(resolver(goPath: "/gp:/other").goInstallPath, "/gp/bin/muse")
        XCTAssertEqual(resolver(goBin: "/gb", goPath: "/gp").goInstallPath, "/gb/muse")
        XCTAssertEqual(resolver(goBin: "~/bin").goInstallPath, home.appendingPathComponent("bin/muse").path)
        XCTAssertEqual(resolver(goBin: " ", goPath: "").goInstallPath, home.appendingPathComponent("go/bin/muse").path)
    }

    func testGoBinIsSearchedAndCanBeNewer() throws {
        let goBin = root.appendingPathComponent("gobin")
        let installed = try makeBinary(goBin.appendingPathComponent("muse"))
        XCTAssertEqual(try resolver(goBin: goBin.path).resolve().get(), .init(url: installed, source: .goInstall))
        try makeBinary(bundled)
        let r = resolver(goBin: goBin.path, built: [bundled: .distantPast, installed: .distantFuture])
        XCTAssertEqual(try r.resolve().get(), .init(url: installed, source: .newerGoInstall))
    }

    func testGoInstallBeatsPath() throws {
        try makeBinary(goInstalled)
        try makeBinary(pathA.appendingPathComponent("muse"))
        XCTAssertEqual(try resolver().resolve().get(), .init(url: goInstalled, source: .goInstall))
    }

    func testPathIsSearchedInOrderSkippingNonExecutablesAndDirectories() throws {
        try makeBinary(pathA.appendingPathComponent("muse"), executable: false)
        let second = try makeBinary(pathB.appendingPathComponent("muse"))
        XCTAssertEqual(try resolver().resolve().get(), .init(url: second, source: .path))

        try FileManager.default.removeItem(at: second)
        try FileManager.default.createDirectory(at: second, withIntermediateDirectories: true)
        XCTAssertThrowsError(try resolver().resolve().get())
    }

    func testMissingBinaryReportsEveryCandidate() {
        guard case .failure(let error) = resolver().resolve() else {
            return XCTFail("nothing is installed")
        }
        XCTAssertNil(error.invalidOverride)
        XCTAssertEqual(error.tried, [
            bundled.path,
            goInstalled.path,
            pathA.appendingPathComponent("muse").path,
            pathB.appendingPathComponent("muse").path,
        ])
        for path in error.tried {
            XCTAssertTrue(error.message.contains(path))
        }
        XCTAssertTrue(error.message.contains("just install"))
    }

    func testMissingBinaryErrorWindowShowsTheMessage() {
        guard case .failure(let error) = resolver().resolve() else {
            return XCTFail("nothing is installed")
        }
        let window = LaunchErrorWindow.make(title: "muse could not be found.", message: error.message)
        let labels = allLabels(in: window.contentView!)
        XCTAssertTrue(labels.contains("muse could not be found."))
        XCTAssertTrue(labels.contains(error.message))
    }

    private func allLabels(in view: NSView) -> [String] {
        var out: [String] = []
        if let field = view as? NSTextField { out.append(field.stringValue) }
        for sub in view.subviews { out.append(contentsOf: allLabels(in: sub)) }
        return out
    }

    func testLoginShellReportExtraction() {
        let noisy = "welcome!\n__B__PATH=/a:/b\0LANG=fr_CA.UTF-8\0LC_ALL=\0X=a=b\0__E__trailing"
        XCTAssertEqual(
            LaunchEnvironment.extractReport(from: noisy, begin: "__B__", end: "__E__"),
            ["PATH": "/a:/b", "LANG": "fr_CA.UTF-8", "X": "a=b"]
        )
        XCTAssertNil(LaunchEnvironment.extractReport(from: "no markers", begin: "__B__", end: "__E__"))
    }

    func testLoginShellEnvironmentFromRealShell() {
        let report = LaunchEnvironment.loginShellEnvironment(shell: "/bin/sh", interactive: false)
        XCTAssertTrue(report?["PATH"]?.contains("/usr/bin") ?? false)
    }

    func testCommandEnvironmentForwardsEditorsButKeepsHomeAndPath() {
        let env = LaunchEnvironment(
            home: "/Users/me",
            path: "/bin",
            forwarded: ["XDG_DATA_HOME": "/data", "LC_CTYPE": "UTF-8"],
            probed: ["GOBIN": "/gb"]
        )
        XCTAssertEqual(env.commandEnvironment, [
            "HOME": "/Users/me", "PATH": "/bin", "XDG_DATA_HOME": "/data", "LC_CTYPE": "UTF-8",
        ])
    }

    func testReportScriptNamesEveryForwardedVariable() {
        for name in ["PATH"] + LaunchEnvironment.forwardedNames + LaunchEnvironment.probedNames {
            XCTAssertTrue(LaunchEnvironment.reportScript.contains("'\(name)' \"$\(name)\""), name)
        }
    }

    func testForwardsXDGLocaleAndEveryLCVariable() {
        for name in ["XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR", "LANG"] {
            XCTAssertTrue(LaunchEnvironment.forwardedNames.contains(name), name)
        }
        XCTAssertFalse(LaunchEnvironment.forwardedNames.contains("TERM"))
        XCTAssertFalse(LaunchEnvironment.forwardedNames.contains("COLORTERM"))
        let env = LaunchEnvironment.make(
            home: "/h", fallbackPath: "/usr/bin",
            reported: ["PATH": "/p", "XDG_DATA_HOME": "/d", "LC_TIME": "en_GB", "GOPATH": "/gp", "EDITOR": "vi"],
            processEnvironment: ["LC_CTYPE": "UTF-8", "XDG_STATE_HOME": "/s", "TERM": "dumb"]
        )
        XCTAssertEqual(env.path, "/p")
        XCTAssertEqual(env.forwarded, ["XDG_DATA_HOME": "/d", "LC_TIME": "en_GB", "LC_CTYPE": "UTF-8", "XDG_STATE_HOME": "/s"])
        XCTAssertEqual(env.probed, ["GOPATH": "/gp"])
        XCTAssertNil(env.commandEnvironment["GOPATH"])
    }

    /// The script, run by a real shell, reports exported LC_ variables it
    /// does not name.
    func testReportScriptEnumeratesLCVariables() throws {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/sh")
        process.arguments = ["-c", LaunchEnvironment.reportScript]
        process.environment = ["PATH": "/usr/bin:/bin", "LC_MESSAGES": "fr_CA.UTF-8", "GOBIN": "/gb"]
        let out = Pipe()
        process.standardOutput = out
        try process.run()
        let data = out.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        let report = LaunchEnvironment.extractReport(
            from: String(decoding: data, as: UTF8.self),
            begin: LaunchEnvironment.beginMarker, end: LaunchEnvironment.endMarker
        )
        XCTAssertEqual(report?["LC_MESSAGES"], "fr_CA.UTF-8")
        XCTAssertEqual(report?["GOBIN"], "/gb")
        XCTAssertEqual(report?["PATH"], "/usr/bin:/bin")
    }

    func testEnvironmentOverridesReplaceTheShellsValues() throws {
        let env = LaunchEnvironment(home: "/h", path: "/bin", forwarded: ["LANG": "C"])
        let applied = env.applying(["PATH": " /opt/bin ", "XDG_DATA_HOME": "/d", "LANG": "", "EDITOR": "vi"])
        XCTAssertEqual(applied.path, "/opt/bin")
        XCTAssertEqual(applied.forwarded, ["LANG": "C", "XDG_DATA_HOME": "/d"])

        let suite = "muse-tests-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        MuseEnvironmentOverrides.save(["LANG": "fr_CA.UTF-8", "BOGUS": "x"], to: defaults)
        XCTAssertEqual(MuseEnvironmentOverrides.load(from: defaults), ["LANG": "fr_CA.UTF-8"])
        MuseEnvironmentOverrides.save([:], to: defaults)
        XCTAssertNil(defaults.object(forKey: MuseEnvironmentOverrides.defaultsKey))
    }
}
