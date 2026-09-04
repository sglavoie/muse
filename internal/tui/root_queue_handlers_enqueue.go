package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/text"
)

// handleTrackEnqueuedMsg adds a newly enqueued track to the queue, sets up an
// undo slot, and triggers playback if the queue was empty. When InsertAfter is
// >= 0 the track is inserted at that position ("play next") instead of being
// appended to the end.
func (m Model) handleTrackEnqueuedMsg(msg libtui.TrackEnqueuedMsg) Model {
	if msg.Err != nil {
		m.statusBar.Notify("Enqueue track failed: " + msg.Err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	entry := queue.Entry{
		ID:             msg.QueueID,
		Path:           msg.Track.Path,
		Hash:           msg.Track.Hash,
		Display:        text.TrackDisplay(msg.Track.Artist, msg.Track.Title, msg.Track.Path),
		Artist:         msg.Track.Artist,
		Title:          msg.Track.Title,
		Album:          msg.Track.Album,
		DurationMs:     int64(msg.Track.Duration * 1000),
		SourcePlaylist: msg.SourcePlaylist,
	}
	var enqueueIdx int
	if msg.InsertAfter >= 0 && msg.InsertAfter <= len(m.queue.Entries) {
		idx := msg.InsertAfter
		entries := make([]queue.Entry, len(m.queue.Entries)+1)
		copy(entries, m.queue.Entries[:idx])
		entries[idx] = entry
		copy(entries[idx+1:], m.queue.Entries[idx:])
		m.queue.Entries = entries
		enqueueIdx = idx
		slog.Debug("track play-next", "hash", msg.Track.Hash, "position", idx)
		if m.playback.currentIndex >= 0 && idx <= m.playback.currentIndex {
			m.playback.currentIndex++
		}
		m.syncPlayerQueue()
	} else {
		m.queue.Entries = append(m.queue.Entries, entry)
		enqueueIdx = len(m.queue.Entries) - 1
		slog.Debug("track enqueued", "hash", msg.Track.Hash, "position", enqueueIdx)
		if appender, ok := m.player.(playerQueueAppender); ok {
			appender.AppendToQueue(queueTrackFromEntry(entry))
		}
	}
	m = m.preserveQueueSort()
	m.setUndoSlot(undoEnqueueAction{
		id:    msg.QueueID,
		entry: m.queue.Entries[enqueueIdx],
		idx:   enqueueIdx,
	}, nil)
	m.queueDirty = true
	player.PlaySound(player.SoundEnqueued)
	if msg.WasEmpty && m.playback.state == playback.StateIdle {
		m = m.tryAutoPlay(msg.Track.Hash, msg.Track.Artist, msg.Track.Title, msg.SourcePlaylist)
	} else if m.playback.queueExhausted && m.playback.state == playback.StateIdle {
		m, _ = m.resumeExtendedQueue(enqueueIdx)
	}
	return m
}

// handleBatchEnqueuedMsg adds a batch of enqueued tracks (e.g. from a playlist)
// to the queue and triggers playback if the queue was empty.
func (m Model) handleBatchEnqueuedMsg(msg BatchEnqueuedMsg) Model {
	if msg.Err != nil {
		m.statusBar.Notify("Enqueue failed: " + msg.Err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	if len(msg.Entries) == 0 {
		m.statusBar.Notify("Playlist is empty")
		return m
	}
	batchStartIdx := len(m.queue.Entries)
	m.queue.Entries = append(m.queue.Entries, msg.Entries...)
	m = m.preserveQueueSort()
	if appender, ok := m.player.(playerQueueAppender); ok {
		for _, entry := range msg.Entries {
			appender.AppendToQueue(queueTrackFromEntry(entry))
		}
	}
	if msg.WasEmpty && m.playback.state == playback.StateIdle {
		entry := msg.Entries[0]
		m = m.tryAutoPlay(entry.Hash, entry.Artist, entry.Title, msg.PlaylistName)
	} else if m.playback.queueExhausted && m.playback.state == playback.StateIdle {
		m, _ = m.resumeExtendedQueue(batchStartIdx)
	}
	m.setUndoSlot(undoBatchEnqueueAction{
		ids:          msg.QueueIDs,
		count:        len(msg.Entries),
		startIdx:     batchStartIdx,
		playlistName: msg.PlaylistName,
	}, nil)
	m.queueDirty = true
	player.PlaySound(player.SoundEnqueued)
	return m
}

// tryAutoPlay attempts to start playback from index 0 after an enqueue into an
// empty queue. On failure it notifies the user and reverts to idle.
func (m Model) tryAutoPlay(hash, artist, title, source string) Model {
	if m.player == nil {
		return m
	}
	// Autoplay is a selection like any other, made on the Update goroutine
	// instead of in a command: it has to supersede the queued ones, or a
	// selection dispatched against the queue this one just replaced could land
	// on top of the row that is starting here.
	if err := m.playRequestExclusive(func() error { return m.player.PlayFromIndex(0) }); err != nil {
		m.playback.state = playback.StateIdle
		m.statusBar.Notify("Play failed: " + err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = hash
	m.playback.currentIndex = 0
	// Clear elapsed/peak/durationCorrected from any prior track so the new
	// track's progress bar and duration-correction logic start fresh.
	m = m.resetPlaybackProgress()
	m.statusBar.Artist = artist
	m.statusBar.Title = title
	m.statusBar.PlaylistSource = queueSourceName(source)
	return m
}

// handlePlayQueueIndexMsg starts playback from a specific queue index.
//
// Selecting the row that is already playing means "start this over", not "start
// a new selection", so it goes to ReplayCurrent when the player offers it: an
// explicit PlayFromIndex would reanchor the shuffle cycle, dropping the played
// history and re-permuting everything still ahead. Every other selection — a
// different row, or any row while nothing is current — is a genuine new
// selection and keeps calling PlayFromIndex, which anchors a fresh cycle around
// the chosen entry.
//
// The player call runs in a goroutine (via tea.Cmd) so that synchronous mpv IPC,
// DB writes, and tea.Program.Send() from inside the player never block the
// Update goroutine. A successful call is followed up by a TrackChangedMsg from
// the player's start-file event; a failure surfaces as PlayFromIndexResultMsg so
// the TUI can recover (including removal when the file disappeared mid-flight).
func (m Model) handlePlayQueueIndexMsg(msg playback.PlayQueueIndexMsg) (Model, tea.Cmd) {
	if m.player == nil {
		return m, nil
	}
	if msg.Index < 0 || msg.Index >= len(m.queue.Entries) {
		return m, nil
	}
	entry := m.queue.Entries[msg.Index]
	// Resolve the operation from the pre-update state: the optimistic block
	// below overwrites currentIndex and currentHash with the selected row, after
	// which every selection would look like a replay.
	replayer, replay := m.selectionReplayer(msg.Index, entry.Hash)
	// Selecting a row is a listen boundary, and it is one the later
	// TrackChangedMsg cannot capture: the optimistic reset below zeroes
	// listenedSecs and peakTotalSecs, so by the time that message arrives the
	// still-armed listen would measure 0s and be dropped at the capture floor.
	// End it here, above the reset, so it is queued with its real measurement.
	// The reset must stay after this call for the same reason. endCurrentListen
	// disarms, so the TrackChangedMsg that follows a successful call is a no-op
	// on the previous track and the failure handler's resetPlaybackState has
	// nothing left to erase. A replay ends the listen too: the track is starting
	// again from zero, so what was heard up to now is a finished listen.
	m = m.endCurrentListen()
	// Optimistic UI update: show the track as playing immediately. The
	// player will either confirm via TrackChangedMsg or we'll reset
	// state from the result handler if the call fails.
	m.playback.state = playback.StatePlaying
	m.playback.currentHash = entry.Hash
	m.playback.currentIndex = msg.Index
	m = m.resetPlaybackProgress()
	m.statusBar.Artist = entry.Artist
	m.statusBar.Title = entry.Title
	m.statusBar.PlaylistSource = queueSourceName(entry.SourcePlaylist)

	// Stamp this request with a fresh identity so its result can be told apart
	// from the results of any other request, including an earlier one aimed at
	// this same row. Without it, holding Enter on one row issues calls whose
	// outcomes are indistinguishable and an older failure can undo a newer
	// success.
	//
	// The same identity goes into the shared gate, which is what carries the
	// dispatch order into the goroutines below: the runtime is free to run them
	// in any order, and a stale one reaching the player last would leave mpv on
	// a row the user has already moved away from. The gate lets the newest
	// request through and supersedes the rest.
	gate := m.playback.requests
	req := gate.dispatch(m.playback.playRequestSeq)
	m.playback.playRequestSeq = req
	p := m.player
	idx := msg.Index
	result := func(err error) tea.Msg {
		return playback.PlayFromIndexResultMsg{
			Request: req,
			Index:   idx,
			Path:    entry.Path,
			Hash:    entry.Hash,
			Err:     err,
			Replay:  replay,
		}
	}
	if replay {
		// ReplayCurrent leaves the player on the entry it is already holding, so
		// the current row stays confirmed and a follow-up Enter can replay again.
		return m, func() tea.Msg {
			err, _ := gate.run(req, replayer.ReplayCurrent)
			return result(err)
		}
	}
	// A new selection moves the player's current entry, and nothing has
	// confirmed the move yet; see pendingSelection. The track-event barrier is
	// raised with it and stays up past this request's own result, until the
	// player reports starting the selected row; see awaitingSelectionStart.
	m.playback.pendingSelection = true
	m.playback.raiseSelectionBarrier(entry.ID)
	return m, func() tea.Msg {
		err, _ := gate.run(req, func() error { return p.PlayFromIndex(idx) })
		return result(err)
	}
}

// selectionReplayer reports whether selecting queue row index means "restart the
// current track", and returns the player operation that does it.
//
// It is a replay only when the selected row is the one the model believes is
// loaded — same index and same identity, so a duplicate of the playing track
// elsewhere in the queue is still a new selection — and only while there is
// something loaded to restart: Playing, Paused, and Stopped all sit on a current
// entry, whereas Idle (including an exhausted queue) does not and must anchor a
// fresh cycle instead.
//
// A selection that has not been confirmed yet is never replayable. The
// optimistic update in handlePlayQueueIndexMsg writes the selected row into
// currentIndex/currentHash before the player has been asked, so a second Enter
// arriving first would match against a row the player is not on yet and hand
// ReplayCurrent whatever entry it is still holding — restarting the previous
// track instead of the selected one. PlayFromIndex names its target explicitly,
// so falling through to it is always right for a row whose status is unresolved.
func (m Model) selectionReplayer(index int, hash string) (currentTrackReplayer, bool) {
	if m.playback.pendingSelection {
		return nil, false
	}
	if index != m.playback.currentIndex || hash != m.playback.currentHash {
		return nil, false
	}
	switch m.playback.state {
	case playback.StatePlaying, playback.StatePaused, playback.StateStopped:
		// A track is loaded, so there is something to restart.
	default:
		// StateIdle, and any state added later: nothing is loaded. Defaulting
		// to "not a replay" keeps a new state anchoring a fresh cycle, which is
		// the behaviour selection had before replay existed.
		return nil, false
	}
	replayer, ok := m.player.(currentTrackReplayer)
	if !ok {
		return nil, false
	}
	return replayer, true
}
