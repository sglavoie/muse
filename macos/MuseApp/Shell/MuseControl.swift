import Combine
import Foundation

// muse's control socket (`muse --control PATH`): newline-delimited JSON in both
// directions. muse.app writes requests and reads one reply per request plus the
// events muse reports unprompted. The wire is `internal/control` in the Go
// tree; these types mirror it. A port of tt.app's TTControl, trimmed to muse's
// vocabulary, with the quit path's needs (the hello pid and the final EOF)
// folded in.

/// The control-socket flag and where muse.app asks muse to serve it.
enum MuseControlLaunch {
    static let flag = "--control"

    /// `$TMPDIR/muse-app-<pid>.sock`: one socket per app process, short since
    /// a Unix socket path is capped near 104 bytes.
    static func socketPath(processID: Int32 = ProcessInfo.processInfo.processIdentifier) -> String {
        FileManager.default.temporaryDirectory.appendingPathComponent("muse-app-\(processID).sock").path
    }

    static func arguments(controlPath: String?) -> [String] {
        controlPath.map { [flag, $0] } ?? []
    }
}

struct MuseControlRequest: Encodable, Equatable {
    enum Op: String, Encodable {
        case togglePlay = "toggle-play"
        case next, previous
        case volumeUp = "volume-up"
        case volumeDown = "volume-down"
        case state
    }

    var id = 0
    let op: Op

    static let togglePlay = Self(op: .togglePlay)
    static let next = Self(op: .next)
    static let previous = Self(op: .previous)
    static let state = Self(op: .state)
}

struct MuseControlReply: Decodable, Equatable {
    let reply: Int
    let ok: Bool
    let message: String?
    /// What a `state` request is answered with.
    let state: MusePlaybackState?

    init(reply: Int, ok: Bool, message: String?, state: MusePlaybackState? = nil) {
        self.reply = reply
        self.ok = ok
        self.message = message
        self.state = state
    }

    /// What the app says when it could not ask: no muse, or a dropped connection.
    static func failure(_ message: String) -> Self {
        Self(reply: 0, ok: false, message: message)
    }
}

/// The muse on the other end of the socket.
struct MuseHello: Decodable, Equatable {
    let pid: Int32
    let version: String
    let `protocol`: Int
}

/// The track muse has loaded. `id` is its content hash and `queueId` its queue
/// row; times are in seconds.
struct MuseTrack: Decodable, Equatable {
    let id: String
    let queueId: Int64
    let title: String
    let artist: String
    let album: String
    let duration: Double

    init(id: String, queueId: Int64 = 0, title: String, artist: String = "", album: String = "", duration: Double = 0) {
        self.id = id
        self.queueId = queueId
        self.title = title
        self.artist = artist
        self.album = album
        self.duration = duration
    }

    enum CodingKeys: String, CodingKey { case id, queueId, title, artist, album, duration }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? ""
        queueId = try c.decodeIfPresent(Int64.self, forKey: .queueId) ?? 0
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        artist = try c.decodeIfPresent(String.self, forKey: .artist) ?? ""
        album = try c.decodeIfPresent(String.self, forKey: .album) ?? ""
        duration = try c.decodeIfPresent(Double.self, forKey: .duration) ?? 0
    }
}

/// What is playing. `track` is nil when nothing is loaded.
struct MusePlaybackState: Decodable, Equatable {
    enum Playback: String, Decodable {
        case playing, paused, stopped

        /// A later muse's new value reads as stopped rather than failing the line.
        init(from decoder: Decoder) throws {
            self = Self(rawValue: try decoder.singleValueContainer().decode(String.self)) ?? .stopped
        }
    }

    let playback: Playback
    let track: MuseTrack?
    let elapsed: Double

    init(playback: Playback, track: MuseTrack? = nil, elapsed: Double = 0) {
        self.playback = playback
        self.track = track
        self.elapsed = elapsed
    }

