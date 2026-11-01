import MediaPlayer
import XCTest
@testable import muse

private final class RecordingSink: NowPlayingInfoSink {
    var nowPlayingInfo: [String: Any]?
    var playbackState: MPNowPlayingPlaybackState = .unknown
}

final class SystemNowPlayingTests: XCTestCase {
    private let track = MuseTrack(id: "abc", queueId: 4, title: "So What", artist: "Miles Davis", album: "Kind of Blue", duration: 562)
    private var sink: RecordingSink!
    private var nowPlaying: NowPlaying!
    private var sent: [MuseControlRequest] = []
    private var system: SystemNowPlaying!

    override func setUp() {
        super.setUp()
        sink = RecordingSink()
        nowPlaying = NowPlaying()
        sent = []
        system = SystemNowPlaying(nowPlaying: nowPlaying, sink: sink, commandCenter: nil) { [unowned self] in
            sent.append($0)
        }
    }

    private func connect(mediaControls: Bool? = true) {
        nowPlaying.connectionChanged(true)
        system.refresh()
        let hello = MuseControlEvent.hello(MuseHello(pid: 1, version: "v1", protocol: 1, mediaControls: mediaControls))
        nowPlaying.apply(hello)
        system.apply(hello)
    }

    private func publish(_ state: MusePlaybackState) {
        let event = MuseControlEvent.state(state)
        nowPlaying.apply(event)
        system.apply(event)
    }

    func testPlayingTrackFillsNowPlaying() throws {
        connect()
        publish(MusePlaybackState(playback: .playing, track: track, elapsed: 12.5))
        let info = try XCTUnwrap(sink.nowPlayingInfo)
        XCTAssertEqual(info[MPMediaItemPropertyTitle] as? String, "So What")
        XCTAssertEqual(info[MPMediaItemPropertyArtist] as? String, "Miles Davis")
        XCTAssertEqual(info[MPMediaItemPropertyAlbumTitle] as? String, "Kind of Blue")
        XCTAssertEqual(info[MPMediaItemPropertyPlaybackDuration] as? Double, 562)
        XCTAssertEqual(info[MPNowPlayingInfoPropertyElapsedPlaybackTime] as? Double, 12.5)
        XCTAssertEqual(info[MPNowPlayingInfoPropertyPlaybackRate] as? Double, 1)
        XCTAssertEqual(sink.playbackState, .playing)
    }

    func testPausedTrackStopsTheClock() throws {
        connect()
        publish(MusePlaybackState(playback: .paused, track: track, elapsed: 30))
        XCTAssertEqual(try XCTUnwrap(sink.nowPlayingInfo)[MPNowPlayingInfoPropertyPlaybackRate] as? Double, 0)
        XCTAssertEqual(sink.playbackState, .paused)
    }

    func testStoppedOrEmptyClearsNowPlaying() {
        connect()
        publish(MusePlaybackState(playback: .playing, track: track))
        publish(.stopped)
        XCTAssertNil(sink.nowPlayingInfo)
        XCTAssertEqual(sink.playbackState, .stopped)
    }

    func testDisconnectingClearsNowPlayingAndRefusesCommands() {
        connect()
        publish(MusePlaybackState(playback: .playing, track: track))
        nowPlaying.connectionChanged(false)
        system.refresh()
        XCTAssertNil(sink.nowPlayingInfo)
        XCTAssertEqual(sink.playbackState, .stopped)
        XCTAssertEqual(system.perform(.togglePlayPause), .noActionableNowPlayingItem)
        XCTAssertTrue(sent.isEmpty)
    }

    func testMediaControlsOffInMuseStepsAside() {
        connect(mediaControls: false)
        publish(MusePlaybackState(playback: .playing, track: track))
        XCTAssertNil(sink.nowPlayingInfo)
        XCTAssertEqual(system.perform(.play), .noActionableNowPlayingItem)
        XCTAssertTrue(sent.isEmpty)
    }

    func testAnOlderMuseWithoutTheSettingGetsMediaControls() {
        connect(mediaControls: nil)
        XCTAssertTrue(system.isEnabled)
    }

    func testCommandsSendTheMatchingRequests() {
        connect()
        publish(MusePlaybackState(playback: .playing, track: track))
        XCTAssertEqual(system.perform(.play), .success)
        system.perform(.pause)
        system.perform(.stop)
        system.perform(.togglePlayPause)
        system.perform(.nextTrack)
        system.perform(.previousTrack)
        system.perform(.changePlaybackPosition, position: 90)
        system.perform(.changePlaybackPosition, position: -3)
        XCTAssertEqual(sent, [.play, .pause, .pause, .togglePlay, .next, .previous, .seek(to: 90), .seek(to: 0)])
    }

    func testEveryCommandMapsToARemoteCommand() {
        let center = MPRemoteCommandCenter.shared()
        let commands = SystemMediaCommand.allCases.map { ObjectIdentifier($0.command(in: center)) }
        XCTAssertEqual(Set(commands).count, SystemMediaCommand.allCases.count)
    }
}
