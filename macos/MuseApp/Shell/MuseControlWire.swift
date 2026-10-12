import Foundation

// Wire types for the muse control socket; see MuseControl.swift.

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
