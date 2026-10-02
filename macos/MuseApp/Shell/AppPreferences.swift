import AppKit
import ServiceManagement

/// Where muse.app shows itself: the Dock and the menu bar, or the menu bar
/// alone. Persisted in `UserDefaults` key `museActivationPolicy`; an absent or
/// unknown value reads as the default.
enum MuseActivationPolicy: String, CaseIterable {
    case dockAndMenuBar = "regular"
    case menuBarOnly = "accessory"

    static let defaultsKey = "museActivationPolicy"
    static let `default`: MuseActivationPolicy = .dockAndMenuBar

    static func load(from defaults: UserDefaults = .standard) -> MuseActivationPolicy {
        defaults.string(forKey: defaultsKey).flatMap(MuseActivationPolicy.init(rawValue:)) ?? .default
    }

    func save(to defaults: UserDefaults = .standard) {
        defaults.set(rawValue, forKey: Self.defaultsKey)
    }

    var appKitPolicy: NSApplication.ActivationPolicy {
        switch self {
        case .dockAndMenuBar: return .regular
        case .menuBarOnly: return .accessory
        }
    }

    var label: String {
        switch self {
        case .dockAndMenuBar: return "Dock and menu bar"
        case .menuBarOnly: return "Menu bar only"
        }
    }
}

/// What the hotkey and a left click on the status item do: hide muse only
/// when its window is already the one in front, otherwise bring it forward.
enum TerminalToggle: Equatable {
    case show
    case hide

    static func action(appIsActive: Bool, windowIsVisible: Bool, windowIsKey: Bool) -> TerminalToggle {
        appIsActive && windowIsVisible && windowIsKey ? .hide : .show
    }
}

/// Launch at Login through `SMAppService.mainApp`: muse.app itself is the
/// login item, with no helper bundle.
enum LoginItem {
    static var status: SMAppService.Status { SMAppService.mainApp.status }

    /// Registered, or registered and waiting for the user to allow it in
    /// System Settings › General › Login Items.
    static var isEnabled: Bool { status == .enabled || status == .requiresApproval }

    static func setEnabled(_ enabled: Bool) throws {
        if enabled {
            try SMAppService.mainApp.register()
        } else {
            try SMAppService.mainApp.unregister()
        }
    }

    /// A note for Preferences, when the state needs one.
    static var note: String? {
        switch status {
        case .requiresApproval:
            return "Allow muse in System Settings › General › Login Items to finish turning this on."
        default:
            return nil
        }
    }
}

/// The terminal's font size at launch, in points: the zoom muse.app opens at
/// and the size Cmd+0 returns to. Zooming with Cmd+= and Cmd+- changes only
/// the running session. Persisted in `UserDefaults` key `museDefaultFontSize`;
/// nil, stored or not, leaves the size to the Ghostty config.
enum MuseDefaultFontSize {
    static let defaultsKey = "museDefaultFontSize"
    static let range: ClosedRange<Float> = 6...72

    /// An absent, non-numeric, or out-of-range value reads as nil.
    static func load(from defaults: UserDefaults = .standard) -> Float? {
        guard let number = defaults.object(forKey: defaultsKey) as? NSNumber else { return nil }
        let points = number.floatValue
        return range.contains(points) ? points : nil
    }

    static func save(_ points: Float?, to defaults: UserDefaults = .standard) {
        if let points {
            defaults.set(clamped(points), forKey: defaultsKey)
        } else {
            defaults.removeObject(forKey: defaultsKey)
        }
    }

    static func clamped(_ points: Float) -> Float {
        min(max(points, range.lowerBound), range.upperBound)
    }

    /// "13 pt", or "13.5 pt" for a fractional size.
    static func label(_ points: Float) -> String {
        "\(Ghostty.ConfigOverrides.number(points)) pt"
    }
}

/// Whether muse.app posts a notification when the track changes. Persisted in
/// `UserDefaults` key `museNotifications`; absent reads as on.
enum MuseNotificationsPreference {
    static let defaultsKey = "museNotifications"

    static func load(from defaults: UserDefaults = .standard) -> Bool {
        defaults.object(forKey: defaultsKey) as? Bool ?? true
    }

    static func save(_ enabled: Bool, to defaults: UserDefaults = .standard) {
        defaults.set(enabled, forKey: defaultsKey)
    }
}
