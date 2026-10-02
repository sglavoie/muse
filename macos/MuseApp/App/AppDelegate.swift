import AppKit
import GhosttyKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    private(set) var window: NSWindow?

    /// A hosted test bundle launches this app too; it must not open a window.
    private static var isRunningTests: Bool {
        ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil
    }

    func applicationWillFinishLaunching(_ notification: Notification) {
        // A regular app with its own Dock icon.
        NSApp.setActivationPolicy(.regular)
        NSApp.mainMenu = Self.makeMainMenu()
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Ghostty's global state must be initialized once per process before any
        // app or surface exists.
        if ghostty_init(UInt(CommandLine.argc), CommandLine.unsafeArgv) != GHOSTTY_SUCCESS {
            NSLog("muse: ghostty_init failed")
            return
        }
        guard !Self.isRunningTests else { return }

        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 900, height: 600),
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered,
            defer: false
        )
        window.title = "muse"
        window.isReleasedWhenClosed = false
        window.center()
        window.makeKeyAndOrderFront(nil)
        self.window = window
        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    /// The app menu alone, so Hide and Quit keep their standard shortcuts.
    private static func makeMainMenu() -> NSMenu {
        let main = NSMenu()
        let appItem = NSMenuItem()
        let appMenu = NSMenu()
        appMenu.addItem(withTitle: "Hide muse", action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "Quit muse", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = appMenu
        main.addItem(appItem)
        return main
    }
}
