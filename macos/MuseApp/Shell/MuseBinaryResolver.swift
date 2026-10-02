import Foundation

/// Finds the `muse` binary the surface runs, first match wins: the
/// `UserDefaults` override, the copy bundled in `Contents/Resources/bin`, the
/// `go install` location (`$GOBIN`, else `$GOPATH/bin`, else `~/go/bin`, where
/// `just install` puts it), then `PATH`. A `go install`ed muse built after the
/// bundled copy runs instead of it, since the older one could not read a
/// database the newer one has already migrated.
struct MuseBinaryResolver {
    /// The `UserDefaults` key holding an explicit binary path.
    static let overrideDefaultsKey = "museBinaryPath"

    enum Source: Equatable {
        case userDefaultsOverride
        case bundled
        case goInstall
        /// The `go install` location, chosen over an older bundled copy.
        case newerGoInstall
        case path
    }

    struct Resolution: Equatable {
        let url: URL
        let source: Source
    }

    /// Nothing resolved. An override that names no executable fails here
    /// rather than quietly running some other `muse`.
    struct NotFound: Error, Equatable {
        /// The override that was set but does not name an executable file.
        let invalidOverride: String?
        /// Every candidate path checked, in order.
        let tried: [String]

        var message: String {
            var lines: [String] = []
            if let invalidOverride {
                lines.append("The muse binary override does not name an executable file:")
                lines.append("  \(invalidOverride)")
                lines.append("")
                lines.append("Fix it, or remove it to use the default search:")
                lines.append("  defaults delete io.github.sglavoie.muse \(MuseBinaryResolver.overrideDefaultsKey)")
            } else {
                lines.append("muse.app could not find the muse binary. Looked in:")
                lines.append(contentsOf: tried.map { "  \($0)" })
                lines.append("")
                lines.append("Install it with `just install`, or point muse.app at one:")
                lines.append("  defaults write io.github.sglavoie.muse \(MuseBinaryResolver.overrideDefaultsKey) /path/to/muse")
            }
            return lines.joined(separator: "\n")
        }
    }

    var overridePath: String?
    var bundleResourcesURL: URL?
    var homeDirectory: URL
    /// A colon-separated search path, normally the login shell's `PATH`.
    var searchPath: String
    /// The login shell's `GOBIN` and `GOPATH`; nil or empty when unset.
    var goBin: String?
    var goPath: String?
    var isExecutableFile: (String) -> Bool = MuseBinaryResolver.isExecutableRegularFile
    /// When a binary was built; nil when unknown, which never displaces the
    /// bundled copy.
    var buildTime: (String) -> Date? = { _ in nil }

    /// The resolver for this process: its bundle, the standard defaults, and
    /// what the login shell reported.
    static func live(environment: LaunchEnvironment, defaults: UserDefaults = .standard) -> MuseBinaryResolver {
        MuseBinaryResolver(
            overridePath: defaults.string(forKey: overrideDefaultsKey),
            bundleResourcesURL: Bundle.main.resourceURL,
            homeDirectory: URL(fileURLWithPath: environment.home),
            searchPath: environment.path,
            goBin: environment.probed["GOBIN"],
            goPath: environment.probed["GOPATH"],
            buildTime: { MuseVersion.read(binary: $0)?.built }
        )
    }

    func resolve() -> Result<Resolution, NotFound> {
        if let raw = overridePath?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty {
            let path = expandingTilde(raw)
            guard isExecutableFile(path) else {
                return .failure(NotFound(invalidOverride: path, tried: [path]))
            }
            return .success(Resolution(url: URL(fileURLWithPath: path), source: .userDefaultsOverride))
        }

        var candidates: [(String, Source)] = []
        if let bundleResourcesURL {
            candidates.append((bundleResourcesURL.appendingPathComponent("bin/muse").path, .bundled))
        }
        candidates.append((goInstallPath, .goInstall))
        for dir in searchPath.split(separator: ":") where !dir.isEmpty {
            candidates.append((URL(fileURLWithPath: expandingTilde(String(dir))).appendingPathComponent("muse").path, .path))
        }

        var tried: [String] = []
        for (path, source) in candidates where !tried.contains(path) {
            tried.append(path)
            if isExecutableFile(path) {
                if source == .bundled, let newer = newerGoInstall(than: path) {
                    return .success(Resolution(url: URL(fileURLWithPath: newer), source: .newerGoInstall))
                }
                return .success(Resolution(url: URL(fileURLWithPath: path), source: source))
            }
        }
        return .failure(NotFound(invalidOverride: nil, tried: tried))
    }

    /// The `go install` location, when it is executable and built after `bundled`.
    private func newerGoInstall(than bundled: String) -> String? {
        let installed = goInstallPath
        guard isExecutableFile(installed),
              let installedBuilt = buildTime(installed),
              let bundledBuilt = buildTime(bundled),
              installedBuilt > bundledBuilt else {
            return nil
        }
        return installed
    }

    /// Where `go install` writes muse: `$GOBIN`, else the first `$GOPATH`
    /// entry's `bin`, else `~/go/bin`, as the go command itself decides.
    var goInstallPath: String {
        if let goBin = nonEmpty(goBin) {
            return URL(fileURLWithPath: expandingTilde(goBin)).appendingPathComponent("muse").path
        }
        if let first = nonEmpty(goPath)?.split(separator: ":").first.map(String.init), !first.isEmpty {
            return URL(fileURLWithPath: expandingTilde(first)).appendingPathComponent("bin/muse").path
        }
        return homeDirectory.appendingPathComponent("go/bin/muse").path
    }

    private func nonEmpty(_ value: String?) -> String? {
        guard let value = value?.trimmingCharacters(in: .whitespacesAndNewlines), !value.isEmpty else { return nil }
        return value
    }

    private func expandingTilde(_ path: String) -> String {
        if path == "~" { return homeDirectory.path }
        if path.hasPrefix("~/") { return homeDirectory.appendingPathComponent(String(path.dropFirst(2))).path }
        return path
    }

    /// `isExecutableFile(atPath:)` alone accepts a searchable directory.
    static func isExecutableRegularFile(_ path: String) -> Bool {
        var isDirectory: ObjCBool = false
        guard FileManager.default.fileExists(atPath: path, isDirectory: &isDirectory), !isDirectory.boolValue else {
            return false
        }
        return FileManager.default.isExecutableFile(atPath: path)
    }
}
