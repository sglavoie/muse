import Foundation

/// What the hidden `muse build-info` reports: one line of
/// `version=V revision=R time=T`, with a value muse cannot know left empty.
struct MuseVersion: Equatable {
    let version: String
    /// The `time=` instant; nil for a build that records none.
    let built: Date?

    static func parse(_ output: String) -> MuseVersion? {
        guard let line = output.split(whereSeparator: \.isNewline).first.map(String.init),
              line.hasPrefix("version=") else {
            return nil
        }
        var fields: [String: String] = [:]
        for pair in line.split(separator: " ") {
            guard let equals = pair.firstIndex(of: "=") else { continue }
            fields[String(pair[..<equals])] = String(pair[pair.index(after: equals)...])
        }
        return MuseVersion(version: fields["version"] ?? "", built: fields["time"].flatMap(date(from:)))
    }

    /// An RFC 3339 time, with or without fractional seconds.
    static func date(from stamp: String) -> Date? {
        guard !stamp.isEmpty else { return nil }
        let plain = ISO8601DateFormatter()
        if let date = plain.date(from: stamp) { return date }
        let fractional = ISO8601DateFormatter()
        fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return fractional.date(from: stamp)
    }

    /// Runs `binary build-info`; nil when it fails, prints something else, or
    /// takes longer than `timeout`. An older muse with no `build-info` fails.
    static func read(binary: String, timeout: TimeInterval = 2) -> MuseVersion? {
        output(of: binary, arguments: ["build-info"], timeout: timeout).flatMap(parse)
    }

    /// Whether `binary --help` lists `flag`, so muse.app passes a flag only to
    /// a muse that knows it: an older one refuses an unknown flag and never starts.
    static func supports(flag: String, binary: String, timeout: TimeInterval = 2) -> Bool {
        supported(flags: [flag], binary: binary, timeout: timeout).contains(flag)
    }

    /// Which of `flags` `binary --help` lists, from one run of it.
    static func supported(flags: [String], binary: String, timeout: TimeInterval = 2) -> Set<String> {
        guard let help = output(of: binary, arguments: ["--help"], timeout: timeout) else { return [] }
        return Set(flags.filter { helpLists(flag: $0, in: help) })
    }

    /// Whether a help text declares `flag` as an option of its own, as the
    /// first token of a line, rather than mentioning it in prose.
    static func helpLists(flag: String, in help: String) -> Bool {
        help.split(whereSeparator: \.isNewline).contains { line in
            line.trimmingCharacters(in: .whitespaces).split(separator: " ").first.map(String.init) == flag
        }
    }

    /// Runs `binary` with `arguments` and returns its standard output; nil
    /// when it fails to start, exits non-zero, or outlasts `timeout`.
    private static func output(of binary: String, arguments: [String], timeout: TimeInterval) -> String? {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: binary)
        process.arguments = arguments
        let stdout = Pipe()
        process.standardOutput = stdout
        process.standardError = FileHandle.nullDevice
        process.standardInput = FileHandle.nullDevice
        do {
            try process.run()
        } catch {
            return nil
        }
        let done = DispatchSemaphore(value: 0)
        let box = OutputBox()
        DispatchQueue.global().async {
            box.data = stdout.fileHandleForReading.readDataToEndOfFile()
            done.signal()
        }
        if done.wait(timeout: .now() + timeout) == .timedOut {
            process.terminate()
            return nil
        }
        process.waitUntilExit()
        guard process.terminationStatus == 0 else { return nil }
        return String(decoding: box.data, as: UTF8.self)
    }

    private final class OutputBox {
        var data = Data()
    }
}
