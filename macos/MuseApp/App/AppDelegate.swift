import AppKit
import GhosttyKit

final class AppDelegate: NSObject, NSApplicationDelegate, GhosttyAppDelegate {
    /// How long Quit waits for muse to drain before SIGKILLing its process
    /// group: muse's own shutdown is bounded well inside this.
    static let quitDrainTimeout: TimeInterval = 12

    /// The launch-error window, when muse could not start.
    private var window: NSWindow?
    private(set) var ghostty: Ghostty.App?
    private(set) var terminal: TerminalWindowController?
    /// The running muse's control socket, for the quit path; nil when the muse
    /// is too old to serve one.
    private(set) var quitLink: MuseQuitLink?
    /// What `build-info` said of the muse running, for About.
    private var museVersion: MuseVersion?

    /// A hosted test bundle launches this app too; it must not spawn muse.
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
            showLaunchError(title: "Ghostty failed to initialize.", message: "ghostty_init returned an error.")
            return
        }
        guard !Self.isRunningTests else { return }

        let environment = LaunchEnvironment.current().applying(MuseEnvironmentOverrides.load())
        let binary: MuseBinaryResolver.Resolution
        switch MuseBinaryResolver.live(environment: environment).resolve() {
        case .success(let resolution):
            binary = resolution
        case .failure(let notFound):
            NSLog("muse: %@", notFound.message)
            showLaunchError(title: "muse could not be found.", message: notFound.message)
            return
        }
        NSLog("muse: running %@ (%@)", binary.url.path, String(describing: binary.source))
        museVersion = MuseVersion.read(binary: binary.url.path)

        guard let launcher = Bundle.main.url(forResource: "muse-launch", withExtension: nil) else {
            showLaunchError(title: "muse.app is incomplete.", message: "Its muse-launch helper is missing from Resources.")
            return
        }
        let statusFile = FileManager.default.temporaryDirectory
            .appendingPathComponent("muse-app-\(ProcessInfo.processInfo.processIdentifier).status")
        // Asked once: the binary is chosen at launch and fixed until muse.app restarts.
        let controlPath = MuseVersion.supports(flag: MuseControlLaunch.flag, binary: binary.url.path)
            ? MuseControlLaunch.socketPath()
            : nil
        if controlPath == nil {
            NSLog("muse: %@ does not accept %@; Quit waits on its exit status instead",
                  binary.url.path, MuseControlLaunch.flag)
        }
        let commandEnvironment = environment.commandEnvironment
        let overrides = Ghostty.ConfigOverrides(
            command: launcher,
            arguments: [statusFile.path, binary.url.path] + MuseControlLaunch.arguments(controlPath: controlPath),
            environment: commandEnvironment,
            waitAfterCommand: true
        )
        let ghostty = Ghostty.App { Ghostty.Config(overrides: overrides) }
        guard ghostty.readiness == .ready else {
            showLaunchError(
                title: "Ghostty failed to start.",
                message: "The terminal engine could not be created; see the log for config diagnostics."
            )
            return
        }
        ghostty.delegate = self
        self.ghostty = ghostty

        if let controlPath {
            quitLink = MuseQuitLink(path: controlPath)
        }

        // The lock a terminal muse holds sits where this muse would look,
        // which the forwarded XDG_RUNTIME_DIR (or the shared TMPDIR) decides.
        let lockEnvironment = ProcessInfo.processInfo.environment.merging(commandEnvironment) { _, forwarded in forwarded }
        let controller = TerminalWindowController(
            ghostty: ghostty,
            statusFile: statusFile,
            holderPID: { MuseInstanceLock.holderPID(environment: lockEnvironment) }
        )
        controller.onWillStartSession = { [weak self] in self?.quitLink?.connect() }
        controller.onSessionEnd = { [weak self] in self?.quitLink?.disconnect() }
        terminal = controller
        controller.show()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        // Closing the window hides it, and the music plays on; only Quit ends muse.app.
        false
    }

    /// Quit drains muse first, so the listen in flight is recorded and mpv
    /// stops with it; the app terminates once that is done.
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard let terminal, terminal.surfaceView != nil else { return .terminateNow }
        guard !terminal.isDraining else { return .terminateLater }
        terminal.drainForQuit(link: quitLink, timeout: Self.quitDrainTimeout) {
            NSApp.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }

    /// A Dock click shows the window, starting a fresh muse if the last one exited.
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        terminal?.show()
        return false
    }

    /// Switching to muse.app (Cmd+Tab, the Dock) brings its window back.
    func applicationDidBecomeActive(_ notification: Notification) {
        guard let terminal, !terminal.window.isVisible, !terminal.isDraining else { return }
        terminal.show()
    }

    func applicationWillTerminate(_ notification: Notification) {
        quitLink?.disconnect()
        terminal?.teardown()
        // muse removes its socket on a clean exit; a killed one leaves it.
        if let path = quitLink?.path {
            try? FileManager.default.removeItem(atPath: path)
        }
    }

    /// Right-clicking the Dock icon.
    func applicationDockMenu(_ sender: NSApplication) -> NSMenu? {
        let menu = NSMenu()
        let relaunch = menu.addItem(withTitle: "Relaunch muse", action: #selector(relaunchFromMenu(_:)), keyEquivalent: "")
        relaunch.target = self
        return menu
    }

    /// Preferences arrive with their window; until then the item still owns
    /// Cmd+, so the shortcut never falls through to Ghostty's config.
    @objc func showPreferences(_ sender: Any?) {
        NSSound.beep()
    }

    @objc func showReadme(_ sender: Any?) {
        NSWorkspace.shared.open(MainMenu.readmeURL)
    }

    @objc func relaunchFromMenu(_ sender: Any?) {
        terminal?.relaunch()
    }

    @objc func showAbout(_ sender: Any?) {
        let notice = Bundle.main.url(forResource: "NOTICE-ghostty", withExtension: nil)
            .flatMap { try? String(contentsOf: $0, encoding: .utf8) }
        NSApp.activate(ignoringOtherApps: true)
        NSApp.orderFrontStandardAboutPanel(options: [
            .credits: MainMenu.aboutCredits(
                museVersion: museVersion?.version, ghosttyVersion: Ghostty.versionDescription, notice: notice
            ),
        ])
    }

    private func showLaunchError(title: String, message: String) {
        let window = LaunchErrorWindow.make(title: title, message: message)
        window.makeKeyAndOrderFront(nil)
        self.window = window
        NSApp.activate(ignoringOtherApps: true)
    }

    // MARK: GhosttyAppDelegate

    func ghosttyApp(_ app: Ghostty.App, perform action: ghostty_action_s, target: ghostty_target_s) -> Bool {
        switch action.tag {
        case GHOSTTY_ACTION_SET_TITLE:
            guard let ptr = action.action.set_title.title else { return false }
            let title = String(cString: ptr)
            // The callback may run off the main thread; AppKit may not.
            DispatchQueue.main.async { [weak self] in self?.terminal?.surfaceView?.setTitle(title) }
            return true
        case GHOSTTY_ACTION_MOUSE_SHAPE:
            let shape = action.action.mouse_shape
            onMain { [weak self] in self?.terminal?.surfaceView?.setCursorShape(shape) }
            return true
        case GHOSTTY_ACTION_MOUSE_VISIBILITY:
            let visible = action.action.mouse_visibility == GHOSTTY_MOUSE_VISIBLE
            onMain { [weak self] in self?.terminal?.surfaceView?.setCursorVisibility(visible) }
            return true
        case GHOSTTY_ACTION_COLOR_CHANGE:
            // muse announces its theme's background (OSC 11); the title bar
            // follows it so the window reads as one surface.
            let change = action.action.color_change
            onMain { [weak self] in self?.terminal?.colorChanged(change) }
            return true
        case GHOSTTY_ACTION_RENDERER_HEALTH:
            if action.action.renderer_health != GHOSTTY_RENDERER_HEALTH_HEALTHY {
                NSLog("muse: renderer reported unhealthy")
            }
            return true
        case GHOSTTY_ACTION_CLOSE_WINDOW:
            onMain { [weak self] in self?.terminal?.hide() }
            return true
        case GHOSTTY_ACTION_SHOW_CHILD_EXITED:
            // Handled even with the window hidden: returning true tells Ghostty
            // the host shows the exit, so it writes nothing into the terminal.
            let surface = target.tag == GHOSTTY_TARGET_SURFACE ? target.target.surface : nil
            onMain { [weak self] in self?.terminal?.childExited(surface: surface) }
            return true
        default:
            // The surface view owns the rest once it exists.
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
        onMain { [weak self] in self?.terminal?.closeRequested(host: host) }
    }
}
