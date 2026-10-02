import AppKit
import Carbon
import XCTest
@testable import muse

final class HotKeyComboTests: XCTestCase {
    func testEveryKeyAndModifierSetRoundTripsThroughItsString() {
        let modifierBits = [controlKey, optionKey, shiftKey, cmdKey].map(UInt32.init)
        for keyCode in HotKeyCombo.keys.keys {
            for mask in 0..<(1 << modifierBits.count) {
                let modifiers = modifierBits.enumerated()
                    .filter { mask & (1 << $0.offset) != 0 }
                    .reduce(UInt32(0)) { $0 | $1.element }
                guard let combo = HotKeyCombo(keyCode: keyCode, modifiers: modifiers) else { continue }
                XCTAssertEqual(HotKeyCombo(string: combo.string), combo, combo.string)
                XCTAssertEqual(combo.keyCode, keyCode)
                XCTAssertEqual(combo.modifiers, modifiers)
            }
        }
    }

    func testStringsParseToCarbonKeyCodesAndModifiers() throws {
        let combo = try XCTUnwrap(HotKeyCombo(string: "ctrl+opt+cmd+t"))
        XCTAssertEqual(combo.keyCode, UInt32(kVK_ANSI_T))
        XCTAssertEqual(combo.modifiers, UInt32(controlKey | optionKey | cmdKey))
        XCTAssertEqual(combo.displayString, "⌃⌥⌘T")

        // Aliases, case, spacing, and order all land on the canonical form.
        XCTAssertEqual(HotKeyCombo(string: "Command + Shift + Space")?.string, "shift+cmd+space")
        XCTAssertEqual(HotKeyCombo(string: "cmd+alt+control+f5")?.string, "ctrl+opt+cmd+f5")
        XCTAssertEqual(HotKeyCombo(string: "cmd+minus")?.displayString, "⌘-")
        XCTAssertEqual(HotKeyCombo(string: "f13")?.keyCode, UInt32(kVK_F13))
    }

    func testCombosThatWouldSwallowTypingOrDoNotParseAreRefused() {
        for bad in ["", "t", "shift+t", "cmd+", "+t", "cmd+cmd+t", "hyper+t", "cmd+t+x", "cmd+nosuchkey", "cmd++"] {
            XCTAssertNil(HotKeyCombo(string: bad), bad)
        }
        XCTAssertNil(HotKeyCombo(keyCode: 0xFF, modifiers: UInt32(cmdKey)))
        XCTAssertNotNil(HotKeyCombo(string: "shift+f1"))
    }

    func testRecordedEventsCarryTheirModifiers() throws {
        let event = try XCTUnwrap(NSEvent.keyEvent(
            with: .keyDown, location: .zero, modifierFlags: [.command, .option, .capsLock],
            timestamp: 0, windowNumber: 0, context: nil, characters: "k",
            charactersIgnoringModifiers: "k", isARepeat: false, keyCode: UInt16(kVK_ANSI_K)
        ))
        XCTAssertEqual(HotKeyCombo(event: event)?.string, "opt+cmd+k")
    }
}

final class ShellPreferenceDefaultsTests: XCTestCase {
    private var suite = ""
    private var defaults: UserDefaults!

    override func setUpWithError() throws {
        suite = "muse-tests-\(UUID().uuidString)"
        defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
    }

    func testHotKeyDefaultsOnCanBeTurnedOffAndRoundTrips() {
        XCTAssertEqual(MuseHotKeyPreference.defaultsKey, "museHotKey")
        XCTAssertEqual(MuseHotKeyPreference.load(from: defaults)?.string, "ctrl+opt+cmd+m")
        MuseHotKeyPreference.save(nil, to: defaults)
        XCTAssertNil(MuseHotKeyPreference.load(from: defaults))
        MuseHotKeyPreference.save(HotKeyCombo(string: "shift+cmd+space"), to: defaults)
        XCTAssertEqual(defaults.string(forKey: MuseHotKeyPreference.defaultsKey), "shift+cmd+space")
        XCTAssertEqual(MuseHotKeyPreference.load(from: defaults)?.string, "shift+cmd+space")
        defaults.set("shift+t", forKey: MuseHotKeyPreference.defaultsKey)
        XCTAssertEqual(MuseHotKeyPreference.load(from: defaults), MuseHotKeyPreference.default)
    }