    enum CodingKeys: String, CodingKey { case playback, track, elapsed }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        playback = try c.decodeIfPresent(Playback.self, forKey: .playback) ?? .stopped
        track = try c.decodeIfPresent(MuseTrack.self, forKey: .track)
        elapsed = try c.decodeIfPresent(Double.self, forKey: .elapsed) ?? 0
    }

    static let stopped = Self(playback: .stopped)
}

/// One banner muse asks the app to show; muse's `notify_on_track_change`
/// already decided it should exist.
struct MuseNotification: Decodable, Equatable {
    let title: String
    let subtitle: String?
    let body: String?
}

enum MuseControlEvent: Equatable {
    case hello(MuseHello)
    case state(MusePlaybackState)
    case notify(MuseNotification)
    /// muse began shutting down; the EOF that follows means it finished.
    case quitting
}

/// One line muse wrote: a reply or an event. Anything else is skipped, so a
/// later muse adding events does not break an older app.
enum MuseControlLine: Equatable {
    case reply(MuseControlReply)
    case event(MuseControlEvent)

    private struct Raw: Decodable {
        let reply: Int?
        let ok: Bool?
        let message: String?
        let event: String?
        let hello: MuseHello?
        let state: MusePlaybackState?
        let notify: MuseNotification?
    }

    static func decode(_ data: Data) -> MuseControlLine? {
        guard let raw = try? JSONDecoder().decode(Raw.self, from: data) else { return nil }
        if let id = raw.reply {
            return .reply(MuseControlReply(reply: id, ok: raw.ok ?? false, message: raw.message, state: raw.state))
        }
        switch raw.event {
        case "hello":
            return raw.hello.map { .event(.hello($0)) }
        case "state":
            return raw.state.map { .event(.state($0)) }
        case "notify":
            return raw.notify.map { .event(.notify($0)) }
        case "quitting":
            return .event(.quitting)
        default:
            return nil
        }
    }
}

/// Splits a byte stream into lines, keeping a partial last line for the next chunk.
struct MuseLineBuffer {
    private var pending = Data()

    mutating func append(_ chunk: Data) -> [Data] {
        pending.append(chunk)
        var lines: [Data] = []
        while let newline = pending.firstIndex(of: UInt8(ascii: "\n")) {
            let line = pending[pending.startIndex..<newline]
            if !line.isEmpty { lines.append(Data(line)) }
            pending.removeSubrange(pending.startIndex...newline)
        }
        return lines
    }
}

/// What muse is playing, as the control socket last said; the status item and
/// its menu read it. Updated on the main queue.
final class NowPlaying: ObservableObject {
    /// Whether a control connection is live.
    @Published var isConnected = false
    @Published var state = MusePlaybackState.stopped

    init(isConnected: Bool = false, state: MusePlaybackState = .stopped) {
        self.isConnected = isConnected
        self.state = state
    }

    var track: MuseTrack? { isConnected ? state.track : nil }
    var isPlaying: Bool { isConnected && state.playback == .playing }

    /// `Title — Artist`, or the title alone; nil with nothing loaded.
    var headline: String? {
        guard let track else { return nil }
        let title = track.title.isEmpty ? "Untitled" : track.title
        return track.artist.isEmpty ? title : "\(title) — \(track.artist)"
    }

    var album: String? {
        guard let album = track?.album, !album.isEmpty else { return nil }
        return album
    }

    /// The status item's tooltip: the current track, else the app's name.
    var toolTip: String { headline ?? "muse" }

    func apply(_ event: MuseControlEvent) {
        if case .state(let state) = event {
            self.state = state
        }
    }

    func connectionChanged(_ connected: Bool) {
        isConnected = connected
        if !connected { state = .stopped }
    }
}

/// The app's one connection to the running muse. It connects once muse has
/// created the socket, retrying while muse starts, and drops every pending
/// request when the connection ends. A connection muse ends without saying it
/// is quitting (a relaunch, a crash) is followed by a fresh round of retries.
/// Callbacks run on the main queue; `helloPID` and `reachedEOF` can be read
/// from any thread, for the quit path.
final class MuseControlClient {
    let path: String
    var onEvent: ((MuseControlEvent) -> Void)?
    /// Whether a connection is live; called on every change.
    var onConnectionChange: ((Bool) -> Void)?

