import AppKit
import GhosttyKit

/// The one window and its one muse session. Closing the window hides it and
/// keeps muse running; a clean muse exit hides it too and leaves the next show to
/// start a fresh muse, or quits when configured; a failed exit keeps the last
/// output on screen under a Relaunch banner. Quitting the app drains muse
/// first, so its last listen is recorded and mpv goes with it.
final class TerminalWindowController: NSObject, NSWindowDelegate {
    static let frameAutosaveName = "muse.main"

    let window: NSWindow
    private let ghostty: Ghostty.App
    private let statusFile: URL
    private let exitBehavior: () -> MuseExitBehavior
    private let holderPID: () -> Int32?
    private let hidesApp: Bool
    private let container = NSStackView()
    private var failureBanner: NSView?
    /// While the banner shows, Return relaunches even with the dead surface
    /// focused (it stays focusable so its output can be selected and copied).
    private var failureKeyMonitor: Any?

    /// The live session's surface; nil between a clean exit and the next show.
    private(set) var surfaceView: Ghostty.SurfaceView?
    /// Called just before a new muse starts, and after a session is gone.
    var onWillStartSession: (() -> Void)?
    var onSessionEnd: (() -> Void)?
    /// Called on the main thread with each title the running muse sets.
    var onTitleChange: ((String) -> Void)?
    /// Set once muse has exited on the current surface, so a later close request
    /// from that dead surface decides nothing.
    private(set) var exitAction: MuseExitAction?
    /// Set while the app's quit drains the session, so its exit decides nothing.
    private(set) var isDraining = false
    private var drainTimer: Timer?