    func testActivationPolicyDefaultsToTheDockAndRoundTrips() {
        XCTAssertEqual(MuseActivationPolicy.load(from: defaults), .dockAndMenuBar)
        XCTAssertEqual(MuseActivationPolicy.dockAndMenuBar.appKitPolicy, .regular)
        XCTAssertEqual(MuseActivationPolicy.menuBarOnly.appKitPolicy, .accessory)
        defaults.set("nonsense", forKey: MuseActivationPolicy.defaultsKey)
        XCTAssertEqual(MuseActivationPolicy.load(from: defaults), .dockAndMenuBar)
        MuseActivationPolicy.menuBarOnly.save(to: defaults)
        XCTAssertEqual(MuseActivationPolicy.load(from: defaults), .menuBarOnly)
    }

    func testDefaultFontSizeIsUnsetUntilSavedAndRefusesNonsense() {
        XCTAssertNil(MuseDefaultFontSize.load(from: defaults))
        MuseDefaultFontSize.save(15, to: defaults)
        XCTAssertEqual(MuseDefaultFontSize.load(from: defaults), 15)
        MuseDefaultFontSize.save(500, to: defaults)
        XCTAssertEqual(MuseDefaultFontSize.load(from: defaults), MuseDefaultFontSize.range.upperBound)
        for bad: Any in ["big", 2, 0, -4] {
            defaults.set(bad, forKey: MuseDefaultFontSize.defaultsKey)
            XCTAssertNil(MuseDefaultFontSize.load(from: defaults), "\(bad)")
        }
        MuseDefaultFontSize.save(nil, to: defaults)
        XCTAssertNil(defaults.object(forKey: MuseDefaultFontSize.defaultsKey))
        XCTAssertEqual(MuseDefaultFontSize.label(14), "14 pt")
        XCTAssertEqual(MuseDefaultFontSize.label(13.5), "13.5 pt")
    }

    func testNotificationsDefaultOnAndRoundTrip() {
        XCTAssertTrue(MuseNotificationsPreference.load(from: defaults))
        MuseNotificationsPreference.save(false, to: defaults)
        XCTAssertFalse(MuseNotificationsPreference.load(from: defaults))
        XCTAssertEqual(defaults.object(forKey: "museNotifications") as? Bool, false)
    }

    func testEnvironmentOverridesKeepKnownSingleLineValuesOnly() {
        MuseEnvironmentOverrides.save([
            "XDG_CONFIG_HOME": "  ~/cfg  ", "LANG": "", "PATH": "a\nb", "HOME": "/elsewhere", "XDG_DATA_HOME": "~/data",
        ], to: defaults)
        XCTAssertEqual(MuseEnvironmentOverrides.load(from: defaults), ["XDG_CONFIG_HOME": "~/cfg", "XDG_DATA_HOME": "~/data"])
        MuseEnvironmentOverrides.save([:], to: defaults)
        XCTAssertNil(defaults.object(forKey: MuseEnvironmentOverrides.defaultsKey))
        XCTAssertEqual(MuseEnvironmentOverrides.names.first, "PATH")
    }

    func testOverridesReplaceTheShellsValuesAndKeepTheRest() {
        let shell = LaunchEnvironment(home: "/Users/me", path: "/bin", forwarded: ["LANG": "en_CA.UTF-8", "XDG_DATA_HOME": "/d"])
        let applied = shell.applying(["PATH": "/opt/bin:/bin", "LANG": "fr_CA.UTF-8", "XDG_STATE_HOME": "/s", "HOME": "/x"])
        XCTAssertEqual(applied.commandEnvironment, [
            "HOME": "/Users/me", "PATH": "/opt/bin:/bin", "LANG": "fr_CA.UTF-8", "XDG_DATA_HOME": "/d", "XDG_STATE_HOME": "/s",
        ])
        XCTAssertEqual(applied.value(of: "PATH"), "/opt/bin:/bin")
        XCTAssertNil(shell.value(of: "XDG_CONFIG_HOME"))
    }

