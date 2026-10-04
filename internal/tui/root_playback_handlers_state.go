package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/text"
)

// handlePlaybackEndedMsg handles the end of the play queue, optionally
// wrapping around in repeat-all mode.
func (m Model) handlePlaybackEndedMsg() (Model, tea.Cmd) {
	wasStopAfter := m.playback.stopAfterCurrent
	m.playback.stopAfterCurrent = false
	// A stop-after-current that ran to completion is the user ending playback, not
	// the queue running dry, so the refill is retired here. The flag is already
	// back to false by the time any later guard could read it, and every
	// !wasStopAfter branch below is skipped, so nothing else on this path would
	// tell a parked refill that its wakeup is no longer wanted.
	if wasStopAfter {
		m = m.radioNoteDeliberateStop()
	}
	// Nothing lowers awaitingSelectionStart here: the traversal cannot have
	// exhausted before the reader dispatched the start-file of the row it played
	// out, and that report already retired the barrier.

	// One snapshot before the branch cascade covers all four exits below.
	m = m.endCurrentListen()

	if !wasStopAfter && m.playback.repeatMode == playback.RepeatAll && len(m.queue.Entries) > 0 && m.player != nil {
		// Prefer RestartQueueCycle so shuffle reshuffles into a fresh permutation
		// (and sequential restarts from index 0). Fall back to PlayFromIndex(0)
		// for lightweight players that do not implement the optional interface.
		restart := m.player.PlayFromIndex
		if r, ok := m.player.(queueCycleRestarter); ok {
			restart = func(int) error { return r.RestartQueueCycle() }
		}
		if err := restart(0); err != nil {
			slog.Warn("repeat-all playback failed", "err", err)
		} else {
			m.playback.state = playback.StatePlaying
			m = m.resetPlaybackProgress()
			m.missingTrackCount = 0
			return m, nil
		}
	}

	// If the queue was concurrently extended (e.g. a paste was processed
	// before this message arrived), resume playback instead of stopping.
	//
	// A refused resume is not exhaustion: the rows are still queued and the
	// model stays idle so the next extension retries. resumeExtendedQueue has
	// already told the user why, so resumeFailed carries that fact past the
	// branches below, which would otherwise announce a completion that did not
	// happen — and, being the later push at equal priority, would win
	// TopNotification's tie-break and bury the actionable error.
	resumeFailed := false
	if !wasStopAfter && m.queueExtensionResumable() {
		resumed := false
		m, resumed = m.resumeExtendedQueue(m.playback.currentIndex + 1)
		if resumed {
			return m, nil
		}
		resumeFailed = true
	}

	// Radio mode: enqueue a new batch if enabled and guards pass.
	if !wasStopAfter {
		if next, cmd := m.startRadioEnqueueCmd(); cmd != nil {
			m = next.idleAfterQueueEnd(true)
			if !resumeFailed {
				player.PlayQueueFinishedSound()
			}
			return m, cmd
		}
	}

	// Built before the state is cleared: the notice reads the missing-track
	// count that idleAfterQueueEnd resets.
	notice := m.queueEndNotice(wasStopAfter)
	// Mark queue as exhausted so a subsequent paste can auto-resume.
	m = m.idleAfterQueueEnd(!wasStopAfter)

	if resumeFailed {
		// The "Resume failed" notification and the error sound are the whole
		// report here; nothing finished, so neither the completion message nor
		// the completion sound belongs on top of them.
		return m, nil
	}

	m.statusBar.Notify(notice)
	player.PlayQueueFinishedSound()
	return m, nil
}

// idleAfterQueueEnd clears the now-playing state that every non-resuming exit of
// handlePlaybackEndedMsg shares. exhausted marks the queue as replenishable so a
// later extension resumes the cycle instead of reanchoring it; a
// stop-after-current is the one end that is not exhaustion, and it leaves the
// flag alone rather than clearing it.
func (m Model) idleAfterQueueEnd(exhausted bool) Model {
	if exhausted {
		m.playback.queueExhausted = true
	}
	m.playback.state = playback.StateIdle
	m = m.resetPlaybackProgress()
	m.statusBar.Artist = ""
	m.statusBar.Title = ""
	m.statusBar.PlaylistSource = ""
	m.playback.currentHash = ""
	m.playback.currentIndex = -1
	m.missingTrackCount = 0
	return m
}