    /// Tests pass no autosave name and `hidesApp: false`, so they neither
    /// touch the saved frame nor hide their host app.
    init(
        ghostty: Ghostty.App,
        statusFile: URL,
        exitBehavior: @escaping () -> MuseExitBehavior = { .load() },
        holderPID: @escaping () -> Int32? = { nil },
        frameAutosaveName: String? = TerminalWindowController.frameAutosaveName,
        hidesApp: Bool = true
    ) {
        self.ghostty = ghostty
        self.statusFile = statusFile
        self.exitBehavior = exitBehavior
        self.holderPID = holderPID
        self.hidesApp = hidesApp
        window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 960, height: 640),
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered,
            defer: false
        )
        super.init()
        window.title = "muse"
        window.isReleasedWhenClosed = false
        window.tabbingMode = .disallowed
        window.delegate = self

        // The title bar shows the window's background, which follows the
        // terminal's; muse announces its theme's, so the window reads as one surface.
        window.titlebarAppearsTransparent = true
        if let background = ghostty.config.backgroundColor {
            setTerminalBackground(background)
        }

        container.orientation = .vertical
        container.spacing = 0
        container.distribution = .fill
        window.contentView = container

        if let frameAutosaveName {
            if !window.setFrameUsingName(frameAutosaveName) {
                window.center()
            }
            window.setFrameAutosaveName(frameAutosaveName)
        } else {
            window.center()
        }
    }

    // MARK: Showing and hiding

    /// Brings the window forward, first starting muse if no session is live.
    func show() {
        if surfaceView == nil {
            startSession()
        }
        NSApp.activate(ignoringOtherApps: true)
        window.makeKeyAndOrderFront(nil)
        if let surfaceView {
            window.makeFirstResponder(surfaceView)
        }
    }

    /// Orders the window out and hides the app, so focus returns to whatever
    /// was in front before; the session, if any, keeps running.
    func hide() {
        window.orderOut(nil)
        if hidesApp {
            NSApp.hide(nil)
        }
    }

    /// Replaces whatever the window holds with a fresh muse and shows it.
    func relaunch() {
        endSession()
        show()
    }

    /// Ends the session for good: freeing the surface sends muse SIGHUP, as a
    /// closed terminal would.
    func teardown() {
        endSession()
    }

    // MARK: Background

    /// The terminal's default background changed: muse set its theme's (OSC 11),
    /// or reset it on exit. The foreground and cursor colors matter to nothing
    /// outside the terminal.
    func colorChanged(_ change: ghostty_action_color_change_s) {
        guard change.kind == GHOSTTY_ACTION_COLOR_KIND_BACKGROUND else { return }
        setTerminalBackground(Ghostty.color(ghostty_config_color_s(r: change.r, g: change.g, b: change.b)))
    }

    /// The title bar takes the background, with the appearance whose title
    /// text reads on it.
    func setTerminalBackground(_ color: NSColor) {
        window.backgroundColor = color
        window.appearance = NSAppearance(named: Self.isDark(color) ? .darkAqua : .aqua)
    }

    static func isDark(_ color: NSColor) -> Bool {
        guard let rgb = color.usingColorSpace(.sRGB) else { return true }
        let luminance = 0.2126 * rgb.redComponent + 0.7152 * rgb.greenComponent + 0.0722 * rgb.blueComponent
        return luminance < 0.5
    }

    // MARK: Session

    private func startSession() {
        guard let app = ghostty.app else { return }
        onWillStartSession?()
        removeLaunchFiles()
        exitAction = nil
        window.title = "muse"

        let surfaceView = Ghostty.SurfaceView(app: app)
        surfaceView.onTitleChange = { [weak self, weak window] title in
            window?.title = title.isEmpty ? "muse" : title
            self?.onTitleChange?(title)
        }
        surfaceView.setContentHuggingPriority(.defaultLow, for: .vertical)
        surfaceView.setContentCompressionResistancePriority(.defaultLow, for: .vertical)
        container.addArrangedSubview(surfaceView)
        window.initialFirstResponder = surfaceView
        self.surfaceView = surfaceView
    }

    private func endSession() {
        removeFailureBanner()
        if let surfaceView {
            container.removeArrangedSubview(surfaceView)
            surfaceView.removeFromSuperview()
        }
        // The view's deinit frees the surface, which stops muse.
        let hadSession = surfaceView != nil
        surfaceView = nil
        if hadSession {
            onSessionEnd?()
        }
        exitAction = nil
        isDraining = false
        drainTimer?.invalidate()
        drainTimer = nil
        removeLaunchFiles()
    }

    private func removeLaunchFiles() {
        try? FileManager.default.removeItem(at: statusFile)
        try? FileManager.default.removeItem(at: Self.launcherPIDFile(for: statusFile))
    }

    private func isCurrent(_ surface: ghostty_surface_t?) -> Bool {
        guard let surface, let current = surfaceView?.surface else { return false }
        return surface == current
    }

    /// muse exited on `surface`. Ghostty reports this from inside its own tick,
    /// with the surface mid-message, so nothing is freed until that returns.
    func childExited(surface: ghostty_surface_t?) {
        guard isCurrent(surface), exitAction == nil, !isDraining else { return }
        let action = MuseExitAction.action(
            for: MuseExitStatus(contentsOf: statusFile), behavior: exitBehavior(), holderPID: holderPID
        )
        exitAction = action
        NSLog("muse: exited: %@", String(describing: action))
        DispatchQueue.main.async { [weak self] in self?.perform(action) }
    }

    private func perform(_ action: MuseExitAction) {
        switch action {
        case .hideAndRelaunchOnShow:
            let wasVisible = window.isVisible
            endSession()
            if wasVisible { hide() }
        case .quitApp:
            NSApp.terminate(nil)
        case .alreadyRunning, .showFailure:
            showFailureBanner(action.bannerMessage ?? "")
        }
    }

    /// Ghostty asked to close `host`'s surface: while muse runs that is Cmd+W or
    /// a close binding, which hides; once muse exited it is a key pressed on the
    /// dead surface, and the exit has already decided what happens. The
    /// callback's own flag cannot tell these apart: it says whether closing
    /// needs confirmation, which muse.app's config turns off.
    func closeRequested(host: GhosttySurfaceHost?) {
        guard isCurrent(host?.surface), let surface = host?.surface, !isDraining else { return }
        if !ghostty_surface_process_exited(surface) {
            hide()
        } else if exitAction == nil {
            childExited(surface: surface)
        }
    }

    // MARK: Quit

    /// The launcher's pid, whose process group muse and mpv share.
    static func launcherPIDFile(for statusFile: URL) -> URL {
        URL(fileURLWithPath: statusFile.path + ".pid")
    }

    var launcherPID: pid_t? {
        guard let text = try? String(contentsOf: Self.launcherPIDFile(for: statusFile), encoding: .utf8),
              let pid = pid_t(text.trimmingCharacters(in: .whitespacesAndNewlines)), pid > 0 else { return nil }
        return pid
    }

    /// The process group to signal for `pid`; nil for one that is gone, or
    /// that is this app's own.
    static func processGroup(of pid: pid_t) -> pid_t? {
        let group = getpgid(pid)
        guard group > 1, group != getpgid(0) else { return nil }
        return group
    }

    /// Ends the session for the app's quit without stalling the main thread:
    /// muse's process group gets the SIGHUP freeing the surface would send,
    /// and the surface is freed only once muse is done — its control socket
    /// reached EOF, or the launcher recorded its exit — or `timeout` passed,
    /// when the group is SIGKILLed so mpv cannot outlive it. Freeing a surface
    /// whose command still runs blocks until it exits, hangup after hangup.
    func drainForQuit(link: MuseControlClient?, timeout: TimeInterval, completion: @escaping () -> Void) {
        guard let surface = surfaceView?.surface, !ghostty_surface_process_exited(surface),
              let pid = link?.helloPID ?? launcherPID, let group = Self.processGroup(of: pid) else {
            teardown()
            completion()
            return
        }
        isDraining = true
        killpg(group, SIGHUP)
        NSLog("muse: draining process group %d", group)
        let deadline = Date().addingTimeInterval(timeout)
        let statusFile = self.statusFile
        let finish: (Timer) -> Void = { [weak self] timer in
            timer.invalidate()
            self?.drainTimer = nil
            self?.teardown()
            completion()
        }
        let timer = Timer(timeInterval: 0.05, repeats: true) { timer in
            if link?.reachedEOF == true
                || FileManager.default.fileExists(atPath: statusFile.path)
                || kill(pid, 0) != 0 {
                finish(timer)
            } else if Date() >= deadline {
                NSLog("muse: still running after %.0f s; killing process group %d", timeout, group)
                killpg(group, SIGKILL)
                finish(timer)
            }
        }
        drainTimer = timer
        // Common modes: a quit answered later runs the loop in the modal panel mode.
        RunLoop.main.add(timer, forMode: .common)
    }

    // MARK: Failure banner

    private func showFailureBanner(_ message: String) {
        removeFailureBanner()
        let banner = FailureBanner(message: message, target: self, action: #selector(relaunchFromBanner(_:)))
        container.addArrangedSubview(banner)
        failureBanner = banner
        failureKeyMonitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
            guard let self, event.window === self.window,
                  Self.isRelaunchKey(event.charactersIgnoringModifiers, event.modifierFlags) else { return event }
            self.relaunch()
            return nil
        }
        if !window.isVisible {
            show()
        }
    }

    static func isRelaunchKey(_ characters: String?, _ flags: NSEvent.ModifierFlags) -> Bool {
        let chord = flags.intersection([.command, .control, .option, .shift])
        return chord.isEmpty && (characters == "\r" || characters == "\u{3}")
    }

    private func removeFailureBanner() {
        if let failureKeyMonitor {
            NSEvent.removeMonitor(failureKeyMonitor)
            self.failureKeyMonitor = nil
        }
        guard let failureBanner else { return }
        container.removeArrangedSubview(failureBanner)
        failureBanner.removeFromSuperview()
        self.failureBanner = nil
    }

    var isShowingFailure: Bool { failureBanner != nil }

    @objc private func relaunchFromBanner(_ sender: Any?) {
        relaunch()
    }

    // MARK: NSWindowDelegate

    func windowShouldClose(_ sender: NSWindow) -> Bool {
        hide()
        return false
    }
}