    func testAnEnvironmentEditAwaitsARelaunchOnlyWhileTheOldValuesRun() {
        let model = PreferencesModel(defaults: defaults)
        var changes = 0
        model.onEnvironmentChange = { changes += 1 }
        model.detectedEnvironment = LaunchEnvironment(home: "/h", path: "/bin", forwarded: ["LANG": "en_CA.UTF-8"])
        XCTAssertFalse(model.environmentAwaitsRelaunch, "no session runs")

        model.sessionEnvironment = model.effectiveEnvironment?.commandEnvironment
        XCTAssertFalse(model.environmentAwaitsRelaunch)

        model.environmentOverrides = ["LANG": "C"]
        XCTAssertTrue(model.environmentAwaitsRelaunch)
        XCTAssertEqual(changes, 1)
        XCTAssertEqual(PreferencesModel(defaults: defaults).environmentOverrides, ["LANG": "C"], "saved")

        model.environmentOverrides = [:]
        XCTAssertFalse(model.environmentAwaitsRelaunch, "back to what runs")
    }

    func testBinaryStatusNamesTheRunningVersionAndAnOlderCopy() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("muse-skew-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: dir) }
        let bundled = dir.appendingPathComponent("bundled/muse")
        let local = dir.appendingPathComponent("go/bin/muse")
        for url in [bundled, local] {
            try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
            try "#!/bin/sh\n".write(to: url, atomically: true, encoding: .utf8)
            try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: url.path)
        }
        let old = "version=v0.1.0 revision=a time=2026-01-01T00:00:00Z"
        let new = "version=v0.2.0 revision=b time=2026-02-01T00:00:00Z"
        let oldLine = "v0.1.0 (built 2026-01-01T00:00:00Z)"
        let newLine = "v0.2.0 (built 2026-02-01T00:00:00Z)"
        let versions = [bundled.path: new, local.path: old]

        let model = PreferencesModel(defaults: defaults)
        model.bundledBinaryPath = bundled.path
        model.goInstallBinaryPath = local.path
        model.readVersion = { versions[$0].flatMap(MuseVersion.parse) }
        model.refreshBinaryStatus()
        XCTAssertNil(model.runningBinaryNote, "nothing runs yet")

        model.runningBinary = .init(url: bundled, source: .bundled)
        model.refreshBinaryStatus()
        XCTAssertEqual(model.runningBinaryNote, "Running \(bundled.path): \(newLine)")
        XCTAssertTrue(model.binarySkewNote?.contains("is older (\(oldLine))") ?? false)

        model.runningBinary = .init(url: local, source: .newerGoInstall)
        model.readVersion = { [bundled.path: old, local.path: new][$0].flatMap(MuseVersion.parse) }
        model.refreshBinaryStatus()
        XCTAssertTrue(model.binarySkewNote?.hasPrefix("Chosen over the older bundled copy") ?? false)