// queueEndNotice builds the message announcing that playback stopped. It reads
// missingTrackCount, so it must run before idleAfterQueueEnd clears it.
func (m Model) queueEndNotice(wasStopAfter bool) string {
	switch {
	case wasStopAfter:
		return "Stopped after track"
	case m.missingTrackCount > 0:
		return fmt.Sprintf("Queue finished (%d %s skipped — missing files)",
			m.missingTrackCount, text.Pluralize(m.missingTrackCount, "track", "tracks"))
	default:
		return "Queue finished"
	}
}

// handlePlayerDisconnectedMsg resets TUI playback state when mpv disconnects
// unexpectedly so the play/pause icon and progress bar are not stale during
// the reconnection window.
func (m Model) handlePlayerDisconnectedMsg() Model {
	m = m.endCurrentListen()
	m.playback.state = playback.StateStopped
	m = m.resetPlaybackProgress()
	m.statusBar.Notify("mpv disconnected, restarting…")
	return m
}

// handlePauseChangedMsg syncs the TUI playback state with mpv's actual pause
// property. Guards prevent redundant transitions (e.g. user presses space ->
// TUI already updated -> mpv confirms via event -> no-op).
func (m Model) handlePauseChangedMsg(msg playback.PauseChangedMsg) (Model, tea.Cmd) {
	if msg.Paused {
		if m.playback.state == playback.StatePlaying {
			m.playback.state = playback.StatePaused
			return m, m.sessionPositionCmd()
		}
	} else {
		if m.playback.state == playback.StatePaused || m.playback.state == playback.StateStopped {
			m.playback.state = playback.StatePlaying
		}
	}
	return m, nil
}

// handlePreviewStartedMsg stores the preview player and overlay state.
func (m Model) handlePreviewStartedMsg(msg previewStartedMsg) Model {
	m.overlay.previewStarting = false
	if msg.request != 0 && msg.request != m.playback.requestSeq {
		_ = msg.player.Close()
		m.overlay.previewWasPlaying = false
		return m
	}
	m.overlay.previewPlayer = msg.player
	m.overlay.preview = &msg.overlay
	m.overlay.previewDurationFilled = false
	return m
}

// handlePreviewPauseResultMsg starts the preview only after the ordered pause
// succeeded and only while no newer playback request has superseded it.
func (m Model) handlePreviewPauseResultMsg(msg previewPauseResultMsg) (Model, tea.Cmd) {
	if msg.request != m.playback.requestSeq || !msg.ran {
		m.overlay.previewStarting = false
		m.overlay.previewWasPlaying = false
		return m, nil
	}
	if msg.err != nil {
		if m.playback.state == playback.StatePaused {
			m.playback.state = playback.StatePlaying
		}
		m.overlay.previewStarting = false
		m.overlay.previewWasPlaying = false
		slog.Warn("player pause for preview failed", "err", msg.err)
		m.statusBar.NotifyError("Preview pause failed: " + msg.err.Error())
		return m, nil
	}
	return m, m.startPreviewCmd(msg.track, msg.request)
}

// handlePreviewStartErrorMsg notifies the user and resumes main playback.
func (m Model) handlePreviewStartErrorMsg(msg previewStartErrorMsg) (Model, tea.Cmd) {
	m.overlay.previewStarting = false
	m.statusBar.NotifyError("Preview error: " + msg.err.Error())
	player.PlaySound(player.SoundError)
	if msg.request != 0 && msg.request != m.playback.requestSeq {
		m.overlay.previewWasPlaying = false
		return m, nil
	}
	return m.resumeAfterPreview()
}

