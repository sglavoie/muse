import AppKit
import SwiftUI

/// Everything Preferences edits. Each change is saved to `UserDefaults` as it
/// is made; the app delegate applies the live ones through the callbacks.
final class PreferencesModel: ObservableObject {
    let defaults: UserDefaults

    @Published var exitBehavior: MuseExitBehavior {
        didSet {
            guard exitBehavior != oldValue else { return }
            exitBehavior.save(to: defaults)
        }
    }

    @Published var activationPolicy: MuseActivationPolicy {
        didSet {
            guard activationPolicy != oldValue else { return }
            activationPolicy.save(to: defaults)
            onActivationPolicyChange?(activationPolicy)
        }
    }

    /// nil is turned off.
    @Published var hotKey: HotKeyCombo? {
        didSet {
            guard hotKey != oldValue else { return }
            MuseHotKeyPreference.save(hotKey, to: defaults)
            onHotKeyChange?(hotKey)
        }
    }

    /// A notification names each new track.
    @Published var notificationsEnabled: Bool {
        didSet {
            guard notificationsEnabled != oldValue else { return }
            MuseNotificationsPreference.save(notificationsEnabled, to: defaults)
            onNotificationsChange?(notificationsEnabled)
        }
    }

    /// The zoom muse.app launches at; nil leaves it to the Ghostty config.
    @Published var defaultFontSize: Float? {
        didSet {
            guard defaultFontSize != oldValue else { return }
            MuseDefaultFontSize.save(defaultFontSize, to: defaults)
            onDefaultFontSizeChange?(defaultFontSize)
        }
    }

    /// The size the terminal uses while `defaultFontSize` is nil: the Ghostty
    /// config's, set by whoever loads it.
    @Published var configFontSize: Float = 13

    /// Set by whoever registers the shortcut: why it is not working, or what
    /// it collides with.
    @Published var hotKeyProblem: String?

    @Published var binaryOverride: String {
        didSet {
            let trimmed = binaryOverride.trimmingCharacters(in: .whitespacesAndNewlines)
            if trimmed.isEmpty {
                defaults.removeObject(forKey: MuseBinaryResolver.overrideDefaultsKey)
            } else {
                defaults.set(trimmed, forKey: MuseBinaryResolver.overrideDefaultsKey)
            }
        }
    }

    /// What the login shell reported at launch, set by the app delegate.
    @Published var detectedEnvironment: LaunchEnvironment?

    /// Values typed in Preferences in place of the shell's; blank uses the shell's.
    @Published var environmentOverrides: [String: String] {
        didSet {
            guard environmentOverrides != oldValue else { return }
            MuseEnvironmentOverrides.save(environmentOverrides, to: defaults)
            onEnvironmentChange?()
        }
    }

    /// The environment the running muse started with; nil while none runs.
    @Published var sessionEnvironment: [String: String]?

    /// What the next muse starts with.
    var effectiveEnvironment: LaunchEnvironment? {
        detectedEnvironment?.applying(environmentOverrides)
    }

    /// The running muse started with other values than the next one would.
    var environmentAwaitsRelaunch: Bool {
        guard let session = sessionEnvironment, let next = effectiveEnvironment else { return false }
        return next.commandEnvironment != session
    }

    /// The binary this session runs, set by the app delegate once resolved;
    /// its notes are read when Preferences opens, not at launch.
    var runningBinary: MuseBinaryResolver.Resolution?
    /// Which muse runs and its version.
    @Published private(set) var runningBinaryNote: String?
    /// A second copy that disagrees with the running one, and what to do.
    @Published private(set) var binarySkewNote: String?
    /// Where the bundled and `go install` copies live: the app's own and
    /// `~/go/bin/muse` until the app delegate sets what the resolver searched.
    var bundledBinaryPath = Bundle.main.resourceURL?.appendingPathComponent("bin/muse").path
    var goInstallBinaryPath = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("go/bin/muse").path
    var readVersion: (String) -> MuseVersion? = { MuseVersion.read(binary: $0) }

    @Published private(set) var launchAtLogin = false
    @Published private(set) var launchAtLoginNote: String?

