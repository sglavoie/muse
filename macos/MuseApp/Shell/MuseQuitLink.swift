import Foundation

/// The control-socket flag and where muse.app asks muse to serve it.
enum MuseControlLaunch {
    static let flag = "--control"

    /// `$TMPDIR/muse-app-<pid>.sock`: one socket per app process.
    static func socketPath(processID: Int32 = ProcessInfo.processInfo.processIdentifier) -> String {
        FileManager.default.temporaryDirectory.appendingPathComponent("muse-app-\(processID).sock").path
    }

    static func arguments(controlPath: String?) -> [String] {
        controlPath.map { [flag, $0] } ?? []
    }
}

/// The least of the control socket the quit path needs: connect once muse
/// serves it, learn muse's pid from the hello, and notice the EOF that says
/// muse's shutdown finished. It never reconnects on EOF, since on quit that
/// EOF is the answer. Session 7 folds this into the full control client.
final class MuseQuitLink {
    let path: String
    private let lock = NSLock()
    private var fd: Int32 = -1
    private var generation = 0
    private var _helloPID: pid_t?
    private var _reachedEOF = false

    init(path: String) {
        self.path = path
    }

    deinit {
        disconnect()
    }

    /// The pid the current connection's hello named; nil before it arrives.
    var helloPID: pid_t? {
        lock.lock()
        defer { lock.unlock() }
        return _helloPID
    }

    /// Whether the current connection has ended from muse's side.
    var reachedEOF: Bool {
        lock.lock()
        defer { lock.unlock() }
        return _reachedEOF
    }

    var isConnected: Bool {
        lock.lock()
        defer { lock.unlock() }
        return fd >= 0 && !_reachedEOF
    }

    /// Connects in the background, retrying while muse starts up, for at
    /// most `timeout`. A previous connection is dropped first.
    func connect(timeout: TimeInterval = 15, retryInterval: TimeInterval = 0.1) {
        disconnect()
        lock.lock()
        let current = generation
        lock.unlock()
        let path = self.path
        Thread.detachNewThread { [weak self] in
            let deadline = Date().addingTimeInterval(timeout)
            while Date() < deadline {
                guard let self, self.isCurrent(current) else { return }
                if let socket = Self.open(path: path) {
                    self.read(socket, generation: current)
                    return
                }
                Thread.sleep(forTimeInterval: retryInterval)
            }
        }
    }

    /// Drops the connection and forgets what it said.
    func disconnect() {
        lock.lock()
        generation += 1
        let old = fd
        fd = -1
        _helloPID = nil
        _reachedEOF = false
        // Wakes the reader, which closes the descriptor it owns. Under the
        // lock, so the reader cannot close it, and the number be reused, first.
        if old >= 0 {
            shutdown(old, SHUT_RDWR)
        }
        lock.unlock()
    }

    private func isCurrent(_ generation: Int) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return self.generation == generation
    }

    /// Reads lines until EOF, then records it; owns and closes `socket`.
    private func read(_ socket: Int32, generation current: Int) {
        lock.lock()
        guard generation == current else {
            lock.unlock()
            close(socket)
            return
        }
        fd = socket
        lock.unlock()

        var pending = Data()
        var buffer = [UInt8](repeating: 0, count: 4096)
        while true {
            let count = buffer.withUnsafeMutableBytes { recv(socket, $0.baseAddress, $0.count, 0) }
            if count < 0, errno == EINTR { continue }
            guard count > 0 else { break }
            pending.append(contentsOf: buffer[0..<count])
            while let newline = pending.firstIndex(of: UInt8(ascii: "\n")) {
                let line = pending[pending.startIndex..<newline]
                pending = Data(pending[pending.index(after: newline)...])
                if let pid = Self.helloPID(in: Data(line)) {
                    lock.lock()
                    if generation == current { _helloPID = pid }
                    lock.unlock()
                }
            }
        }
        lock.lock()
        if generation == current {
            _reachedEOF = true
            fd = -1
        }
        close(socket)
        lock.unlock()
    }

    /// The pid of a `{"event":"hello","hello":{"pid":N,...}}` line.
    static func helloPID(in line: Data) -> pid_t? {
        guard let object = try? JSONSerialization.jsonObject(with: line) as? [String: Any],
              object["event"] as? String == "hello",
              let hello = object["hello"] as? [String: Any],
              let pid = (hello["pid"] as? NSNumber)?.int32Value, pid > 0 else {
            return nil
        }
        return pid
    }

    private static func open(path: String) -> Int32? {
        let socket = Darwin.socket(AF_UNIX, SOCK_STREAM, 0)
        guard socket >= 0 else { return nil }
        var address = sockaddr_un()
        address.sun_family = sa_family_t(AF_UNIX)
        let capacity = MemoryLayout.size(ofValue: address.sun_path)
        let bytes = Array(path.utf8)
        guard bytes.count < capacity else {
            close(socket)
            return nil
        }
        withUnsafeMutableBytes(of: &address.sun_path) { raw in
            raw.copyBytes(from: bytes)
            raw[bytes.count] = 0
        }
        let length = socklen_t(MemoryLayout<sockaddr_un>.size)
        let connected = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) { Darwin.connect(socket, $0, length) }
        }
        guard connected == 0 else {
            close(socket)
            return nil
        }
        var noSigPipe: Int32 = 1
        setsockopt(socket, SOL_SOCKET, SO_NOSIGPIPE, &noSigPipe, socklen_t(MemoryLayout<Int32>.size))
        return socket
    }
}