// raiseSelectionBarrier arms the track-event barrier for the queue occurrence
// the user just selected. id is that occurrence's queue row id, or 0 when the
// row is not persisted.
func (p *playbackInfo) raiseSelectionBarrier(id int64) {
	p.awaitingSelectionStart = true
	p.awaitingSelectionID = id
}

// lowerSelectionBarrier retires the barrier. The two fields always move
// together: a stale id left behind an already-lowered barrier would be compared
// against the next selection's reports.
func (p *playbackInfo) lowerSelectionBarrier() {
	p.awaitingSelectionStart = false
	p.awaitingSelectionID = 0
}

// startsSelectedOccurrence reports whether a track-change event is the report
// the standing barrier is waiting for.
//
// The queue row id settles it whenever both sides have one: it names the
// occurrence rather than the track, so the abandoned duplicate's delayed report
// is told apart from the selected duplicate's arrival even though the two carry
// the same hash, artist, and title. Falling back to the hash covers the rows
// that have no durable id — an unpersisted queue — where nothing can separate
// duplicates and the behaviour is the pre-existing one.
func (m Model) startsSelectedOccurrence(msg playback.TrackChangedMsg) bool {
	if m.playback.awaitingSelectionID != 0 && msg.QueueID != 0 {
		return msg.QueueID == m.playback.awaitingSelectionID
	}
	return msg.Hash == m.playback.currentHash
}

