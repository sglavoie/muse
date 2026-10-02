import AppKit

/// What the status item asks of the app.
protocol StatusItemDelegate: AnyObject {
    /// True when muse's window is the one in front, so Show/Hide reads Hide.
    var isTerminalFrontmost: Bool { get }
    var canRelaunchTerminal: Bool { get }
    var isLaunchAtLoginEnabled: Bool { get }
    func toggleTerminal()
    func relaunchTerminal()
    func showPreferences(_ sender: Any?)
    func toggleLaunchAtLogin()
}

/// The menu-bar icon: a left click toggles muse like the global shortcut; a
/// right click, or a Control-click, opens its menu.
final class StatusItemController: NSObject, NSMenuDelegate {
    let statusItem: NSStatusItem
    let menu = NSMenu()
    private weak var delegate: StatusItemDelegate?

    private let toggleItem = NSMenuItem(title: "Show muse", action: #selector(toggle(_:)), keyEquivalent: "")
    private let relaunchItem = NSMenuItem(title: "Relaunch muse", action: #selector(relaunch(_:)), keyEquivalent: "")
    private let preferencesItem = NSMenuItem(title: "Preferences…", action: #selector(preferences(_:)), keyEquivalent: "")
    private let loginItem = NSMenuItem(title: "Launch at Login", action: #selector(launchAtLogin(_:)), keyEquivalent: "")
    private let quitItem = NSMenuItem(title: "Quit muse", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "")

    init(delegate: StatusItemDelegate, statusBar: NSStatusBar = .system) {
        self.delegate = delegate
        statusItem = statusBar.statusItem(withLength: NSStatusItem.squareLength)
        super.init()

        for item in [toggleItem, relaunchItem, preferencesItem, loginItem] {
            item.target = self
        }
        menu.addItem(toggleItem)
        menu.addItem(relaunchItem)
        menu.addItem(.separator())
        menu.addItem(preferencesItem)
        menu.addItem(loginItem)
        menu.addItem(.separator())
        menu.addItem(quitItem)
        menu.delegate = self
        menu.autoenablesItems = false

        if let button = statusItem.button {
            button.image = Self.icon()
            button.toolTip = "muse"
            button.target = self
            button.action = #selector(clicked(_:))
            button.sendAction(on: [.leftMouseUp, .rightMouseUp])
        }
    }

    deinit {
        statusItem.statusBar?.removeStatusItem(statusItem)
    }

    /// A StatusIcon asset when the bundle has one, else the music-note
    /// symbol, as a template image so the menu bar tints it for light and dark.
    static func icon() -> NSImage {
        // A copy, since resizing the shared named image would leak to others.
        let image = (NSImage(named: "StatusIcon")?.copy() as? NSImage)
            ?? NSImage(systemSymbolName: "music.note", accessibilityDescription: "muse")
            ?? NSImage(size: NSSize(width: 18, height: 18))
        image.size = NSSize(width: 18, height: 18)
        image.isTemplate = true
        image.accessibilityDescription = "muse"
        return image
    }

    /// Brings the menu's titles and states in line with the app.
    func refreshMenu() {
        let frontmost = delegate?.isTerminalFrontmost ?? false
        toggleItem.title = frontmost ? "Hide muse" : "Show muse"
        relaunchItem.isEnabled = delegate?.canRelaunchTerminal ?? false
        loginItem.state = (delegate?.isLaunchAtLoginEnabled ?? false) ? .on : .off
    }

    static func opensMenu(_ event: NSEvent?) -> Bool {
        guard let event else { return false }
        return event.type == .rightMouseUp || event.modifierFlags.contains(.control)
    }

    @objc private func clicked(_ sender: Any?) {
        if Self.opensMenu(NSApp.currentEvent) {
            refreshMenu()
            // Attached only for this click, so a left click keeps its action.
            statusItem.menu = menu
            statusItem.button?.performClick(nil)
            statusItem.menu = nil
        } else {
            delegate?.toggleTerminal()
        }
    }

    func menuNeedsUpdate(_ menu: NSMenu) {
        refreshMenu()
    }

    @objc private func toggle(_ sender: Any?) { delegate?.toggleTerminal() }
    @objc private func relaunch(_ sender: Any?) { delegate?.relaunchTerminal() }
    @objc private func preferences(_ sender: Any?) { delegate?.showPreferences(sender) }
    @objc private func launchAtLogin(_ sender: Any?) { delegate?.toggleLaunchAtLogin() }
}