    var onActivationPolicyChange: ((MuseActivationPolicy) -> Void)?
    var onHotKeyChange: ((HotKeyCombo?) -> Void)?
    var onDefaultFontSizeChange: ((Float?) -> Void)?
    var onNotificationsChange: ((Bool) -> Void)?
    /// True while the recorder listens, so the live shortcuts can step aside
    /// and be recorded like any other key.
    var onRecordingChange: ((Bool) -> Void)?
    var onOpenGhosttyConfig: (() -> Void)?
    var onReloadGhosttyConfig: (() -> Void)?
    var onEnvironmentChange: (() -> Void)?
    var onRelaunch: (() -> Void)?

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        exitBehavior = .load(from: defaults)
        activationPolicy = .load(from: defaults)
        hotKey = MuseHotKeyPreference.load(from: defaults)
        defaultFontSize = MuseDefaultFontSize.load(from: defaults)
        notificationsEnabled = MuseNotificationsPreference.load(from: defaults)
        binaryOverride = defaults.string(forKey: MuseBinaryResolver.overrideDefaultsKey) ?? ""
        environmentOverrides = MuseEnvironmentOverrides.load(from: defaults)
    }

    /// Re-reads what may have changed elsewhere, e.g. the login item in System
    /// Settings or a `defaults write`.
    func refresh() {
        exitBehavior = .load(from: defaults)
        let stored = defaults.string(forKey: MuseBinaryResolver.overrideDefaultsKey) ?? ""
        if stored != binaryOverride.trimmingCharacters(in: .whitespacesAndNewlines) {
            binaryOverride = stored
        }
        let storedEnvironment = MuseEnvironmentOverrides.load(from: defaults)
        if storedEnvironment != MuseEnvironmentOverrides.sanitized(environmentOverrides) {
            environmentOverrides = storedEnvironment
        }
        refreshLoginItem()
        refreshBinaryStatus()
    }

    /// Reads the running binary's version and, when it is one of the bundled
    /// and `go install` pair, how the other one compares.
    func refreshBinaryStatus() {
        guard let running = runningBinary else {
            runningBinaryNote = nil
            binarySkewNote = nil
            return
        }
        let path = running.url.path
        let version = readVersion(path)
        runningBinaryNote = "Running \(Self.abbreviated(path)): \(version?.line ?? "version unknown")"

        binarySkewNote = nil
        let other: String?
        switch running.source {
        case .bundled: other = goInstallBinaryPath
        case .newerGoInstall: other = bundledBinaryPath
        default: other = nil
        }
        guard let other, MuseBinaryResolver.isExecutableRegularFile(other),
              let otherVersion = readVersion(other), otherVersion.line != version?.line else {
            return
        }
        if running.source == .newerGoInstall {
            binarySkewNote = "Chosen over the older bundled copy (\(otherVersion.line)). "
                + "Reinstall muse.app to update it."
        } else if let built = otherVersion.built, let runningBuilt = version?.built, built < runningBuilt {
            binarySkewNote = "\(Self.abbreviated(other)) is older (\(otherVersion.line)) "
                + "and may not open a database this one migrates: `just install` updates it."
        }
    }

    static func abbreviated(_ path: String) -> String {
        let home = NSHomeDirectory()
        return path.hasPrefix(home + "/") ? "~" + path.dropFirst(home.count) : path
    }

    func refreshLoginItem() {
        launchAtLogin = LoginItem.isEnabled
        launchAtLoginNote = LoginItem.note
    }

    /// Returns the failure, which the note also shows.
    @discardableResult
    func setLaunchAtLogin(_ enabled: Bool) -> String? {
        do {
            try LoginItem.setEnabled(enabled)
            refreshLoginItem()
            return nil
        } catch {
            refreshLoginItem()
            let message = "Launch at Login could not be changed: \(error.localizedDescription)"
            launchAtLoginNote = message
            return message
        }
    }

    /// Why the override will not work, when it is set; checked the way the
    /// resolver checks it.
    var binaryOverrideProblem: String? {
        let trimmed = binaryOverride.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        let path = (trimmed as NSString).expandingTildeInPath
        return MuseBinaryResolver.isExecutableRegularFile(path) ? nil : "Not an executable file."
    }
}

/// The SwiftUI Preferences window.
final class PreferencesWindowController: NSObject, NSWindowDelegate {
    let model: PreferencesModel
    let recorder = HotKeyRecorder()
    private(set) lazy var window: NSWindow = makeWindow()