        model.runningBinary = .init(url: bundled, source: .bundled)
        model.readVersion = { _ in MuseVersion.parse(new) }
        model.refreshBinaryStatus()
        XCTAssertNil(model.binarySkewNote, "matching copies say nothing")
    }

    func testTheModelStartsFromTheDefaultsAndSavesEachChange() {
        let model = PreferencesModel(defaults: defaults)
        XCTAssertEqual(model.exitBehavior, .relaunchOnShow)
        XCTAssertEqual(model.activationPolicy, .dockAndMenuBar)
        XCTAssertEqual(model.hotKey, MuseHotKeyPreference.default)
        XCTAssertEqual(model.binaryOverride, "")
        XCTAssertNil(model.binaryOverrideProblem)
        XCTAssertNil(model.defaultFontSize)

        var policies: [MuseActivationPolicy] = []
        var hotKeys: [HotKeyCombo?] = []
        var fontSizes: [Float?] = []
        model.onActivationPolicyChange = { policies.append($0) }
        model.onHotKeyChange = { hotKeys.append($0) }
        model.onDefaultFontSizeChange = { fontSizes.append($0) }

        model.defaultFontSize = 16
        model.defaultFontSize = 16
        XCTAssertEqual(MuseDefaultFontSize.load(from: defaults), 16)
        XCTAssertEqual(PreferencesModel(defaults: defaults).defaultFontSize, 16, "the next launch starts from it")
        model.defaultFontSize = nil
        XCTAssertEqual(fontSizes, [16, nil])
        XCTAssertNil(defaults.object(forKey: MuseDefaultFontSize.defaultsKey))

        model.activationPolicy = .menuBarOnly
        model.activationPolicy = .menuBarOnly
        model.hotKey = nil
        model.exitBehavior = .quitApp
        model.binaryOverride = " /no/such/muse "

        XCTAssertEqual(policies, [.menuBarOnly], "an unchanged value applies nothing")
        XCTAssertEqual(hotKeys, [nil])
        XCTAssertEqual(MuseActivationPolicy.load(from: defaults), .menuBarOnly)
        XCTAssertNil(MuseHotKeyPreference.load(from: defaults))
        XCTAssertEqual(MuseExitBehavior.load(from: defaults), .quitApp)
        XCTAssertEqual(defaults.string(forKey: MuseBinaryResolver.overrideDefaultsKey), "/no/such/muse")
        XCTAssertNotNil(model.binaryOverrideProblem)

        model.binaryOverride = "/bin/sh"
        XCTAssertNil(model.binaryOverrideProblem)
        model.binaryOverride = "  "
        XCTAssertNil(defaults.object(forKey: MuseBinaryResolver.overrideDefaultsKey))
    }
}

final class TerminalToggleTests: XCTestCase {
    func testHidesOnlyWhenTheWindowIsAlreadyInFront() {
        XCTAssertEqual(TerminalToggle.action(appIsActive: true, windowIsVisible: true, windowIsKey: true), .hide)
        XCTAssertEqual(TerminalToggle.action(appIsActive: false, windowIsVisible: true, windowIsKey: true), .show)
        XCTAssertEqual(TerminalToggle.action(appIsActive: true, windowIsVisible: false, windowIsKey: false), .show)
        // Preferences in front: the shortcut brings muse forward rather than hiding.
        XCTAssertEqual(TerminalToggle.action(appIsActive: true, windowIsVisible: true, windowIsKey: false), .show)
    }
}

final class HotKeyCenterTests: XCTestCase {
    func testRegistersReplacesAndTurnsOff() throws {
        let center = HotKeyCenter()
        let first = try XCTUnwrap(HotKeyCombo(string: "ctrl+opt+shift+cmd+f19"))
        let second = try XCTUnwrap(HotKeyCombo(string: "ctrl+opt+shift+cmd+f18"))
        XCTAssertEqual(center.register(first), .registered(first))
        XCTAssertNil(center.registration.problem)
        XCTAssertEqual(center.register(second), .registered(second))
        XCTAssertEqual(center.register(nil), .none)
        center.unregister()
        XCTAssertEqual(center.registration, .none)
    }

    /// A press arrives as a Carbon event naming the registration; only the
    /// center that owns that ID answers it.
    func testAPressReachesOnlyItsOwnCenter() throws {
        let center = HotKeyCenter()
        let other = HotKeyCenter()
        var presses: [String] = []
        center.onPress = { presses.append("center") }
        other.onPress = { presses.append("other") }

        var eventRef: EventRef?
        XCTAssertEqual(CreateEvent(nil, OSType(kEventClassKeyboard), UInt32(kEventHotKeyPressed), 0, 0, &eventRef), noErr)
        let event = try XCTUnwrap(eventRef)
        defer { ReleaseEvent(event) }
        var id = center.hotKeyID
        XCTAssertEqual(SetEventParameter(event, EventParamName(kEventParamDirectObject), EventParamType(typeEventHotKeyID),
                                         MemoryLayout<EventHotKeyID>.size, &id), noErr)
        XCTAssertEqual(SendEventToEventTarget(event, GetApplicationEventTarget()), noErr)
        XCTAssertEqual(presses, ["center"])
    }

