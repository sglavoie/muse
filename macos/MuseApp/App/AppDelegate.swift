import AppKit
import GhosttyKit

final class AppDelegate: NSObject, NSApplicationDelegate, GhosttyAppDelegate {
    private(set) var window: NSWindow?
    private(set) var ghostty: Ghostty.App?
    private(set) var surfaceView: Ghostty.SurfaceView?

    /// A hosted test bundle launches this app too; it must not open a window.
    private static var isRunningTests: Bool {
        ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil
    }

    func applicationWillFinishLaunching(_ notification: Notification) {
        // A regular app with its own Dock icon.
        NSApp.setActivationPolicy(.regular)
        NSApp.mainMenu = MainMenu.make()
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Ghostty's global state must be initialized once per process before any
        // app or surface exists.
        if ghostty_init(UInt(CommandLine.argc), CommandLine.unsafeArgv) != GHOSTTY_SUCCESS {
            NSLog("muse: ghostty_init failed")
            return
        }
        guard !Self.isRunningTests else { return }
        startTerminal()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    // MARK: Terminal

    /// Runs `muse` from the login shell's PATH, so a Finder launch finds it and
    /// mpv. Session 5 replaces this with MuseBinaryResolver and
    /// LaunchEnvironment.
    private func startTerminal() {
        var environment: [String: String] = [:]
        if let path = Self.loginShellPath() {
            environment["PATH"] = path
        }
        let overrides = Ghostty.ConfigOverrides(
            command: URL(fileURLWithPath: "/usr/bin/env"),
            arguments: ["muse"],
            environment: environment,
            waitAfterCommand: true
        )
        let ghostty = Ghostty.App { Ghostty.Config(overrides: overrides) }
        guard ghostty.readiness == .ready, let app = ghostty.app else {
            NSLog("muse: Ghostty failed to start; see the log for config diagnostics")
            return
        }
        ghostty.delegate = self
        self.ghostty = ghostty

        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 960, height: 640),
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered,
            defer: false
        )
        window.title = "muse"
        window.isReleasedWhenClosed = false
        window.tabbingMode = .disallowed
        window.titlebarAppearsTransparent = true
        self.window = window
        if let background = ghostty.config.backgroundColor {
            setTerminalBackground(background)
        }

        let surfaceView = Ghostty.SurfaceView(app: app)
        surfaceView.onTitleChange = { [weak window] title in
            window?.title = title.isEmpty ? "muse" : title
        }
        window.contentView = surfaceView
        window.initialFirstResponder = surfaceView
        self.surfaceView = surfaceView

        window.center()
        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(surfaceView)
        NSApp.activate(ignoringOtherApps: true)
    }

    /// The title bar takes the terminal's background, with the appearance
    /// whose title text reads on it.
    private func setTerminalBackground(_ color: NSColor) {
        guard let window else { return }
        window.backgroundColor = color
        let rgb = color.usingColorSpace(.sRGB)
        let luminance = rgb.map { 0.2126 * $0.redComponent + 0.7152 * $0.greenComponent + 0.0722 * $0.blueComponent } ?? 0
        window.appearance = NSAppearance(named: luminance < 0.5 ? .darkAqua : .aqua)
    }

    /// The login shell's PATH, or nil when the shell fails or takes too long.
    private static func loginShellPath() -> String? {
        let shell = ProcessInfo.processInfo.environment["SHELL"] ?? "/bin/zsh"
        let process = Process()
        process.executableURL = URL(fileURLWithPath: shell)
        process.arguments = ["-l", "-c", "printf %s \"$PATH\""]
        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = FileHandle.nullDevice
        process.standardInput = FileHandle.nullDevice
        let done = DispatchSemaphore(value: 0)
        process.terminationHandler = { _ in done.signal() }
        do {
            try process.run()
        } catch {
            return nil
        }
        // A PATH fits the pipe's buffer, so waiting before reading cannot stall.
        guard done.wait(timeout: .now() + 5) == .success, process.terminationStatus == 0 else {
            process.terminate()
            return nil
        }
        let output = pipe.fileHandleForReading.readDataToEndOfFile()
        let path = String(decoding: output, as: UTF8.self)
        return path.isEmpty ? nil : path
    }

    // MARK: GhosttyAppDelegate

    func ghosttyApp(_ app: Ghostty.App, perform action: ghostty_action_s, target: ghostty_target_s) -> Bool {
        switch action.tag {
        case GHOSTTY_ACTION_SET_TITLE:
            guard let ptr = action.action.set_title.title else { return false }
            let title = String(cString: ptr)
            // The callback may run off the main thread; AppKit may not.
            DispatchQueue.main.async { [weak self] in self?.surfaceView?.setTitle(title) }
            return true
        case GHOSTTY_ACTION_MOUSE_SHAPE:
            let shape = action.action.mouse_shape
            onMain { [weak self] in self?.surfaceView?.setCursorShape(shape) }
            return true
        case GHOSTTY_ACTION_MOUSE_VISIBILITY:
            let visible = action.action.mouse_visibility == GHOSTTY_MOUSE_VISIBLE
            onMain { [weak self] in self?.surfaceView?.setCursorVisibility(visible) }
            return true
        case GHOSTTY_ACTION_COLOR_CHANGE:
            // muse announces its theme's background (OSC 11); the title bar
            // follows it so the window reads as one surface.
            let change = action.action.color_change
            guard change.kind == GHOSTTY_ACTION_COLOR_KIND_BACKGROUND else { return true }
            let color = Ghostty.color(ghostty_config_color_s(r: change.r, g: change.g, b: change.b))
            onMain { [weak self] in self?.setTerminalBackground(color) }
            return true
        case GHOSTTY_ACTION_RENDERER_HEALTH:
            if action.action.renderer_health != GHOSTTY_RENDERER_HEALTH_HEALTHY {
                NSLog("muse: renderer reported unhealthy")
            }
            return true
        case GHOSTTY_ACTION_CLOSE_WINDOW:
            onMain { [weak self] in self?.window?.performClose(nil) }
            return true
        default:
            // Unhandled here, including SHOW_CHILD_EXITED, so Ghostty writes
            // its own exit notice into the terminal.
            return false
        }
    }

    /// Actions arrive from `ghostty_app_tick` on the main thread; anything
    /// else is hopped there, since AppKit is main-thread only.
    private func onMain(_ work: @escaping () -> Void) {
        if Thread.isMainThread {
            work()
        } else {
            DispatchQueue.main.async(execute: work)
        }
    }

    func ghosttyApp(_ app: Ghostty.App, closeSurface host: GhosttySurfaceHost?, needsConfirm: Bool) {
        onMain { [weak self] in
            guard let self, host == nil || host === self.surfaceView else { return }
            self.window?.close()
        }
    }
}