    init(model: PreferencesModel) {
        self.model = model
        super.init()
    }

    func show() {
        model.refresh()
        NSApp.activate(ignoringOtherApps: true)
        window.makeKeyAndOrderFront(nil)
    }

    private func makeWindow() -> NSWindow {
        let window = NSWindow(contentViewController: NSHostingController(
            rootView: PreferencesView(model: model, recorder: recorder)
        ))
        window.title = "muse Preferences"
        window.styleMask = [.titled, .closable]
        window.isReleasedWhenClosed = false
        window.delegate = self
        window.center()
        return window
    }

    /// A closed window records nothing.
    func windowWillClose(_ notification: Notification) {
        recorder.stop()
    }
}

struct PreferencesView: View {
    @ObservedObject var model: PreferencesModel
    @ObservedObject var recorder: HotKeyRecorder

    var body: some View {
        Form {
            Section {
                LabeledContent("Show and hide muse") {
                    HotKeyRecorderView(combo: $model.hotKey, model: model, recorder: recorder)
                }
                if let problem = model.hotKeyProblem {
                    note(problem, warning: true)
                }
                note("Works from any app. Pressing it while muse is in front hides muse; the music plays on. "
                    + "A Karabiner rule can send this same shortcut.")
            }

            Section {
                Picker("Show muse in", selection: $model.activationPolicy) {
                    ForEach(MuseActivationPolicy.allCases, id: \.self) { Text($0.label).tag($0) }
                }
                Toggle("Launch at Login", isOn: Binding(
                    get: { model.launchAtLogin },
                    set: { model.setLaunchAtLogin($0) }
                ))
                if let loginNote = model.launchAtLoginNote {
                    note(loginNote, warning: true)
                }
            }

            Section {
                LabeledContent("Default zoom") {
                    // Reset leads, so the trailing stepper holds still as it appears.
                    HStack {
                        if model.defaultFontSize != nil {
                            Button("Reset") { model.defaultFontSize = nil }
                        }
                        Stepper(
                            MuseDefaultFontSize.label(model.defaultFontSize ?? model.configFontSize),
                            value: Binding(
                                get: { model.defaultFontSize ?? model.configFontSize },
                                set: { model.defaultFontSize = $0.rounded() }
                            ),
                            in: MuseDefaultFontSize.range
                        )
                    }
                }
                note((model.defaultFontSize == nil
                    ? "The Ghostty config's font size. "
                    : "Overrides the Ghostty config's font-size. ")
                    + "⌘+ and ⌘− zoom until muse.app quits; each launch and ⌘0 return to this size.")
            }

            Section {
                Toggle("Notify when the track changes", isOn: $model.notificationsEnabled)
                note("muse.app posts these itself, in place of muse's own notify_on_track_change banners.")
            }

            Section {
                Picker("When muse exits", selection: $model.exitBehavior) {
                    ForEach(MuseExitBehavior.allCases, id: \.self) { Text($0.label).tag($0) }
                }
                note("A crash or a non-zero exit always keeps muse's last output on screen with a Relaunch button.")
            }

            Section {
                LabeledContent("muse binary") {
                    HStack {
                        TextField("", text: $model.binaryOverride, prompt: Text("Automatic"))
                            .labelsHidden()
                            .frame(minWidth: 200)
                        Button("Choose…", action: chooseBinary)
                    }
                }
                if let problem = model.binaryOverrideProblem {
                    note(problem, warning: true)
                }
                if let running = model.runningBinaryNote {
                    note(running)
                }
                if let skew = model.binarySkewNote {
                    note(skew, warning: true)
                }
                note("Leave empty to use the bundled muse, the go install location "
                    + "($GOBIN, else $GOPATH/bin, else ~/go/bin), then PATH; "
                    + "a go-installed muse built after the bundled one wins. "
                    + "Takes effect the next time muse.app starts.")
            }

            Section {
                ForEach(MuseEnvironmentOverrides.names, id: \.self) { name in
                    LabeledContent(name) {
                        let detected = model.detectedEnvironment?.value(of: name)
                        TextField("", text: environmentBinding(name), prompt: Text(detected ?? "not set"))
                            .labelsHidden()
                            .font(.system(.body, design: .monospaced))
                            .frame(minWidth: 260)
                            .help(detected.map { "From your login shell: \($0)" } ?? "Your login shell does not set \(name).")
                    }
                }
                if model.environmentAwaitsRelaunch {
                    HStack {
                        note("The running muse started with the previous values.", warning: true)
                        Spacer()
                        Button("Relaunch muse") { model.onRelaunch?() }
                    }
                }
                note("Gray values are what your login shell sets. Type one to give muse "
                    + "another; clear it to go back. muse reads these each time it starts.")
            }

            Section {
                LabeledContent("Ghostty config") {
                    HStack {
                        Button("Open") { model.onOpenGhosttyConfig?() }
                        Button("Reload") { model.onReloadGhosttyConfig?() }
                    }
                }
                note(Ghostty.Config.userConfigURL().path
                    .replacingOccurrences(of: NSHomeDirectory(), with: "~"))
            }
        }
        .formStyle(.grouped)
        .frame(width: 520)
        .fixedSize(horizontal: false, vertical: true)
    }

