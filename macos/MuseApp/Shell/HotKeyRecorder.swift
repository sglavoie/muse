import AppKit
import SwiftUI

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
