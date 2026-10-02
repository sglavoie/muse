import Foundation

/// The environment muse runs with. An app launched from Finder or launchd
/// inherits a minimal environment, so the user's login shell is asked for the
/// variables muse reads — its `PATH` (where mpv, exiftool and ffprobe live),
/// its XDG directories, and its locale — as an interactive shell would set
/// them; `HOME` is passed through, and TERM/COLORTERM are left to Ghostty.
struct LaunchEnvironment: Equatable {
    /// The variables taken from the login shell, `PATH` aside. Without the XDG
    /// ones a Finder-launched muse would open another config and database than
    /// the terminal's, and miss the instance lock a terminal muse holds.
    static let forwardedNames = [
        "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR", "LANG", "LC_ALL",
    ]

    /// Every other `LC_*` variable the shell exports is forwarded too.
    static let forwardedPrefix = "LC_"

    /// Read from the login shell for the binary resolver alone, never passed
    /// to muse: where `go install` put it.
    static let probedNames = ["GOBIN", "GOPATH"]

    var home: String
    var path: String
    /// Forwarded variables the shell set to a non-empty value.
    var forwarded: [String: String] = [:]
    /// Probed variables the shell set to a non-empty value.
    var probed: [String: String] = [:]

    /// Variables the surface's command receives on top of its inherited ones.
    var commandEnvironment: [String: String] {
        forwarded.merging(["HOME": home, "PATH": path]) { _, fixed in fixed }
    }

    /// The value muse receives for one of `PATH` and the forwarded names.
    func value(of name: String) -> String? {
        name == "PATH" ? path : forwarded[name]
    }

    /// This environment with Preferences' values in place of the shell's.
    func applying(_ overrides: [String: String]) -> LaunchEnvironment {
        var result = self
        for (name, value) in MuseEnvironmentOverrides.sanitized(overrides) {
            if name == "PATH" {
                result.path = value
            } else {
                result.forwarded[name] = value
            }
        }
        return result
    }

    static func isForwarded(_ name: String) -> Bool {
        forwardedNames.contains(name) || name.hasPrefix(forwardedPrefix)
    }

    static func current(processEnvironment: [String: String] = ProcessInfo.processInfo.environment) -> LaunchEnvironment {
        let home = processEnvironment["HOME"] ?? FileManager.default.homeDirectoryForCurrentUser.path
        let fallbackPath = processEnvironment["PATH"] ?? "/usr/bin:/bin:/usr/sbin:/sbin"
        let shell = processEnvironment["SHELL"] ?? "/bin/zsh"
        // Interactive first, since that is where most profiles export these;
        // a shell that cannot start interactively still reports its login PATH.
        let reported = loginShellEnvironment(shell: shell, interactive: true)
            ?? loginShellEnvironment(shell: shell, interactive: false)
            ?? [:]
        return make(home: home, fallbackPath: fallbackPath, reported: reported, processEnvironment: processEnvironment)
    }

    /// Combines the shell's report with what the app itself inherited: a
    /// variable from a `launchctl setenv` is the fallback for one the shell
    /// leaves unset.
    static func make(
        home: String, fallbackPath: String, reported: [String: String], processEnvironment: [String: String]
    ) -> LaunchEnvironment {
        func pick(_ name: String) -> String? {
            guard let value = reported[name] ?? processEnvironment[name], !value.isEmpty else { return nil }
            return value
        }
        var forwarded: [String: String] = [:]
        let names = Set(forwardedNames)
            .union(reported.keys.filter(isForwarded))
            .union(processEnvironment.keys.filter { $0.hasPrefix(forwardedPrefix) })
        for name in names {
            forwarded[name] = pick(name)
        }
        var probed: [String: String] = [:]
        for name in probedNames {
            probed[name] = pick(name)
        }
        return LaunchEnvironment(home: home, path: reported["PATH"] ?? fallbackPath, forwarded: forwarded, probed: probed)
    }

    static let beginMarker = "__MUSE_ENV_BEGIN__"
    static let endMarker = "__MUSE_ENV_END__"

    /// The shell script printing `PATH`, every forwarded and probed variable,
    /// and each exported `LC_*` one as NUL-terminated `NAME=value` records
    /// between markers, so anything a profile prints is ignored.
    static var reportScript: String {
        let records = (["PATH"] + forwardedNames + probedNames)
            .map { "printf '%s=%s\\0' '\($0)' \"$\($0)\"" }
            .joined(separator: "; ")
        // The LC_ names come from env, since a shell cannot glob variable names.
        let locales = "for n in $(env | sed -n 's/^\\(\(forwardedPrefix)[A-Za-z0-9_]*\\)=.*/\\1/p'); do "
            + "printf '%s=%s\\0' \"$n\" \"$(printenv \"$n\")\"; done"
        return "printf '%s' '\(beginMarker)'; \(records); \(locales); printf '%s' '\(endMarker)'"
    }

    /// Runs `shell -l [-i] -c` and reads the report. Nil on failure, on a
    /// report without a `PATH`, or after `timeout`.
    static func loginShellEnvironment(shell: String, interactive: Bool, timeout: TimeInterval = 5) -> [String: String]? {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: shell)
        process.arguments = (interactive ? ["-l", "-i"] : ["-l"]) + ["-c", reportScript]
        let stdout = Pipe()
        process.standardOutput = stdout
        process.standardError = FileHandle.nullDevice
        process.standardInput = FileHandle.nullDevice

        do {
            try process.run()
        } catch {
            Ghostty.logger.warning("login shell \(shell, privacy: .public) failed to start: \(error.localizedDescription, privacy: .public)")
            return nil
        }
        // Read to EOF off this thread, so a chatty profile cannot fill the
        // pipe, and give up on a shell that never closes it.
        let box = OutputBox()
        let done = DispatchSemaphore(value: 0)
        DispatchQueue.global().async {
            box.data = stdout.fileHandleForReading.readDataToEndOfFile()
            done.signal()
        }
        if done.wait(timeout: .now() + timeout) == .timedOut {
            process.terminate()
            Ghostty.logger.warning("login shell \(shell, privacy: .public) timed out reporting its environment")
            return nil
        }
        process.waitUntilExit()
        let report = extractReport(from: String(decoding: box.data, as: UTF8.self), begin: beginMarker, end: endMarker)
        guard let report, let path = report["PATH"], !path.isEmpty else { return nil }
        return report
    }

    private final class OutputBox {
        var data = Data()
    }

    /// The `NAME=value` records between the markers; empty values are dropped,
    /// since the script cannot tell an unset variable from an empty one.
    static func extractReport(from output: String, begin: String, end: String) -> [String: String]? {
        guard let start = output.range(of: begin),
              let stop = output.range(of: end, range: start.upperBound..<output.endIndex) else {
            return nil
        }
        var report: [String: String] = [:]
        for record in output[start.upperBound..<stop.lowerBound].split(separator: "\0") {
            guard let equals = record.firstIndex(of: "=") else { continue }
            let value = String(record[record.index(after: equals)...])
            if !value.isEmpty {
                report[String(record[..<equals])] = value
            }
        }
        return report
    }
}
