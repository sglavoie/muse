// Build with: swiftc -parse-as-library demo-driver.swift -o demo-driver
// Captures only the specified app's window and posts keys only to its PID.
import AppKit
import ApplicationServices
import ScreenCaptureKit
import Vision

struct DemoError: Error, CustomStringConvertible {
    let description: String
    init(_ description: String) { self.description = description }
}

struct Scene: Decodable {
    let name: String
    let keys: [String]
    let seconds: Double
    let contains: [String]
}

@main
struct DemoDriver {
    static func permissions() throws {
        let accessibility = AXIsProcessTrusted()
        let recording = CGPreflightScreenCaptureAccess()
        print("Accessibility: \(accessibility ? "enabled" : "disabled"); Screen Recording: \(recording ? "enabled" : "disabled")")
        guard accessibility, recording else {
            throw DemoError("Enable the disabled permissions for your terminal/agent app in System Settings > Privacy & Security, then restart that app if macOS requests it.")
        }
    }

    @MainActor
    static func main() async {
        setbuf(stdout, nil)
        do {
            try permissions()
            let args = CommandLine.arguments
            if args.count == 2, args[1] == "--check" {
                print("Accessibility and Screen Recording are available.")
                return
            }
            guard args.count == 5, let pid = Int32(args[1]),
                  let app = NSRunningApplication(processIdentifier: pid),
                  app.bundleIdentifier == "io.github.sglavoie.muse.demo" else {
                throw DemoError("Usage: demo-driver DEMO_PID SCENES_JSON FRAMES_DIR REPORT_JSON (only muse's isolated demo bundle is accepted)")
            }
            let scenes = try JSONDecoder().decode([Scene].self, from: Data(contentsOf: URL(fileURLWithPath: args[2])))
            let frames = URL(fileURLWithPath: args[3], isDirectory: true)
            try FileManager.default.createDirectory(at: frames, withIntermediateDirectories: true)
            app.activate(options: [.activateAllWindows])
            try await Task.sleep(for: .seconds(1))
            let element = AXUIElementCreateApplication(pid)
            var value: CFTypeRef?
            guard AXUIElementCopyAttributeValue(element, kAXWindowsAttribute as CFString, &value) == .success,
                  let windows = value as? [AXUIElement], let window = windows.first else {
                throw DemoError("The demo app has no accessible window.")
            }
            var size = CGSize(width: 1200, height: 800)
            guard let axSize = AXValueCreate(.cgSize, &size),
                  AXUIElementSetAttributeValue(window, kAXSizeAttribute as CFString, axSize) == .success else {
                throw DemoError("Could not resize the demo window.")
            }
            AXUIElementPerformAction(window, kAXRaiseAction as CFString)
            try await Task.sleep(for: .seconds(1))
            let content = try await SCShareableContent.excludingDesktopWindows(true, onScreenWindowsOnly: true)
            guard let target = content.windows.first(where: {
                $0.owningApplication?.processID == pid && $0.windowLayer == 0 && $0.frame.width > 500
            }) else { throw DemoError("Could not find the demo window for capture.") }
            let filter = SCContentFilter(desktopIndependentWindow: target)
            let config = SCStreamConfiguration()
            config.width = 2400
            config.height = 1600
            config.showsCursor = false
            config.ignoreShadowsSingleWindow = true
            config.captureResolution = .best
            let fps = 4
            var index = 0
            var report: [[String: Any]] = []
            for scene in scenes {
                guard !app.isTerminated else { throw DemoError("The demo app exited during \(scene.name).") }
                app.activate(options: [.activateAllWindows])
                AXUIElementPerformAction(window, kAXRaiseAction as CFString)
                try await Task.sleep(for: .milliseconds(250))
                for key in scene.keys {
                    try post(key, to: pid)
                    try await Task.sleep(for: .milliseconds(160))
                }
                try await Task.sleep(for: .milliseconds(600))
                var recognized = ""
                let count = max(1, Int(scene.seconds * Double(fps)))
                for frame in 0..<count {
                    let started = ContinuousClock.now
                    let image = try await SCScreenshotManager.captureImage(contentFilter: filter, configuration: config)
                    let bitmap = NSBitmapImageRep(cgImage: image)
                    guard let png = bitmap.representation(using: .png, properties: [:]) else {
                        throw DemoError("Could not encode screenshot.")
                    }
                    try png.write(to: frames.appendingPathComponent(String(format: "%04d.png", index)))
                    if frame == 0 {
                        recognized = try readText(image)
                        for expected in scene.contains where !recognized.localizedCaseInsensitiveContains(expected) {
                            try png.write(to: URL(fileURLWithPath: args[4]).deletingLastPathComponent().appendingPathComponent("failed-scene.png"))
                            throw DemoError("Scene '\(scene.name)' is missing '\(expected)'. OCR: \(recognized)")
                        }
                    }
                    index += 1
                    let remaining = Duration.milliseconds(250) - (ContinuousClock.now - started)
                    if remaining > .zero { try await Task.sleep(for: remaining) }
                }
                report.append(["scene": scene.name, "frames": count, "verified": scene.contains, "ocr": recognized])
                print("Recorded and verified: \(scene.name)")
            }
            try JSONSerialization.data(withJSONObject: ["fps": fps, "frames": index, "scenes": report], options: [.prettyPrinted, .sortedKeys])
                .write(to: URL(fileURLWithPath: args[4]))
        } catch {
            FileHandle.standardError.write(Data("demo-driver: \(error)\n".utf8))
            exit(1)
        }
    }

