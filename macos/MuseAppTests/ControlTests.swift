import AppKit
import UserNotifications
import XCTest
@testable import muse

/// The wire is `internal/control` in the Go tree; these pin the lines muse.app
/// writes and reads.
final class ControlWireTests: XCTestCase {
    func testEachEventDecodes() {
        XCTAssertEqual(
            MuseControlLine.decode(Data(#"{"event":"hello","hello":{"pid":77,"version":"v1.2.0","protocol":1}}"#.utf8)),
            .event(.hello(MuseHello(pid: 77, version: "v1.2.0", protocol: 1)))
        )
        let state = #"{"event":"state","state":{"playback":"playing","elapsed":12.5,"track":{"id":"abc","queueId":4,"title":"So What","artist":"Miles Davis","album":"Kind of Blue","duration":562}}}"#
        XCTAssertEqual(
            MuseControlLine.decode(Data(state.utf8)),
            .event(.state(MusePlaybackState(
                playback: .playing,
                track: MuseTrack(id: "abc", queueId: 4, title: "So What", artist: "Miles Davis", album: "Kind of Blue", duration: 562),
                elapsed: 12.5
            )))
        )
        // Nothing loaded: the track is absent.
        XCTAssertEqual(
            MuseControlLine.decode(Data(#"{"event":"state","state":{"playback":"stopped","elapsed":0}}"#.utf8)),
            .event(.state(MusePlaybackState(playback: .stopped)))
        )
        XCTAssertEqual(
            MuseControlLine.decode(Data(#"{"event":"notify","notify":{"title":"So What","subtitle":"Miles Davis","body":"Kind of Blue"}}"#.utf8)),
            .event(.notify(MuseNotification(title: "So What", subtitle: "Miles Davis", body: "Kind of Blue")))
        )
        XCTAssertEqual(MuseControlLine.decode(Data(#"{"event":"quitting"}"#.utf8)), .event(.quitting))
    }

    func testRepliesDecodeWithAndWithoutState() {
        XCTAssertEqual(
            MuseControlLine.decode(Data(#"{"reply":3,"ok":true,"message":"Paused"}"#.utf8)),
            .reply(MuseControlReply(reply: 3, ok: true, message: "Paused"))
        )
        XCTAssertEqual(
            MuseControlLine.decode(Data(#"{"reply":4,"ok":true,"state":{"playback":"paused","elapsed":1}}"#.utf8)),
            .reply(MuseControlReply(reply: 4, ok: true, message: nil, state: MusePlaybackState(playback: .paused, elapsed: 1)))
        )
        XCTAssertEqual(
            MuseControlLine.decode(Data(#"{"reply":5,"ok":false,"message":"muse is quitting"}"#.utf8)),
            .reply(MuseControlReply(reply: 5, ok: false, message: "muse is quitting"))
        )
    }

    /// A later muse may add events or playback values; an older app skips or
    /// tolerates what it does not know.
    func testUnknownLinesAreSkipped() {
        XCTAssertNil(MuseControlLine.decode(Data(#"{"event":"something-new"}"#.utf8)))
        XCTAssertNil(MuseControlLine.decode(Data(#"{"event":"hello"}"#.utf8)))
        XCTAssertNil(MuseControlLine.decode(Data("not json".utf8)))
        XCTAssertEqual(
            MuseControlLine.decode(Data(#"{"event":"state","state":{"playback":"buffering"}}"#.utf8)),
            .event(.state(MusePlaybackState(playback: .stopped)))
        )
    }

    func testRequestsEncodeTheGoSpelling() throws {
        func object(_ request: MuseControlRequest) throws -> [String: Any] {
            try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(request)) as? [String: Any])
        }
        var request = MuseControlRequest.togglePlay
        request.id = 9
        XCTAssertEqual(try object(request)["op"] as? String, "toggle-play")
        XCTAssertEqual(try object(request)["id"] as? Int, 9)
        XCTAssertEqual(try object(.next)["op"] as? String, "next")
        XCTAssertEqual(try object(.previous)["op"] as? String, "previous")
        XCTAssertEqual(try object(.state)["op"] as? String, "state")
        XCTAssertEqual(try object(MuseControlRequest(op: .volumeUp))["op"] as? String, "volume-up")
    }

    func testLineBufferKeepsAPartialLine() {
        var buffer = MuseLineBuffer()
        XCTAssertEqual(buffer.append(Data("{\"a\":1}\n{\"b\"".utf8)), [Data("{\"a\":1}".utf8)])
        XCTAssertEqual(buffer.append(Data(":2}\n\n".utf8)), [Data("{\"b\":2}".utf8)])
        XCTAssertEqual(buffer.append(Data()), [])
    }

    func testControlArgumentsOnlyWhenSupported() {
        XCTAssertEqual(MuseControlLaunch.arguments(controlPath: "/s.sock"), ["--control", "/s.sock"])
        XCTAssertEqual(MuseControlLaunch.arguments(controlPath: nil), [])
        XCTAssertTrue(MuseControlLaunch.socketPath(processID: 12).hasSuffix("/muse-app-12.sock"))
        XCTAssertLessThan(MuseControlLaunch.socketPath(processID: 99999).utf8.count, 104)
    }
}

final class NowPlayingTests: XCTestCase {
    static let track = MuseTrack(id: "abc", title: "So What", artist: "Miles Davis", album: "Kind of Blue")

    func testStateEventsUpdateItAndADroppedConnectionClearsIt() {
        let model = NowPlaying()
        XCTAssertNil(model.headline)
        XCTAssertEqual(model.toolTip, "muse")

        model.connectionChanged(true)
        model.apply(.state(MusePlaybackState(playback: .playing, track: Self.track)))
        XCTAssertTrue(model.isPlaying)
        XCTAssertEqual(model.headline, "So What — Miles Davis")
        XCTAssertEqual(model.album, "Kind of Blue")
        XCTAssertEqual(model.toolTip, "So What — Miles Davis")

        // Other events leave the state alone.
        model.apply(.quitting)
        model.apply(.notify(MuseNotification(title: "x", subtitle: nil, body: nil)))
        XCTAssertTrue(model.isPlaying)

        model.connectionChanged(false)
        XCTAssertFalse(model.isPlaying)
        XCTAssertNil(model.headline)
        XCTAssertEqual(model.state, .stopped)
    }

    func testHeadlineWithoutArtistOrTitle() {
        let model = NowPlaying(isConnected: true, state: MusePlaybackState(
            playback: .paused, track: MuseTrack(id: "x", title: "Intro")
        ))
        XCTAssertEqual(model.headline, "Intro")
        XCTAssertNil(model.album)
        XCTAssertFalse(model.isPlaying)

        model.state = MusePlaybackState(playback: .paused, track: MuseTrack(id: "x", title: "", artist: "Someone"))
        XCTAssertEqual(model.headline, "Untitled — Someone")
    }
}

private final class MiniControlsDelegate: StatusItemDelegate {
    var isTerminalFrontmost = false
    var canRelaunchTerminal = true
    var isLaunchAtLoginEnabled = false
    var calls: [String] = []
    func toggleTerminal() { calls.append("toggle") }
    func relaunchTerminal() { calls.append("relaunch") }
    func showPreferences(_ sender: Any?) { calls.append("preferences") }
    func toggleLaunchAtLogin() { calls.append("login") }
    func togglePlayback() { calls.append("play-pause") }
    func playNext() { calls.append("next") }
    func playPrevious() { calls.append("previous") }
}

/// The status item's menu, built from a NowPlaying fixture.
final class MiniControlsMenuTests: XCTestCase {
    private func visibleTitles(_ controller: StatusItemController) -> [String] {
        controller.menu.items.filter { !$0.isSeparatorItem && !$0.isHidden }.map(\.title)
    }

    func testPlayingTrackHeadsTheMenuAndControlsFollowTheState() throws {
        let delegate = MiniControlsDelegate()
        let nowPlaying = NowPlaying(isConnected: true, state: MusePlaybackState(playback: .playing, track: NowPlayingTests.track))
        let controller = StatusItemController(delegate: delegate, nowPlaying: nowPlaying)
        controller.refreshMenu()

        XCTAssertEqual(visibleTitles(controller), [
            "So What — Miles Davis", "Kind of Blue", "Pause", "Next", "Previous",
            "Show muse", "Relaunch muse", "Preferences…", "Launch at Login", "Quit muse",
        ])
        XCTAssertFalse(controller.menu.items[0].isEnabled, "the header is information, not an action")
        XCTAssertFalse(controller.menu.items[1].isEnabled)
        XCTAssertEqual(controller.statusItem.button?.toolTip, "So What — Miles Davis")
        for title in ["Pause", "Next", "Previous"] {
            let item = try XCTUnwrap(controller.menu.item(withTitle: title))
            XCTAssertTrue(item.isEnabled, title)
            _ = (item.target as? NSObject)?.perform(item.action, with: item)
        }
        XCTAssertEqual(delegate.calls, ["play-pause", "next", "previous"])

        nowPlaying.state = MusePlaybackState(playback: .paused, track: NowPlayingTests.track)
        controller.refreshMenu()
        XCTAssertNotNil(controller.menu.item(withTitle: "Play"))
        XCTAssertNil(controller.menu.item(withTitle: "Pause"))
    }

    func testWithNoConnectionTheMenuSaysNotPlayingAndControlsAreDisabled() throws {
        let nowPlaying = NowPlaying(isConnected: false, state: MusePlaybackState(playback: .playing, track: NowPlayingTests.track))
        let controller = StatusItemController(delegate: MiniControlsDelegate(), nowPlaying: nowPlaying)
        controller.refreshMenu()
        XCTAssertEqual(Array(visibleTitles(controller).prefix(4)), ["Not playing", "Play", "Next", "Previous"])
        for title in ["Play", "Next", "Previous"] {
            XCTAssertFalse(try XCTUnwrap(controller.menu.item(withTitle: title)).isEnabled, title)
        }
        XCTAssertEqual(controller.statusItem.button?.toolTip, "muse")

        // Connected but with nothing loaded: still "Not playing", but the controls act.
        nowPlaying.connectionChanged(true)
        nowPlaying.apply(.state(.stopped))
        controller.refreshMenu()
        XCTAssertEqual(visibleTitles(controller).first, "Not playing")
        XCTAssertTrue(try XCTUnwrap(controller.menu.item(withTitle: "Play")).isEnabled)
    }

    /// A state change reaches the tooltip without the menu being opened.
    func testTheTooltipFollowsTheModel() {
        let nowPlaying = NowPlaying()
        let controller = StatusItemController(delegate: MiniControlsDelegate(), nowPlaying: nowPlaying)
        nowPlaying.connectionChanged(true)
        nowPlaying.apply(.state(MusePlaybackState(playback: .playing, track: NowPlayingTests.track)))
        let updated = expectation(description: "tooltip")
        DispatchQueue.main.async {
            XCTAssertEqual(controller.statusItem.button?.toolTip, "So What — Miles Davis")
            updated.fulfill()
        }
        wait(for: [updated], timeout: 2)
    }
}

final class MuseNotificationsTests: XCTestCase {
    func testABannerIsTitleArtistAndAlbum() {
        let content = MuseNotifications.content(for: MuseNotification(title: "So What", subtitle: "Miles Davis", body: "Kind of Blue"))
        XCTAssertEqual(content.title, "So What")
        XCTAssertEqual(content.subtitle, "Miles Davis")
        XCTAssertEqual(content.body, "Kind of Blue")
        XCTAssertEqual(MuseNotifications.content(for: MuseNotification(title: "", subtitle: nil, body: nil)).title, "muse")
    }

    /// The app's toggle can only suppress, and muse in front needs no banner.
    func testPostingNeedsTheToggleOnAndMuseNotInFront() {
        XCTAssertTrue(MuseNotifications.shouldPost(enabled: true, museInFront: false))
        XCTAssertFalse(MuseNotifications.shouldPost(enabled: false, museInFront: false))
        XCTAssertFalse(MuseNotifications.shouldPost(enabled: true, museInFront: true))
    }

    func testClickingABannerShowsMuse() {
        let notifications = MuseNotifications()
        var shown = 0
        notifications.onActivate = { shown += 1 }
        notifications.respond(to: UNNotificationDefaultActionIdentifier)
        notifications.respond(to: UNNotificationDismissActionIdentifier)
        XCTAssertEqual(shown, 1)
    }

    /// A suppressed banner never reaches the notification center, so the
    /// user is never asked for authorization by it.
    func testASuppressedBannerIsDropped() {
        let notifications = MuseNotifications()
        var asked = 0
        notifications.isEnabled = { asked += 1; return false }
        notifications.post(MuseNotification(title: "x", subtitle: nil, body: nil))
        XCTAssertEqual(asked, 1)
    }
}

/// The client against a stand-in muse that accepts connections, reads the
/// request lines, and writes what each test scripts.
final class ControlClientTests: XCTestCase {
    private var path = ""
    private var listener: Int32 = -1
    private var connections: [Int32] = []

    override func setUpWithError() throws {
        path = "/tmp/musec-\(UUID().uuidString.prefix(8)).sock"
        listener = socket(AF_UNIX, SOCK_STREAM, 0)
        var address = sockaddr_un()
        address.sun_family = sa_family_t(AF_UNIX)
        withUnsafeMutableBytes(of: &address.sun_path) { raw in
            let bytes = Array(path.utf8)
            raw.copyBytes(from: bytes)
            raw[bytes.count] = 0
        }
        let bound = withUnsafePointer(to: &address) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.bind(listener, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        XCTAssertEqual(bound, 0)
        XCTAssertEqual(Darwin.listen(listener, 4), 0)
    }

    override func tearDown() {
        for connection in connections { close(connection) }
        close(listener)
        unlink(path)
    }

    /// The next connection the client makes, waiting up to `timeout`.
    private func accept(timeout: TimeInterval = 5) throws -> Int32 {
        var poll = pollfd(fd: listener, events: Int16(POLLIN), revents: 0)
        guard Darwin.poll(&poll, 1, Int32(timeout * 1000)) == 1 else { throw Timeout() }
        let connection = Darwin.accept(listener, nil, nil)
        XCTAssertGreaterThanOrEqual(connection, 0)
        var seconds = timeval(tv_sec: Int(timeout), tv_usec: 0)
        setsockopt(connection, SOL_SOCKET, SO_RCVTIMEO, &seconds, socklen_t(MemoryLayout<timeval>.size))
        connections.append(connection)
        return connection
    }

    private func write(_ line: String, to connection: Int32) {
        let text = line + "\n"
        _ = text.withCString { Darwin.write(connection, $0, strlen($0)) }
    }

    /// Reads `count` request lines as (id, op).
    private func readRequests(_ count: Int, from connection: Int32) throws -> [(Int, String)] {
        var buffer = MuseLineBuffer()
        var requests: [(Int, String)] = []
        var chunk = [UInt8](repeating: 0, count: 4096)
        while requests.count < count {
            let read = Darwin.read(connection, &chunk, chunk.count)
            guard read > 0 else { throw Timeout() }
            for line in buffer.append(Data(chunk[0..<read])) {
                let object = try XCTUnwrap(JSONSerialization.jsonObject(with: line) as? [String: Any])
                requests.append((try XCTUnwrap(object["id"] as? Int), try XCTUnwrap(object["op"] as? String)))
            }
        }
        return requests
    }

    private struct Timeout: Error {}

    private func waitUntil(_ what: String, timeout: TimeInterval = 5, _ done: () -> Bool) throws {
        let deadline = Date().addingTimeInterval(timeout)
        while !done() {
            if Date() > deadline { throw Timeout() }
            RunLoop.current.run(until: Date().addingTimeInterval(0.02))
        }
    }

    private func client() -> MuseControlClient {
        let client = MuseControlClient(path: path)
        client.retryInterval = 0.05
        return client
    }

    /// Requests sent before the connection exists wait for it, and each reply
    /// reaches its own request even when muse answers out of order.
    func testQueuesUntilConnectedAndMatchesRepliesById() throws {
        let client = client()
        defer { client.disconnect() }
        var events: [MuseControlEvent] = []
        client.onEvent = { events.append($0) }
        var connected: [Bool] = []
        client.onConnectionChange = { connected.append($0) }

        var answers: [String: String] = [:]
        client.connect()
        client.send(.togglePlay) { answers["toggle-play"] = $0.message }
        client.send(.next) { answers["next"] = $0.message }

        let connection = try accept()
        write(#"{"event":"hello","hello":{"pid":4321,"version":"v1","protocol":1}}"#, to: connection)
        write(#"{"event":"state","state":{"playback":"paused","elapsed":0}}"#, to: connection)
        let requests = try readRequests(2, from: connection)
        XCTAssertEqual(requests.map(\.1), ["toggle-play", "next"])
        for (id, op) in requests.reversed() {
            write("{\"reply\":\(id),\"ok\":true,\"message\":\"did \(op)\"}", to: connection)
        }

        try waitUntil("both replies") { answers.count == 2 }
        XCTAssertEqual(answers, ["toggle-play": "did toggle-play", "next": "did next"])
        try waitUntil("the events") { events.count == 2 }
        XCTAssertEqual(events, [
            .hello(MuseHello(pid: 4321, version: "v1", protocol: 1)),
            .state(MusePlaybackState(playback: .paused)),
        ])
        XCTAssertEqual(client.helloPID, 4321)
        XCTAssertEqual(connected, [true])
        XCTAssertTrue(client.isConnected)
    }

    /// muse ending the connection without announcing a quit (a relaunch, a
    /// crash) sends the client looking for the next muse.
    func testReconnectsAfterAnUnannouncedEOF() throws {
        let client = client()
        defer { client.disconnect() }
        var connected: [Bool] = []
        client.onConnectionChange = { connected.append($0) }
        client.connect()

        let first = try accept()
        write(#"{"event":"hello","hello":{"pid":1001,"version":"v1","protocol":1}}"#, to: first)
        try waitUntil("the first hello") { client.helloPID == 1001 }
        var answer: MuseControlReply?
        client.send(.next) { answer = $0 }
        _ = try readRequests(1, from: first)
        close(first)
        connections.removeAll { $0 == first }
        // The request in flight fails rather than hanging.
        try waitUntil("the dropped request") { answer != nil }
        XCTAssertEqual(answer?.ok, false)

        let second = try accept()
        write(#"{"event":"hello","hello":{"pid":1002,"version":"v1","protocol":1}}"#, to: second)
        try waitUntil("the second hello") { client.helloPID == 1002 }
        XCTAssertFalse(client.reachedEOF)
        try waitUntil("the reconnect") { connected == [true, false, true] }
    }

    /// The quit path: after muse says it is quitting, the EOF is the answer;
    /// the client records it and stays put.
    func testAnAnnouncedQuitEndsInEOFWithoutReconnecting() throws {
        let client = client()
        defer { client.disconnect() }
        client.connect()

        let connection = try accept()
        write(#"{"event":"hello","hello":{"pid":4321,"version":"v1","protocol":1}}"#, to: connection)
        try waitUntil("the hello") { client.helloPID == 4321 }
        XCTAssertFalse(client.reachedEOF)
        write(#"{"event":"quitting"}"#, to: connection)
        close(connection)
        connections.removeAll { $0 == connection }

        try waitUntil("the EOF") { client.reachedEOF }
        XCTAssertEqual(client.helloPID, 4321, "the pid outlives the connection for the drain")
        XCTAssertThrowsError(try accept(timeout: 0.5), "no reconnect after a quit")

        client.disconnect()
        XCTAssertNil(client.helloPID)
        XCTAssertFalse(client.reachedEOF)
    }

    func testARequestWithNoMuseFails() {
        let client = MuseControlClient(path: path + ".absent")
        let failed = expectation(description: "failure")
        client.send(.togglePlay) { reply in
            XCTAssertFalse(reply.ok)
            failed.fulfill()
        }
        wait(for: [failed], timeout: 2)
    }
}