    func testFailuresNameTheShortcut() throws {
        let combo = try XCTUnwrap(HotKeyCombo(string: "ctrl+opt+cmd+t"))
        XCTAssertEqual(HotKeyCenter.signature, 0x6D73_484B, "'msHK'")
        let taken = HotKeyCenter.Registration.failed(combo, OSStatus(eventHotKeyExistsErr)).problem
        XCTAssertEqual(taken, "⌃⌥⌘T is already taken by another app. Choose another shortcut.")
        XCTAssertTrue(HotKeyCenter.Registration.failed(combo, -1).problem?.contains("error -1") == true)
    }

    func testSystemShortcutsAreReadable() {
        // Whatever the machine's settings, reading them must not fail or
        // yield combos the recorder itself would refuse.
        for combo in SystemHotKeys.enabledCombos() {
            XCTAssertEqual(HotKeyCombo(string: combo.string), combo)
        }
    }
}

private final class FakeStatusDelegate: StatusItemDelegate {
    var isTerminalFrontmost = false
    var canRelaunchTerminal = true
    var isLaunchAtLoginEnabled = false
    var calls: [String] = []
    func toggleTerminal() { calls.append("toggle") }
    func relaunchTerminal() { calls.append("relaunch") }
    func showPreferences(_ sender: Any?) { calls.append("preferences") }
    func toggleLaunchAtLogin() { calls.append("login") }
    func togglePlayback() { calls.append("play-pause") }
    func playNext() { calls.append("next") }
    func playPrevious() { calls.append("previous") }
}

final class StatusItemTests: XCTestCase {
    func testMenuOffersTheShellActionsAndTracksState() throws {
        let delegate = FakeStatusDelegate()
        let controller = StatusItemController(delegate: delegate)
        XCTAssertTrue(try XCTUnwrap(controller.statusItem.button?.image).isTemplate)

        controller.refreshMenu()
        let titles = controller.menu.items.filter { !$0.isSeparatorItem && !$0.isHidden }.map(\.title)
        XCTAssertEqual(titles, [
            "Not playing", "Play", "Next", "Previous",
            "Show muse", "Relaunch muse", "Preferences…", "Launch at Login", "Quit muse",
        ])
        XCTAssertEqual(controller.menu.items.last?.action, #selector(NSApplication.terminate(_:)))

        delegate.isTerminalFrontmost = true
        delegate.isLaunchAtLoginEnabled = true
        delegate.canRelaunchTerminal = false
        controller.refreshMenu()
        XCTAssertNotNil(controller.menu.item(withTitle: "Hide muse"))
        XCTAssertFalse(controller.menu.item(withTitle: "Relaunch muse")?.isEnabled ?? true)
        XCTAssertEqual(controller.menu.item(withTitle: "Launch at Login")?.state, .on)

        for title in ["Hide muse", "Relaunch muse", "Preferences…", "Launch at Login"] {
            let item = try XCTUnwrap(controller.menu.item(withTitle: title))
            _ = (item.target as? NSObject)?.perform(item.action, with: item)
        }
        XCTAssertEqual(delegate.calls, ["toggle", "relaunch", "preferences", "login"])
    }

    func testRightAndControlClicksOpenTheMenu() throws {
        func mouse(_ type: NSEvent.EventType, _ flags: NSEvent.ModifierFlags = []) -> NSEvent? {
            NSEvent.mouseEvent(with: type, location: .zero, modifierFlags: flags, timestamp: 0,
                               windowNumber: 0, context: nil, eventNumber: 0, clickCount: 1, pressure: 0)
        }
        XCTAssertFalse(StatusItemController.opensMenu(mouse(.leftMouseUp)))
        XCTAssertTrue(StatusItemController.opensMenu(mouse(.rightMouseUp)))
        XCTAssertTrue(StatusItemController.opensMenu(mouse(.leftMouseUp, .control)))
    }
}
