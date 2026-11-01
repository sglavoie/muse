import MediaPlayer

/// Where Now Playing goes: `MPNowPlayingInfoCenter` in the app, a recorder in
/// tests.
protocol NowPlayingInfoSink: AnyObject {
    var nowPlayingInfo: [String: Any]? { get set }
    var playbackState: MPNowPlayingPlaybackState { get set }
}

extension MPNowPlayingInfoCenter: NowPlayingInfoSink {}

/// The system media commands muse answers, and the request each one sends.
enum SystemMediaCommand: CaseIterable {
    case play, pause, togglePlayPause, stop, nextTrack, previousTrack, changePlaybackPosition

    /// The request this command sends; `position` is the seek target in
    /// seconds, used by `changePlaybackPosition` alone.
    func request(position: Double = 0) -> MuseControlRequest {
        switch self {
        case .play: return .play
        // muse keeps its place in the queue, so stop pauses.
        case .pause, .stop: return .pause
        case .togglePlayPause: return .togglePlay
        case .nextTrack: return .next
        case .previousTrack: return .previous
        case .changePlaybackPosition: return .seek(to: max(0, position))
        }
    }

    func command(in center: MPRemoteCommandCenter) -> MPRemoteCommand {
        switch self {
        case .play: return center.playCommand
        case .pause: return center.pauseCommand
        case .togglePlayPause: return center.togglePlayPauseCommand
        case .stop: return center.stopCommand
        case .nextTrack: return center.nextTrackCommand
        case .previousTrack: return center.previousTrackCommand
        case .changePlaybackPosition: return center.changePlaybackPositionCommand
        }
    }
}

/// Publishes what muse plays to macOS's Now Playing (Control Center, the menu
/// bar's Now Playing item, the lock screen) and routes media keys, headphone
/// buttons and the Now Playing controls to muse. muse plays through mpv, a
/// separate process, so without this the system sees no player at all.
///
/// It mirrors `NowPlaying`, which the app delegate updates first and then calls
/// `apply(_:)` or `refresh()`, so it shows a track only while the control
/// socket is connected, and it steps aside (clearing Now Playing and releasing the
/// commands, so media keys reach other apps) whenever muse's `media_controls`
/// setting is off or muse is gone.
final class SystemNowPlaying {
    private let nowPlaying: NowPlaying
    private let send: (MuseControlRequest) -> Void
    private let sink: NowPlayingInfoSink
    private let commandCenter: MPRemoteCommandCenter?
    private var targets: [(MPRemoteCommand, Any)] = []

    /// Whether muse wants its playback in the system's media controls; the
    /// hello on each connection sets it.
    var isEnabled = false {
        didSet {
            guard isEnabled != oldValue else { return }
            refresh()
        }
    }

    /// `commandCenter` is nil in tests, which drive commands through
    /// `perform(_:position:)` instead of the shared center.
    init(
        nowPlaying: NowPlaying,
        sink: NowPlayingInfoSink = MPNowPlayingInfoCenter.default(),
        commandCenter: MPRemoteCommandCenter? = .shared(),
        send: @escaping (MuseControlRequest) -> Void
    ) {
        self.nowPlaying = nowPlaying
        self.sink = sink
        self.commandCenter = commandCenter
        self.send = send
    }

    deinit {
        releaseCommands()
    }

    /// Applies a control event once `NowPlaying` has: a hello says whether
    /// muse wants media controls, and a state moves Now Playing.
    func apply(_ event: MuseControlEvent) {
        if case .hello(let hello) = event {
            isEnabled = hello.mediaControls ?? true
        }
        refresh()
    }

    /// Whether a command would reach a muse right now.
    var isActive: Bool { isEnabled && nowPlaying.isConnected }

    /// Sends the request for `command`, as a media key would.
    @discardableResult
    func perform(_ command: SystemMediaCommand, position: Double = 0) -> MPRemoteCommandHandlerStatus {
        guard isActive else { return .noActionableNowPlayingItem }
        send(command.request(position: position))
        return .success
    }

    /// Mirrors the current state into Now Playing and claims or releases the
    /// media commands to match.
    func refresh() {
        guard isActive else {
            releaseCommands()
            sink.nowPlayingInfo = nil
            sink.playbackState = .stopped
            return
        }
        claimCommands()
        let (info, playback) = Self.nowPlayingInfo(for: nowPlaying.state)
        sink.nowPlayingInfo = info
        sink.playbackState = playback
        commandCenter?.changePlaybackPositionCommand.isEnabled = info?[MPMediaItemPropertyPlaybackDuration] != nil
    }

    /// The Now Playing dictionary for a state: nil with nothing loaded.
    static func nowPlayingInfo(for state: MusePlaybackState) -> ([String: Any]?, MPNowPlayingPlaybackState) {
        let playback: MPNowPlayingPlaybackState
        switch state.playback {
        case .playing: playback = .playing
        case .paused: playback = .paused
        case .stopped: playback = .stopped
        }
        guard let track = state.track, state.playback != .stopped else { return (nil, playback) }
        var info: [String: Any] = [
            MPMediaItemPropertyTitle: track.title.isEmpty ? "Untitled" : track.title,
            MPNowPlayingInfoPropertyMediaType: MPNowPlayingInfoMediaType.audio.rawValue,
            MPNowPlayingInfoPropertyElapsedPlaybackTime: max(0, state.elapsed),
            MPNowPlayingInfoPropertyPlaybackRate: state.playback == .playing ? 1.0 : 0.0,
            MPNowPlayingInfoPropertyDefaultPlaybackRate: 1.0,
            MPNowPlayingInfoPropertyExternalContentIdentifier: "\(track.id)#\(track.queueId)",
        ]
        if !track.artist.isEmpty { info[MPMediaItemPropertyArtist] = track.artist }
        if !track.album.isEmpty { info[MPMediaItemPropertyAlbumTitle] = track.album }
        if track.duration > 0 { info[MPMediaItemPropertyPlaybackDuration] = track.duration }
        return (info, playback)
    }

    private func claimCommands() {
        guard let center = commandCenter, targets.isEmpty else { return }
        for media in SystemMediaCommand.allCases {
            let command = media.command(in: center)
            command.isEnabled = true
            let target = command.addTarget { [weak self] event in
                let position = (event as? MPChangePlaybackPositionCommandEvent)?.positionTime ?? 0
                return self?.perform(media, position: position) ?? .commandFailed
            }
            targets.append((command, target))
        }
    }

    private func releaseCommands() {
        for (command, target) in targets {
            command.removeTarget(target)
            command.isEnabled = false
        }
        targets.removeAll()
    }
}
