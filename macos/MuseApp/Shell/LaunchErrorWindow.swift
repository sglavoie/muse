import AppKit

/// A plain window that says why muse could not start and offers Quit; it
/// replaces the terminal window rather than sitting over it.
enum LaunchErrorWindow {
    static func make(title: String, message: String) -> NSWindow {
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 560, height: 260),
            styleMask: [.titled, .closable, .miniaturizable],
            backing: .buffered,
            defer: false
        )
        window.title = "muse"
        window.isReleasedWhenClosed = false

        let heading = NSTextField(labelWithString: title)
        heading.font = .boldSystemFont(ofSize: NSFont.systemFontSize + 2)

        let body = NSTextField(wrappingLabelWithString: message)
        body.font = .monospacedSystemFont(ofSize: NSFont.smallSystemFontSize, weight: .regular)
        body.isSelectable = true

        let quit = NSButton(title: "Quit", target: NSApp, action: #selector(NSApplication.terminate(_:)))
        quit.keyEquivalent = "\r"

        let stack = NSStackView(views: [heading, body, quit])
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 12
        stack.edgeInsets = NSEdgeInsets(top: 20, left: 20, bottom: 20, right: 20)
        stack.translatesAutoresizingMaskIntoConstraints = false

        let content = NSView()
        content.addSubview(stack)
        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(equalTo: content.leadingAnchor),
            stack.trailingAnchor.constraint(equalTo: content.trailingAnchor),
            stack.topAnchor.constraint(equalTo: content.topAnchor),
            stack.bottomAnchor.constraint(lessThanOrEqualTo: content.bottomAnchor),
            body.widthAnchor.constraint(equalToConstant: 520),
        ])
        window.contentView = content
        window.center()
        return window
    }
}