    private func note(_ text: String, warning: Bool = false) -> some View {
        Text(text)
            .font(.callout)
            .foregroundStyle(warning ? Color.orange : Color.secondary)
            .fixedSize(horizontal: false, vertical: true)
    }

    /// One variable's override; an empty field removes it.
    private func environmentBinding(_ name: String) -> Binding<String> {
        Binding(
            get: { model.environmentOverrides[name] ?? "" },
            set: { value in
                var overrides = model.environmentOverrides
                overrides[name] = value.isEmpty ? nil : value
                model.environmentOverrides = overrides
            }
        )
    }

    private func chooseBinary() {
        let panel = NSOpenPanel()
        panel.canChooseFiles = true
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = false
        panel.treatsFilePackagesAsDirectories = true
        panel.message = "Choose the muse binary muse.app runs."
        if panel.runModal() == .OK, let url = panel.url {
            model.binaryOverride = url.path
        }
    }
}

/// A button that records the next shortcut typed. Esc cancels; the shortcut
/// must use Control, Option, or Command, unless it is a function key.
struct HotKeyRecorderView: View {
    @Binding var combo: HotKeyCombo?
    @ObservedObject var model: PreferencesModel
    @ObservedObject var recorder: HotKeyRecorder

    var body: some View {
        HStack {
            Button(action: toggleRecording) {
                Text(label)
                    .frame(minWidth: 140)
                    .foregroundStyle(recorder.isRecording ? Color.accentColor : Color.primary)
            }
            if combo != nil, !recorder.isRecording {
                Button("Clear") { combo = nil }
            }
        }
        .onDisappear { recorder.stop() }
    }

    private var label: String {
        if recorder.isRecording {
            return recorder.hint ?? "Type a shortcut…"
        }
        return combo?.displayString ?? "Record Shortcut"
    }

    private func toggleRecording() {
        if recorder.isRecording {
            recorder.stop()
            return
        }
        let binding = $combo
        recorder.start(
            onStateChange: { [weak model] recording in model?.onRecordingChange?(recording) },
            onRecord: { binding.wrappedValue = $0 }
        )
    }
}

final class HotKeyRecorder: ObservableObject {
    @Published private(set) var isRecording = false
    @Published private(set) var hint: String?
    private var monitor: Any?
    private var onStateChange: ((Bool) -> Void)?

    func start(onStateChange: @escaping (Bool) -> Void, onRecord: @escaping (HotKeyCombo) -> Void) {
        stop()
        self.onStateChange = onStateChange
        isRecording = true
        hint = nil
        onStateChange(true)
        monitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
            guard let self else { return event }
            let chord = event.modifierFlags.intersection([.command, .control, .option, .shift])
            if Int(event.keyCode) == HotKeyRecorder.escapeKeyCode, chord.isEmpty {
                self.stop()
            } else if let combo = HotKeyCombo(event: event) {
                self.stop()
                onRecord(combo)
            } else {
                self.hint = "Add ⌃, ⌥, or ⌘…"
            }
            return nil
        }
    }

    func stop() {
        if let monitor {
            NSEvent.removeMonitor(monitor)
            self.monitor = nil
        }
        guard isRecording else { return }
        isRecording = false
        hint = nil
        onStateChange?(false)
        onStateChange = nil
    }

    deinit {
        if let monitor {
            NSEvent.removeMonitor(monitor)
        }
    }

    private static let escapeKeyCode = 0x35
}
