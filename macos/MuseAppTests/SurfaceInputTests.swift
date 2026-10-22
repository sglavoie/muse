import AppKit
import GhosttyKit
import XCTest
@testable import muse

/// The key translation the surface hands libghostty, checked two ways: the
/// `ghostty_input_key_s` fields themselves, and the bytes a program on the
/// pty receives once it has asked for the kitty keyboard protocol, which is
/// the only place ctrl+m, ctrl+i, shift+enter, and ctrl+j differ from the keys
/// they alias, and the only way muse's ctrl+, reaches it at all.
final class SurfaceInputTests: XCTestCase {
    // US virtual key codes.
    private static let kVKReturn: UInt16 = 0x24
    private static let kVKTab: UInt16 = 0x30
    private static let kVKM: UInt16 = 0x2E
    private static let kVKI: UInt16 = 0x22
    private static let kVKJ: UInt16 = 0x26
    private static let kVKE: UInt16 = 0x0E
    private static let kVKC: UInt16 = 0x08
    private static let kVKComma: UInt16 = 0x2B
    private static let kVKLeftArrow: UInt16 = 0x7B
    private static let kVKRightArrow: UInt16 = 0x7C

    private func requireUSLayout() throws {
        let id = KeyboardLayout.id ?? ""
        guard id.hasSuffix(".US") || id.hasSuffix(".ABC") else {
            throw XCTSkip("characters(byApplyingModifiers:) follows the active layout (\(id))")
        }
    }

    private func key(
        _ keyCode: UInt16,
        _ characters: String,
        ignoring: String? = nil,
        mods: NSEvent.ModifierFlags = [],
        type: NSEvent.EventType = .keyDown
    ) throws -> NSEvent {
        try XCTUnwrap(NSEvent.keyEvent(
            with: type,
            location: .zero,
            modifierFlags: mods,
            timestamp: ProcessInfo.processInfo.systemUptime,
            windowNumber: 0,
            context: nil,
            characters: characters,
            charactersIgnoringModifiers: ignoring ?? characters,
            isARepeat: false,
            keyCode: keyCode
        ))
    }

    // MARK: Pure translation

    func testControlIsNeverAConsumedModifier() throws {
        try requireUSLayout()
        let ctrlM = try key(Self.kVKM, "\r", ignoring: "m", mods: .control)
        let event = ctrlM.ghosttyKeyEvent(GHOSTTY_ACTION_PRESS)
        XCTAssertEqual(event.keycode, UInt32(Self.kVKM))
        XCTAssertEqual(event.mods.rawValue, GHOSTTY_MODS_CTRL.rawValue)
        XCTAssertEqual(event.consumed_mods.rawValue, GHOSTTY_MODS_NONE.rawValue)
        XCTAssertEqual(event.unshifted_codepoint, UInt32(("m" as Unicode.Scalar).value))
        // The control character is replaced by its key; Ghostty applies control.
        XCTAssertEqual(ctrlM.ghosttyCharacters, "m")
    }

    func testShiftIsConsumedAndCommandIsNot() throws {
        try requireUSLayout()
        let shiftEnter = try key(Self.kVKReturn, "\r", mods: .shift)
        let event = shiftEnter.ghosttyKeyEvent(GHOSTTY_ACTION_PRESS)
        XCTAssertEqual(event.mods.rawValue, GHOSTTY_MODS_SHIFT.rawValue)
        XCTAssertEqual(event.consumed_mods.rawValue, GHOSTTY_MODS_SHIFT.rawValue)
        XCTAssertEqual(event.unshifted_codepoint, 0x0D)

        let cmdShiftE = try key(Self.kVKE, "E", mods: [.command, .shift])
        let cmd = cmdShiftE.ghosttyKeyEvent(GHOSTTY_ACTION_PRESS)
        XCTAssertEqual(cmd.mods.rawValue, GHOSTTY_MODS_SUPER.rawValue | GHOSTTY_MODS_SHIFT.rawValue)
        XCTAssertEqual(cmd.consumed_mods.rawValue, GHOSTTY_MODS_SHIFT.rawValue)
        XCTAssertEqual(cmd.unshifted_codepoint, UInt32(("e" as Unicode.Scalar).value))
    }