    private let queue = DispatchQueue(label: "io.github.sglavoie.muse.control")
    private var fd: Int32 = -1
    private var readSource: DispatchSourceRead?
    private var buffer = MuseLineBuffer()
    private var nextID = 1
    private var pending: [Int: (MuseControlReply) -> Void] = [:]
    /// Requests sent while muse is still starting, written once it connects.
    private var queued: [(MuseControlRequest, (MuseControlReply) -> Void)] = []
    private var connecting = false
    /// Whether muse said it is quitting on the current connection.
    private var quitting = false
    /// Bumped by each connect and disconnect, so a retry scheduled for an
    /// earlier session gives up.
    private var generation = 0

    /// How long to keep trying while muse starts, and how often.
    var connectWindow: TimeInterval = 30
    var retryInterval: TimeInterval = 0.25

    private let lock = NSLock()
    private var _helloPID: pid_t?
    private var _reachedEOF = false

    init(path: String) {
        self.path = path
    }

    deinit {
        if fd >= 0 { close(fd) }
    }

    /// The pid the current connection's hello named; nil before it arrives.
    var helloPID: pid_t? {
        lock.lock()
        defer { lock.unlock() }
        return _helloPID
    }

    /// Whether the last connection ended from muse's side, with no newer one since.
    var reachedEOF: Bool {
        lock.lock()
        defer { lock.unlock() }
        return _reachedEOF
    }

    /// Whether a connection is live right now; read on the main queue.
    private(set) var isConnected = false

    /// Starts connecting, replacing any connection there was.
    func connect() {
        resetSnapshot()
        queue.async { [self] in
            closeLocked(reason: "muse restarted")
            resetSnapshot()
            generation += 1
            connecting = true
            attempt(generation: generation, deadline: Date().addingTimeInterval(connectWindow))
        }
    }

    /// Drops the connection and forgets what it said.
    func disconnect() {
        resetSnapshot()
        queue.async { [self] in
            generation += 1
            connecting = false
            closeLocked(reason: "muse is not running")
            resetSnapshot()
            failQueued("muse is not running")
        }
    }

    /// Sends a request; `completion` runs on the main queue with muse's reply,
    /// or a failure when there is no muse to ask. A request sent while muse is
    /// still starting waits for it.
    func send(_ request: MuseControlRequest, completion: @escaping (MuseControlReply) -> Void = { _ in }) {
        queue.async { [self] in
            if fd < 0, connecting {
                queued.append((request, completion))
                return
            }
            transmit(request, completion: completion)
        }
    }

    private func resetSnapshot() {
        lock.lock()
        _helloPID = nil
        _reachedEOF = false
        lock.unlock()
    }

    // MARK: On the queue

    private func transmit(_ request: MuseControlRequest, completion: @escaping (MuseControlReply) -> Void) {
        guard fd >= 0 else {
            DispatchQueue.main.async { completion(.failure("muse is not running")) }
            return
        }
        var request = request
        request.id = nextID
        nextID += 1
        guard var line = try? JSONEncoder().encode(request) else {
            DispatchQueue.main.async { completion(.failure("The request could not be encoded")) }
            return
        }
        line.append(UInt8(ascii: "\n"))
        pending[request.id] = completion
        if !writeAll(line) {
            closeLocked(reason: "The connection to muse ended")
        }
    }

    private func failQueued(_ reason: String) {
        let failed = queued
        queued = []
        DispatchQueue.main.async {
            for (_, completion) in failed { completion(.failure(reason)) }
        }
    }