/// The strip under a failed session's last output.
private final class FailureBanner: NSView {
    init(message: String, target: AnyObject, action: Selector) {
        super.init(frame: .zero)
        wantsLayer = true
        layer?.backgroundColor = NSColor.windowBackgroundColor.cgColor

        let label = NSTextField(wrappingLabelWithString: message)
        label.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        let relaunch = NSButton(title: "Relaunch", target: target, action: action)
        relaunch.bezelStyle = .rounded
        relaunch.keyEquivalent = "\r"
        relaunch.setContentHuggingPriority(.required, for: .horizontal)

        let stack = NSStackView(views: [label, relaunch])
        stack.orientation = .horizontal
        stack.alignment = .centerY
        stack.spacing = 12
        stack.edgeInsets = NSEdgeInsets(top: 10, left: 16, bottom: 10, right: 16)
        stack.translatesAutoresizingMaskIntoConstraints = false
        addSubview(stack)
        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(equalTo: leadingAnchor),
            stack.trailingAnchor.constraint(equalTo: trailingAnchor),
            stack.topAnchor.constraint(equalTo: topAnchor),
            stack.bottomAnchor.constraint(equalTo: bottomAnchor),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is not supported for this view")
    }

    override func updateLayer() {
        layer?.backgroundColor = NSColor.windowBackgroundColor.cgColor
    }
}