    func testTranslationModsOverrideConsumedMods() throws {
        // Option-as-alt: the surface says option did not translate.
        let optE = try key(Self.kVKE, "´", mods: .option)
        let event = optE.ghosttyKeyEvent(GHOSTTY_ACTION_PRESS, translationMods: [])
        XCTAssertEqual(event.mods.rawValue, GHOSTTY_MODS_ALT.rawValue)
        XCTAssertEqual(event.consumed_mods.rawValue, GHOSTTY_MODS_NONE.rawValue)
    }

    func testFunctionKeyPrivateUseCharactersAreWithheld() throws {
        let up = try key(0x7E, "\u{F700}")
        XCTAssertNil(up.ghosttyCharacters)
        let accented = try key(Self.kVKE, "é")
        XCTAssertEqual(accented.ghosttyCharacters, "é")
    }

    func testSidedModifiersAndCapsLock() {
        let rightShift = NSEvent.ModifierFlags(rawValue: NSEvent.ModifierFlags.shift.rawValue | UInt(NX_DEVICERSHIFTKEYMASK))
        XCTAssertEqual(
            Ghostty.ghosttyMods(rightShift).rawValue,
            GHOSTTY_MODS_SHIFT.rawValue | GHOSTTY_MODS_SHIFT_RIGHT.rawValue)
        XCTAssertEqual(Ghostty.ghosttyMods(.capsLock).rawValue, GHOSTTY_MODS_CAPS.rawValue)
        XCTAssertEqual(Ghostty.eventModifierFlags(mods: Ghostty.ghosttyMods([.control, .option])), [.control, .option])
    }

    func testFlagsChangedPressAndRelease() {
        let leftShiftDown = NSEvent.ModifierFlags(rawValue: NSEvent.ModifierFlags.shift.rawValue | UInt(NX_DEVICELSHIFTKEYMASK))
        XCTAssertEqual(Ghostty.modifierKeyAction(keyCode: 0x38, flags: leftShiftDown), GHOSTTY_ACTION_PRESS)
        XCTAssertEqual(Ghostty.modifierKeyAction(keyCode: 0x38, flags: []), GHOSTTY_ACTION_RELEASE)
        // Right shift released while left is still held: shift is set, but not
        // the right side, so it is the right side's release.
        XCTAssertEqual(Ghostty.modifierKeyAction(keyCode: 0x3C, flags: leftShiftDown), GHOSTTY_ACTION_RELEASE)
        let rightCtrl = NSEvent.ModifierFlags(rawValue: NSEvent.ModifierFlags.control.rawValue | UInt(NX_DEVICERCTLKEYMASK))
        XCTAssertEqual(Ghostty.modifierKeyAction(keyCode: 0x3E, flags: rightCtrl), GHOSTTY_ACTION_PRESS)
        XCTAssertEqual(Ghostty.modifierKeyAction(keyCode: 0x39, flags: .capsLock), GHOSTTY_ACTION_PRESS)
        XCTAssertNil(Ghostty.modifierKeyAction(keyCode: Self.kVKE, flags: []))
    }