// handleTrackChangedMsg updates the status bar and playback state when the
// player advances to a new track.
func (m Model) handleTrackChangedMsg(msg playback.TrackChangedMsg) (Model, tea.Cmd) {
	// Guard: discard stale TrackChangedMsg that arrives after the queue was
	// cleared (e.g. mpv's gapless prefetch fired a start-file event before
	// the clear was processed). Without this check the TUI would resurrect
	// playback state on an empty queue, showing a playing status bar with
	// no corresponding queue entry.
	if len(m.queue.Entries) == 0 {
		return m.endCurrentListen(), nil
	}

	// Guard: a selection the player has not started yet outranks a report about
	// any other row. While awaitingSelectionStart is set, awaitingSelectionID is
	// the occurrence the user asked for and the player has already been told to
	// move onto it, so a start-file naming a different entry describes the row
	// that selection is in the middle of replacing — an older command that
	// reached mpv first, or the gapless successor it was about to start. Applying
	// it would move the current row off the newer selection, and the request
	// identity that separates the two commands' results is not carried on this
	// message.
	//
	// The barrier is keyed to the start-file rather than to the selection's own
	// result because the result is the earlier of the two: mpv answers the load
	// on the IPC socket while the event reader is still syncing the position of
	// the start-file it read before it. Only the selected row's report, below,
	// proves every earlier one has already been delivered. A refused load lowers
	// the barrier in handlePlayFromIndexResultMsg instead, since no start-file
	// will follow it.
	//
	// Keep the suspended outgoing measurement across rejected reports; only
	// an accepted start proves that listen ended.
	if m.playback.awaitingSelectionStart && !m.startsSelectedOccurrence(msg) {
		slog.Debug("track change discarded: a newer selection has not started yet",
			"reported", msg.Hash, "reportedQueueID", msg.QueueID,
			"selected", m.playback.currentHash, "selectedQueueID", m.playback.awaitingSelectionID)
		return m, nil
	}

	m = m.endCurrentListen()

	// Capture the outgoing hash before the overwrite below: the notification
	// at the end of this handler must only fire on a genuine track change.
	prevHash := m.playback.currentHash

	m.playback.currentHash = msg.Hash
	idx := m.trackChangeIndex(msg)
	m.playback.currentIndex = idx
	// The player has named the row it is on, which settles any selection still
	// waiting for confirmation: from here currentIndex is the player's answer,
	// not the optimistic guess, so Enter on it is a genuine replay again. It also
	// retires the track-event barrier — this report cleared the reader of every
	// start-file that preceded it, so the next one describes the present.
	m.playback.pendingSelection = false
	m.playback.lowerSelectionBarrier()
	m.playback.queueExhausted = false
	if m.followNowPlaying && m.ActiveView == viewQueue {
		m.queue = m.queue.ScrollToNowPlaying(idx)
	}
	m.statusBar.Artist = msg.Artist
	m.statusBar.Title = msg.Title
	if t, ok := m.library.Index.FindByHash(msg.Hash); ok {
		m.statusBar.Album = t.Album
	} else {
		// Clear the previous track's album so it does not leak into the
		// status bar when the new hash isn't in the library index.
		m.statusBar.Album = ""
	}
	source := msg.PlaylistSource
	if source == "" {
		source = "Queue"
	}
	m.statusBar.PlaylistSource = source
	m.playback.state = playback.StatePlaying
	m = m.resetPlaybackProgress()
	m = m.armPendingListen(msg.Hash, msg.Artist)

	// Refresh the now-playing overlay with the new track's metadata.
	if m.overlay.nowPlaying != nil {
		m.overlay.nowPlaying.Artist = msg.Artist
		m.overlay.nowPlaying.Title = msg.Title
		m.overlay.nowPlaying.Album = ""
		m.overlay.nowPlaying.PlayCount = 0
		m.overlay.nowPlaying.Elapsed = 0
		m.overlay.nowPlaying.Total = 0
		m.overlay.nowPlaying.State = playback.StatePlaying
		m.overlay.nowPlaying.TagsLine = m.buildTagsLine(msg.Hash)
		if t, ok := m.library.Index.FindByHash(msg.Hash); ok {
			if m.overlay.nowPlaying.Artist == "" {
				m.overlay.nowPlaying.Artist = t.Artist
			}
			if m.overlay.nowPlaying.Title == "" {
				m.overlay.nowPlaying.Title = t.Title
			}
			m.overlay.nowPlaying.Album = t.Album
			m.overlay.nowPlaying.PlayCount = t.PlayCount
		}
	}

	// Radio mode: update seed and check low-water-mark.
	m.radio.seedHash = msg.Hash
	// A report may arrive before the command result, or merely re-announce a
	// loaded row on reconnect. Record it, but require the current start request
	// to succeed before lifting a deliberate-stop refill suspension.
	m = m.radioConfirmStartedTrack()
	// The DJ seed moves only under Follow, and only here: every stale-event
	// guard above has already passed, so this is the player's confirmation of
	// the row it is actually on rather than a start-file a newer selection
	// superseded. An Anchor session keeps the context it was captured with.
	// A confirmed track change is the settlement a deferred refresh was waiting
	// for, so it is carried across the reseed below rather than dropped by it.
	// Re-anchoring supersedes DJ work in flight because that work was planned
	// about the previous subject; a deferred refresh is the opposite — the
	// user's applied settings still on their way to the queue, which the
	// relaunch replans against whatever the seed has just become.
	resumeRefresh := m.djRefresh.deferred
	m = m.djNoteTrackChanged(msg.Hash, time.Now())
	var radioCmd tea.Cmd
	m, radioCmd = m.startRadioEnqueueCmd()
	var djRefreshCmd tea.Cmd
	m.djRefresh.deferred = m.djRefresh.deferred || resumeRefresh
	m, djRefreshCmd = m.djResumeDeferredRefresh(time.Now())

	// Pulse the now-playing row briefly so the user notices the transition.
	m.pulseUntil = time.Now().Add(pulseDuration)

	return m, tea.Batch(m.sessionPositionCmd(), radioCmd, pulseCmd(), djRefreshCmd,
		m.trackNotifyCmd(msg, prevHash))
}

// trackChangeIndex resolves the queue row a track-change event names.
//
// msg.Index is only a hint about where the row was when mpv emitted the event.
// The queue can have shrunk or been reordered before this handler ran — a
// dequeue or a move racing a gapless transition — and an index that survives
// the shuffle of positions still in range would then name an unrelated row. It
// is accepted only when the entry sitting there is the occurrence the event
// describes; otherwise the row is found by identity.
//
// An occurrence that is not in the queue at all resolves to -1 rather than
// falling back to 0: pointing currentIndex at an unrelated track is worse than
// pointing it at nothing, and the caller still updates the status bar from the
// message.
func (m Model) trackChangeIndex(msg playback.TrackChangedMsg) int {
	if msg.Index >= 0 && msg.Index < len(m.queue.Entries) &&
		occurrenceMatches(m.queue.Entries[msg.Index], msg) {
		return msg.Index
	}
	for i, e := range m.queue.Entries {
		if occurrenceMatches(e, msg) {
			return i
		}
	}
	return -1
}

