// Ported and trimmed from Ghostty's macOS app (MIT); see macos/NOTICE-ghostty.
import AppKit
import GhosttyKit

// Menu validation and the NSTextInputClient (IME) conformance for the surface.

extension Ghostty.SurfaceView: NSMenuItemValidation {
    func validateMenuItem(_ item: NSMenuItem) -> Bool {
        switch item.action {
        case #selector(copy(_:)):
            guard let surface else { return false }
            return ghostty_surface_has_selection(surface)
        default:
            return true
        }
    }
}

// MARK: - NSTextInputClient

extension Ghostty.SurfaceView: NSTextInputClient {
    func hasMarkedText() -> Bool {
        markedText.length > 0
    }

    func markedRange() -> NSRange {
        guard markedText.length > 0 else { return NSRange() }
        return NSRange(location: 0, length: markedText.length)
    }

    func selectedRange() -> NSRange {
        guard let surface else { return NSRange() }
        var text = ghostty_text_s()
        guard ghostty_surface_read_selection(surface, &text) else { return NSRange() }
        defer { ghostty_surface_free_text(surface, &text) }
        return NSRange(location: Int(text.offset_start), length: Int(text.offset_len))
    }

    func setMarkedText(_ string: Any, selectedRange: NSRange, replacementRange: NSRange) {
        switch string {
        case let value as NSAttributedString:
            markedText = NSMutableAttributedString(attributedString: value)
        case let value as String:
            markedText = NSMutableAttributedString(string: value)
        default:
            return
        }

        // Outside keyDown (a layout switch mid dead key) the preedit is synced
        // here, since no key event will.
        if keyTextAccumulator == nil {
            syncPreedit()
        }
    }

    func unmarkText() {
        guard markedText.length > 0 else { return }
        markedText.mutableString.setString("")
        syncPreedit()
    }

    func validAttributesForMarkedText() -> [NSAttributedString.Key] {
        []
    }

    func attributedSubstring(forProposedRange range: NSRange, actualRange: NSRangePointer?) -> NSAttributedString? {
        guard let surface, range.length > 0 else { return nil }
        // System callers ask for ranges that match nothing; upstream answers
        // every one with the selection.
        var text = ghostty_text_s()
        guard ghostty_surface_read_selection(surface, &text) else { return nil }
        defer { ghostty_surface_free_text(surface, &text) }
        return NSAttributedString(string: String(cString: text.text))
    }

    func characterIndex(for point: NSPoint) -> Int {
        0
    }

    /// Where the input method draws its candidate window: Ghostty's cursor.
    func firstRect(forCharacterRange range: NSRange, actualRange: NSRangePointer?) -> NSRect {
        guard let surface else {
            return NSRect(x: frame.origin.x, y: frame.origin.y, width: 0, height: 0)
        }

        let cell = cellSize
        var x: Double = 0
        var y: Double = 0
        var width: Double = cell.width
        var height: Double = cell.height
        ghostty_surface_ime_point(surface, &x, &y, &width, &height)
        if range.length == 0, width > 0 {
            // Dictation's indicator wants a zero-width rect at the insertion point.
            width = 0
            x += cell.width * Double(range.location + range.length)
        }

        let viewRect = NSRect(x: x, y: frame.size.height - y, width: width, height: max(height, cell.height))
        let windowRect = convert(viewRect, to: nil)
        guard let window else { return windowRect }
        return window.convertToScreen(windowRect)
    }

    func insertText(_ string: Any, replacementRange: NSRange) {
        guard NSApp.currentEvent != nil, let surface else { return }

        let chars: String
        switch string {
        case let value as NSAttributedString: chars = value.string
        case let value as String: chars = value
        default: return
        }

        // A commit ends the composition.
        unmarkText()

        // Inside keyDown the text rides the key event.
        if keyTextAccumulator != nil {
            keyTextAccumulator?.append(chars)
            return
        }

        let len = chars.utf8CString.count
        guard len > 1 else { return }
        chars.withCString { ptr in
            ghostty_surface_text(surface, ptr, UInt(len - 1))
        }
    }

    /// Unimplemented text commands would beep; a Cmd chord AppKit declined
    /// comes back here and is redispatched so `keyDown` encodes it.
    override func doCommand(by selector: Selector) {
        if let last = lastPerformKeyEvent,
           let current = NSApp.currentEvent,
           last == current.timestamp {
            NSApp.sendEvent(current)
            return
        }

        switch selector {
        case #selector(moveToBeginningOfDocument(_:)):
            performBindingAction("scroll_to_top")
        case #selector(moveToEndOfDocument(_:)):
            performBindingAction("scroll_to_bottom")
        default:
            break
        }
    }

    /// Mirrors `markedText` to the surface's preedit.
    func syncPreedit(clearIfNeeded: Bool = true) {
        guard let surface else { return }
        if markedText.length > 0 {
            let str = markedText.string
            let len = str.utf8CString.count
            str.withCString { ptr in
                ghostty_surface_preedit(surface, ptr, UInt(len - 1))
            }
        } else if clearIfNeeded {
            ghostty_surface_preedit(surface, nil, 0)
        }
    }

    /// One cell in points, from the surface's pixel metrics.
    var cellSize: NSSize {
        guard let surface else { return .zero }
        let size = ghostty_surface_size(surface)
        let scale = window?.backingScaleFactor ?? NSScreen.main?.backingScaleFactor ?? 1
        return NSSize(width: Double(size.cell_width_px) / scale, height: Double(size.cell_height_px) / scale)
    }
}