    private func attempt(generation: Int, deadline: Date) {
        guard generation == self.generation, fd < 0 else { return }
        if let socket = Self.open(path: path) {
            fd = socket
            connecting = false
            quitting = false
            resetSnapshot()
            startReading()
            let waiting = queued
            queued = []
            for (request, completion) in waiting { transmit(request, completion: completion) }
            DispatchQueue.main.async { [self] in
                isConnected = true
                onConnectionChange?(true)
            }
            return
        }
        guard Date() < deadline else {
            NSLog("muse: control: gave up connecting to %@", path)
            connecting = false
            failQueued("muse did not start its control socket")
            return
        }
        queue.asyncAfter(deadline: .now() + retryInterval) { [self] in
            attempt(generation: generation, deadline: deadline)
        }
    }

    private static func open(path: String) -> Int32? {
        let socket = Darwin.socket(AF_UNIX, SOCK_STREAM, 0)
        guard socket >= 0 else { return nil }
        var on: Int32 = 1
        setsockopt(socket, SOL_SOCKET, SO_NOSIGPIPE, &on, socklen_t(MemoryLayout<Int32>.size))
        var address = sockaddr_un()
        address.sun_family = sa_family_t(AF_UNIX)
        let bytes = Array(path.utf8)
        guard bytes.count < MemoryLayout.size(ofValue: address.sun_path) else {
            close(socket)
            return nil
        }
        withUnsafeMutableBytes(of: &address.sun_path) { raw in
            raw.copyBytes(from: bytes)
            raw[bytes.count] = 0
        }
        let connected = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.connect(socket, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        guard connected == 0 else {
            close(socket)
            return nil
        }
        return socket
    }

    private func startReading() {
        let source = DispatchSource.makeReadSource(fileDescriptor: fd, queue: queue)
        source.setEventHandler { [weak self] in self?.readAvailable() }
        readSource = source
        source.resume()
    }

    private func readAvailable() {
        var chunk = [UInt8](repeating: 0, count: 16 * 1024)
        let count = Darwin.read(fd, &chunk, chunk.count)
        if count < 0, errno == EINTR || errno == EAGAIN { return }
        guard count > 0 else {
            let saidQuitting = quitting
            closeLocked(reason: "The connection to muse ended")
            lock.lock()
            _reachedEOF = true
            lock.unlock()
            // On quit that EOF is the answer. Otherwise muse went away on its
            // own, a relaunch or a crash: keep looking for the next one until
            // disconnect() says none is coming.
            guard !saidQuitting else { return }
            connecting = true
            attempt(generation: generation, deadline: Date().addingTimeInterval(connectWindow))
            return
        }
        for line in buffer.append(Data(chunk[0..<count])) {
            guard let decoded = MuseControlLine.decode(line) else { continue }
            switch decoded {
            case .reply(let reply):
                if let completion = pending.removeValue(forKey: reply.reply) {
                    DispatchQueue.main.async { completion(reply) }
                }
            case .event(let event):
                switch event {
                case .hello(let hello) where hello.pid > 0:
                    lock.lock()
                    _helloPID = hello.pid
                    lock.unlock()
                case .quitting:
                    quitting = true
                default:
                    break
                }
                DispatchQueue.main.async { [weak self] in self?.onEvent?(event) }
            }
        }
    }

    private func writeAll(_ data: Data) -> Bool {
        data.withUnsafeBytes { raw in
            var offset = 0
            while offset < raw.count {
                let written = Darwin.write(fd, raw.baseAddress!.advanced(by: offset), raw.count - offset)
                if written <= 0 { return false }
                offset += written
            }
            return true
        }
    }

    private func closeLocked(reason: String) {
        let wasOpen = fd >= 0
        if let readSource {
            // The cancel handler is not used: closing here is synchronous on the queue.
            readSource.cancel()
            self.readSource = nil
        }
        if fd >= 0 {
            close(fd)
            fd = -1
        }
        buffer = MuseLineBuffer()
        let failed = pending
        pending = [:]
        DispatchQueue.main.async { [self] in
            for completion in failed.values { completion(.failure(reason)) }
            if wasOpen {
                isConnected = false
                onConnectionChange?(false)
            }
        }
    }
}