    func testMouseButtonsScrollModsAndCursors() {
        XCTAssertEqual(Ghostty.mouseButton(fromNSEventButtonNumber: 0), GHOSTTY_MOUSE_LEFT)
        XCTAssertEqual(Ghostty.mouseButton(fromNSEventButtonNumber: 2), GHOSTTY_MOUSE_MIDDLE)
        XCTAssertEqual(Ghostty.mouseButton(fromNSEventButtonNumber: 3), GHOSTTY_MOUSE_EIGHT)
        XCTAssertEqual(Ghostty.mouseButton(fromNSEventButtonNumber: 42), GHOSTTY_MOUSE_UNKNOWN)

        XCTAssertEqual(Ghostty.scrollMods(precision: false, momentum: []), 0)
        XCTAssertEqual(Ghostty.scrollMods(precision: true, momentum: []), 1)
        XCTAssertEqual(
            Ghostty.scrollMods(precision: true, momentum: .changed),
            1 | Int32(GHOSTTY_MOUSE_MOMENTUM_CHANGED.rawValue) << 1)

        XCTAssertEqual(Ghostty.cursor(for: GHOSTTY_MOUSE_SHAPE_TEXT), .iBeam)
        XCTAssertEqual(Ghostty.cursor(for: GHOSTTY_MOUSE_SHAPE_POINTER), .pointingHand)
        XCTAssertEqual(Ghostty.cursor(for: GHOSTTY_MOUSE_SHAPE_DEFAULT), .arrow)
    }

    // MARK: Bytes on the pty

    private var ghostty: Ghostty.App!
    private var window: NSWindow!
    private var view: Ghostty.SurfaceView!
    private var scriptURL: URL!
    private var configURL: URL?

    /// Starts a surface whose program enables kitty disambiguation, puts its
    /// tty in raw mode, and prints every byte it reads as `xx.`.
    private func startByteDump(optionAsAlt: Bool? = nil) throws {
        let script = """
        #!/bin/sh
        printf '\\033[>1u'
        stty raw -echo
        printf 'READY:'
        while :; do
          b=$(dd bs=1 count=1 2>/dev/null | od -An -tx1 | tr -d ' \\n')
          printf '%s.' "$b"
        done
        """
        scriptURL = FileManager.default.temporaryDirectory
            .appendingPathComponent("muse-keydump-\(UUID().uuidString).sh")
        try script.write(to: scriptURL, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: scriptURL.path)

        let overrides = Ghostty.ConfigOverrides(command: scriptURL)
        if let optionAsAlt {
            let url = scriptURL.appendingPathExtension("conf")
            try "macos-option-as-alt = \(optionAsAlt)\n".write(to: url, atomically: true, encoding: .utf8)
            configURL = url
        }
        let userConfig = configURL
        ghostty = Ghostty.App { Ghostty.Config(userConfig: userConfig, overrides: overrides) }
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
        window.makeFirstResponder(view)
        view.focusDidChange(true)
        try waitForScreen { $0.contains("READY:") }
    }

    override func tearDownWithError() throws {
        window?.contentView = nil
        view = nil
        window?.close()
        window = nil
        ghostty = nil
        if let scriptURL { try? FileManager.default.removeItem(at: scriptURL) }
        if let configURL { try? FileManager.default.removeItem(at: configURL) }
        configURL = nil
    }

    private func screenText() -> String {
        guard let surface = view.surface else { return "" }
        var text = ghostty_text_s()
        let selection = ghostty_selection_s(
            top_left: ghostty_point_s(tag: GHOSTTY_POINT_SCREEN, coord: GHOSTTY_POINT_COORD_TOP_LEFT, x: 0, y: 0),
            bottom_right: ghostty_point_s(tag: GHOSTTY_POINT_SCREEN, coord: GHOSTTY_POINT_COORD_BOTTOM_RIGHT, x: 0, y: 0),
            rectangle: false)
        guard ghostty_surface_read_text(surface, selection, &text) else { return "" }
        defer { ghostty_surface_free_text(surface, &text) }
        return String(cString: text.text).replacingOccurrences(of: "\n", with: "")
    }

