import Foundation

/// What muse.app does when muse exits cleanly, e.g. after ctrl+c. A failed
/// exit always stays on screen instead, whatever this says.
enum MuseExitBehavior: String, CaseIterable {
    /// Hide the window; the next show starts a fresh muse.
    case relaunchOnShow = "relaunch"
    /// Quit muse.app.
    case quitApp = "quit"

    static let defaultsKey = "museExitBehavior"
    static let `default`: MuseExitBehavior = .relaunchOnShow

    /// An absent or unknown stored value reads as the default.
    static func load(from defaults: UserDefaults = .standard) -> MuseExitBehavior {
        defaults.string(forKey: defaultsKey).flatMap(MuseExitBehavior.init(rawValue:)) ?? .default
    }

    func save(to defaults: UserDefaults = .standard) {
        defaults.set(rawValue, forKey: Self.defaultsKey)
    }

    var label: String {
        switch self {
        case .relaunchOnShow: return "Hide, and start a fresh muse next time"
        case .quitApp: return "Quit muse.app"
        }
    }
}

/// How muse ended, as the launcher's shell reported it.
enum MuseExitStatus: Equatable {
    case exited(Int32)
    case signaled(Int32)
    /// The launcher left no readable status.
    case unknown

    /// muse's status when another muse holds its instance lock.
    static let alreadyRunningCode: Int32 = 3

    /// A shell status: 0-128 is an exit code, above 128 a signal number plus
    /// 128, the shell's own convention.
    init(shellStatus: Int32?) {
        guard let status = shellStatus, status >= 0, status <= 255 else {
            self = .unknown
            return
        }
        self = status > 128 ? .signaled(status - 128) : .exited(status)
    }

    /// Reads the one-line status file the launcher writes.
    init(contentsOf url: URL) {
        let text = try? String(contentsOf: url, encoding: .utf8)
        self.init(shellStatus: text.flatMap { Int32($0.trimmingCharacters(in: .whitespacesAndNewlines)) })
    }

    var isClean: Bool { self == .exited(0) }

    /// One sentence for the failure banner.
    var summary: String {
        switch self {
        case .exited(let code):
            return "muse exited with status \(code)."
        case .signaled(let signal):
            return "muse was terminated by signal \(signal) (\(Self.signalName(signal)))."
        case .unknown:
            return "muse stopped, and its exit status could not be read."
        }
    }

    static func signalName(_ signal: Int32) -> String {
        guard let name = strsignal(signal) else { return "unknown signal" }
        return String(cString: name)
    }
}

/// What the window does when muse exits.
enum MuseExitAction: Equatable {
    case hideAndRelaunchOnShow
    case quitApp
    /// Another muse, in a terminal, holds the instance lock; `pid` is the
    /// holder's when its lockfile could be read.
    case alreadyRunning(pid: Int32?)
    /// Keep the last output on screen with a Relaunch button.
    case showFailure(String)

    static func action(
        for status: MuseExitStatus, behavior: MuseExitBehavior, holderPID: () -> Int32? = { nil }
    ) -> MuseExitAction {
        if status == .exited(MuseExitStatus.alreadyRunningCode) {
            return .alreadyRunning(pid: holderPID())
        }
        guard status.isClean else { return .showFailure(status.summary) }
        switch behavior {
        case .relaunchOnShow: return .hideAndRelaunchOnShow
        case .quitApp: return .quitApp
        }
    }

    /// What the banner says for a failure; nil for an action that shows none.
    var bannerMessage: String? {
        switch self {
        case .hideAndRelaunchOnShow, .quitApp:
            return nil
        case .alreadyRunning(let pid):
            let holder = pid.map { " (PID \($0))" } ?? ""
            return "muse is already running in a terminal\(holder). Quit it there, then Relaunch."
        case .showFailure(let summary):
            return summary + " Its last output is shown above."
        }
    }
}

/// muse's single-instance lock, `<runtime>/muse.pid`, read for the PID of the
/// muse holding it. The runtime directory follows muse's own rule:
/// `$XDG_RUNTIME_DIR/muse` when that is absolute, else `$TMPDIR/muse-<uid>`.
enum MuseInstanceLock {
    static func path(environment: [String: String]) -> URL {
        if let runtime = environment["XDG_RUNTIME_DIR"], runtime.hasPrefix("/") {
            return URL(fileURLWithPath: runtime).appendingPathComponent("muse/muse.pid")
        }
        let tmp = environment["TMPDIR"].flatMap { $0.isEmpty ? nil : $0 } ?? "/tmp"
        return URL(fileURLWithPath: tmp).appendingPathComponent("muse-\(getuid())/muse.pid")
    }

    /// The recorded PID, or nil when the file is missing or holds none.
    static func holderPID(environment: [String: String]) -> Int32? {
        guard let text = try? String(contentsOf: path(environment: environment), encoding: .utf8),
              let pid = Int32(text.trimmingCharacters(in: .whitespacesAndNewlines)), pid > 0 else {
            return nil
        }
        return pid
    }
}