// occurrenceMatches reports whether queue row e is the occurrence a track-change
// event names. The queue row id decides it whenever both sides carry one, so a
// duplicate of the same track elsewhere in the queue is never mistaken for it;
// rows with no durable id fall back to the content hash, under which duplicates
// stay indistinguishable exactly as they were before.
func occurrenceMatches(e queue.Entry, msg playback.TrackChangedMsg) bool {
	if e.ID != 0 && msg.QueueID != 0 {
		return e.ID == msg.QueueID
	}
	return e.Hash == msg.Hash
}

// trackNotifyCmd posts a desktop banner for a newly started track, or returns
// nil when notifications are off, no notifier is wired, or the hash did not
// actually change (mpv reconnect and session restore both re-emit the current
// track). A failing notifier — a missing alerter binary, most likely — is
// logged and otherwise ignored: it must never disturb playback.
func (m Model) trackNotifyCmd(msg playback.TrackChangedMsg, prevHash string) tea.Cmd {
	if !m.notifyOnTrackChange || m.notifier == nil || msg.Hash == "" || msg.Hash == prevHash {
		return nil
	}
	notifier := m.notifier
	title, artist, album := msg.Title, msg.Artist, m.statusBar.Album
	return func() tea.Msg {
		if err := notifier.NotifyTrack(title, artist, album); err != nil {
			slog.Warn("track notification failed", "err", err)
		}
		return nil
	}
}

// bumpSessionArtist records one more play for artist in the session
// artist-count map, allocating it lazily. No-op for empty strings.
func (m *Model) bumpSessionArtist(artist string) {
	if artist == "" {
		return
	}
	if m.sessionArtistCounts == nil {
		m.sessionArtistCounts = make(map[string]int)
	}
	m.sessionArtistCounts[artist]++
}

// handlePlayRecordedMsg prepends the just-played track to the in-memory
// library History mode so it updates in realtime without a full DB reload.
func (m Model) handlePlayRecordedMsg(msg playback.PlayRecordedMsg) Model {
	// The listen is in play_events now, so every Insights aggregate that counts
	// it — including the skip counts that gate mix readiness — is out of date.
	m.insightsInvalidate()

	// History reads the counted-only play_history view, so prepending an
	// uncounted listen would show a row that vanishes on the next reload.
	if !msg.Counted {
		return m
	}

	// Only a counted listen moves the Auto-DJ's inputs: PlayCount and
	// LastPlayedAt decide discovery classification, and the completion-rate and
	// co-occurrence signals read the same counted-only history. A skip changes
	// neither, which is why this sits below the guard rather than beside the
	// Insights bump above it.
	m.djInvalidate()

	// Only update when History mode is active and has been loaded.
	if m.library.Mode != libtui.LibraryModeHistory || len(m.library.SourceTracks) == 0 {
		return m
	}

	t, ok := m.library.Index.FindByHash(msg.Hash)
	if !ok {
		return m
	}
	t.LastPlayedAt = msg.PlayedAt

	// Prepend and cap at 500 entries. Drop any pre-existing entry for the
	// same hash so replaying a track in the same session does not produce
	// stacked duplicates at the top of History.
	const maxHistory = 500
	tracks := make([]library.Track, 0, min(len(m.library.SourceTracks)+1, maxHistory))
	tracks = append(tracks, t)
	for _, existing := range m.library.SourceTracks {
		if existing.Hash == msg.Hash {
			continue
		}
		if len(tracks) >= maxHistory {
			break
		}
		tracks = append(tracks, existing)
	}
	m.library.SourceTracks = tracks

	m.library = m.library.RefreshView().ClampCursor()
	return m
}