    private func waitForScreen(timeout: TimeInterval = 10, _ done: (String) -> Bool) throws {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            ghostty.appTick()
            if done(screenText()) { return }
            RunLoop.current.run(until: Date().addingTimeInterval(0.02))
        }
        XCTFail("screen never matched; it reads: \(screenText())")
    }

    /// The bytes the program printed after `READY:`, as `xx.` pairs.
    private func dumped() -> String {
        let screen = screenText()
        guard let range = screen.range(of: "READY:") else { return "" }
        return String(screen[range.upperBound...]).trimmingCharacters(in: .whitespaces)
    }

    private func press(_ event: NSEvent, expecting bytes: String) throws {
        let before = dumped()
        view.keyDown(with: event)
        try waitForScreen { _ in dumped().count >= before.count + bytes.count }
        XCTAssertEqual(String(dumped().dropFirst(before.count)), bytes)
    }

    func testKittyOnlyChordsReachTheProgramDistinctly() throws {
        try requireUSLayout()
        try startByteDump()

        // Enter and Tab stay legacy bytes; their aliases become CSI u.
        try press(key(Self.kVKReturn, "\r"), expecting: "0d.")
        try press(key(Self.kVKM, "\r", ignoring: "m", mods: .control), expecting: "1b.5b.31.30.39.3b.35.75.")
        try press(key(Self.kVKTab, "\t"), expecting: "09.")
        try press(key(Self.kVKI, "\t", ignoring: "i", mods: .control), expecting: "1b.5b.31.30.35.3b.35.75.")
        try press(key(Self.kVKReturn, "\r", mods: .shift), expecting: "1b.5b.31.33.3b.32.75.")
        try press(key(Self.kVKJ, "\n", ignoring: "j", mods: .control), expecting: "1b.5b.31.30.36.3b.35.75.")
    }

    /// muse's own bindings: shift+enter enqueue-replace, ctrl+, settings,
    /// shift+left/right seek, and ctrl+c quit.
    func testMuseChordsReachTheProgramDistinctly() throws {
        try requireUSLayout()
        try startByteDump()

        try press(key(Self.kVKReturn, "\r", mods: .shift), expecting: "1b.5b.31.33.3b.32.75.")
        // ctrl+, has no legacy encoding; only CSI u carries it.
        try press(key(Self.kVKComma, ",", mods: .control), expecting: "1b.5b.34.34.3b.35.75.")
        let left = String(UnicodeScalar(UInt16(NSLeftArrowFunctionKey))!)
        let right = String(UnicodeScalar(UInt16(NSRightArrowFunctionKey))!)
        try press(key(Self.kVKLeftArrow, left, mods: [.shift, .function, .numericPad]), expecting: "1b.5b.31.3b.32.44.")
        try press(key(Self.kVKRightArrow, right, mods: [.shift, .function, .numericPad]), expecting: "1b.5b.31.3b.32.43.")
        try press(key(Self.kVKC, "\u{03}", ignoring: "c", mods: .control), expecting: "1b.5b.39.39.3b.35.75.")
    }

    func testComposedTextIsSentAsUTF8() throws {
        try requireUSLayout()
        try startByteDump(optionAsAlt: true)
        // A layout key producing an accented character sends its UTF-8.
        try press(key(Self.kVKE, "é"), expecting: "c3.a9.")
        try press(key(0x08, "ç", ignoring: "c"), expecting: "c3.a7.")
        // Explicit option-as-alt must send alt+c. Do not depend on the host's
        // layout detection to choose a default for an embedded Ghostty surface.
        try press(key(0x08, "ç", ignoring: "c", mods: .option), expecting: "1b.5b.39.39.3b.33.75.")
    }

    func testOptionCanProduceComposedTextWhenAltIsDisabled() throws {
        try requireUSLayout()
        try startByteDump(optionAsAlt: false)
        try press(key(0x08, "ç", ignoring: "c", mods: .option), expecting: "c3.a7.")
    }

    func testPreeditIsNotSentUntilCommitted() throws {
        try startByteDump()
        let before = dumped()
        view.setMarkedText("^", selectedRange: NSRange(location: 1, length: 0), replacementRange: NSRange(location: NSNotFound, length: 0))
        XCTAssertTrue(view.hasMarkedText())
        XCTAssertEqual(view.markedRange(), NSRange(location: 0, length: 1))
        view.unmarkText()
        XCTAssertFalse(view.hasMarkedText())
        for _ in 0..<10 {
            ghostty.appTick()
            RunLoop.current.run(until: Date().addingTimeInterval(0.02))
        }
        XCTAssertEqual(dumped(), before)
    }

    // MARK: Menu shortcuts

    private final class MenuRecorder: NSObject {
        var fired: [String] = []
        @objc func preferences(_ sender: Any?) { fired.append("preferences") }
    }

    func testMenuShortcutWinsOverTheConsumedBindingItShares() throws {
        try requireUSLayout()
        try startByteDump()
        let recorder = MenuRecorder()
        let appMenu = NSMenu()
        let preferences = appMenu.addItem(withTitle: "Preferences…", action: #selector(MenuRecorder.preferences(_:)), keyEquivalent: ",")
        preferences.target = recorder
        let main = NSMenu()
        let appItem = NSMenuItem()
        appItem.submenu = appMenu
        main.addItem(appItem)
        let previousMenu = NSApp.mainMenu
        NSApp.mainMenu = main
        defer { NSApp.mainMenu = previousMenu }

        // Cmd+, is Ghostty's open_config; with the terminal focused the
        // Preferences item takes it and the surface never sees it.
        let before = dumped()
        XCTAssertTrue(view.performKeyEquivalent(with: try key(0x2B, ",", mods: .command)))
        XCTAssertEqual(recorder.fired, ["preferences"])

        // A binding no item owns stays the surface's.
        XCTAssertNil(MainMenu.item(in: main, matching: try key(0x2B, "<", mods: [.command, .shift])))

        // A Ctrl chord is no binding, so the menu is never asked and it
        // still reaches the program.
        XCTAssertFalse(view.performKeyEquivalent(with: try key(Self.kVKM, "\r", ignoring: "m", mods: .control)))
        XCTAssertEqual(recorder.fired, ["preferences"])
        XCTAssertEqual(dumped(), before)
    }

    // MARK: Mouse

    private func mouse(_ type: NSEvent.EventType, x: CGFloat, fromTop y: CGFloat) throws -> NSEvent {
        try XCTUnwrap(NSEvent.mouseEvent(
            with: type,
            location: NSPoint(x: x, y: view.frame.height - y),
            modifierFlags: [],
            timestamp: ProcessInfo.processInfo.systemUptime,
            windowNumber: window.windowNumber,
            context: nil,
            eventNumber: 0,
            clickCount: 1,
            pressure: 1
        ))
    }

    func testDraggingSelectsTheTextUnderThePointer() throws {
        try startByteDump()
        let surface = try XCTUnwrap(view.surface)
        XCTAssertFalse(ghostty_surface_has_selection(surface))

        view.mouseMoved(with: try mouse(.mouseMoved, x: 1, fromTop: 5))
        view.mouseDown(with: try mouse(.leftMouseDown, x: 1, fromTop: 5))
        view.mouseDragged(with: try mouse(.leftMouseDragged, x: 300, fromTop: 5))
        view.mouseUp(with: try mouse(.leftMouseUp, x: 300, fromTop: 5))

        XCTAssertTrue(ghostty_surface_has_selection(surface))
        var text = ghostty_text_s()
        XCTAssertTrue(ghostty_surface_read_selection(surface, &text))
        defer { ghostty_surface_free_text(surface, &text) }
        XCTAssertTrue(String(cString: text.text).hasPrefix("READY:"))

        // Copy is enabled exactly when there is a selection to copy.
        let copy = NSMenuItem(title: "Copy", action: #selector(Ghostty.SurfaceView.copy(_:)), keyEquivalent: "c")
        XCTAssertTrue(view.validateMenuItem(copy))
    }
}