    static func post(_ key: String, to pid: pid_t) throws {
        let special: [String: CGKeyCode] = ["enter": 36, "esc": 53, "space": 49, "tab": 48,
                                            "down": 125, "up": 126]
        let codes: [Character: CGKeyCode] = ["a": 0, "s": 1, "d": 2, "f": 3, "h": 4, "g": 5,
            "z": 6, "x": 7, "c": 8, "v": 9, "b": 11, "q": 12, "w": 13, "e": 14, "r": 15,
            "y": 16, "t": 17, "1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23,
            "9": 25, "7": 26, "8": 28, "0": 29, "o": 31, "u": 32, "i": 34, "p": 35,
            "l": 37, "j": 38, "k": 40, ";": 41, ":": 41, "/": 44, "?": 44, "n": 45,
            "m": 46, ",": 43, " ": 49]
        if let code = special[key] {
            try event(code, text: nil, shift: false, pid: pid)
        } else {
            for char in key {
                guard let lower = char.lowercased().first, let code = codes[lower] else {
                    throw DemoError("Unsupported demo key: \(char)")
                }
                try event(code, text: String(char), shift: char.isUppercase || char == ":" || char == "?", pid: pid)
            }
        }
    }

    static func event(_ code: CGKeyCode, text: String?, shift: Bool, pid: pid_t) throws {
        for down in [true, false] {
            guard let event = CGEvent(keyboardEventSource: nil, virtualKey: code, keyDown: down) else {
                throw DemoError("Could not create keyboard event.")
            }
            // Do not inherit modifiers the user happens to be holding.
            event.flags = shift ? .maskShift : []
            if let text {
                let chars = Array(text.utf16)
                chars.withUnsafeBufferPointer { event.keyboardSetUnicodeString(stringLength: chars.count, unicodeString: $0.baseAddress!) }
            }
            event.postToPid(pid)
        }
    }

    static func readText(_ image: CGImage) throws -> String {
        let request = VNRecognizeTextRequest()
        request.recognitionLevel = .accurate
        request.recognitionLanguages = ["en-US"]
        request.usesLanguageCorrection = false
        // Avoid neural-engine initialization stalls in a command-line host.
        // Accurate CPU recognition also distinguishes the terminal font's r/p.
        for (stage, devices) in try request.supportedComputeStageDevices {
            for device in devices {
                if case .cpu = device {
                    request.setComputeDevice(device, for: stage)
                    break
                }
            }
        }
        try VNImageRequestHandler(cgImage: image).perform([request])
        return (request.results ?? []).compactMap { $0.topCandidates(1).first?.string }.joined(separator: "\n")
    }
}
