import Foundation

/// Values muse.app gives muse in place of what the login shell reports, for
/// the variables `LaunchEnvironment` carries. Persisted in `UserDefaults` key
/// `museEnvironment` as a dictionary; an absent or blank value uses the shell's.
enum MuseEnvironmentOverrides {
    static let defaultsKey = "museEnvironment"

    /// What may be overridden, in the order Preferences lists it.
    static var names: [String] { ["PATH"] + LaunchEnvironment.forwardedNames }

    static func load(from defaults: UserDefaults = .standard) -> [String: String] {
        sanitized(defaults.dictionary(forKey: defaultsKey)?.compactMapValues { $0 as? String } ?? [:])
    }

    static func save(_ overrides: [String: String], to defaults: UserDefaults = .standard) {
        let clean = sanitized(overrides)
        if clean.isEmpty {
            defaults.removeObject(forKey: defaultsKey)
        } else {
            defaults.set(clean, forKey: defaultsKey)
        }
    }

    /// Known names only, trimmed, single-line, and non-empty: a value Ghostty's
    /// config would drop is never stored as though it applied.
    static func sanitized(_ overrides: [String: String]) -> [String: String] {
        var clean: [String: String] = [:]
        for name in names {
            guard let value = overrides[name]?.trimmingCharacters(in: .whitespacesAndNewlines),
                  !value.isEmpty, !value.contains(where: \.isNewline) else { continue }
            clean[name] = value
        }
        return clean
    }
}
