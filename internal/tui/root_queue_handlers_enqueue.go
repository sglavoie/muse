package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/player"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// handleTrackEnqueuedMsg adds a newly enqueued track to the queue, sets up an
// undo slot, and triggers playback if the queue was empty. When InsertAfter is
// >= 0 the track is inserted at that position ("play next") instead of being
// appended to the end.
func (m Model) handleTrackEnqueuedMsg(msg libtui.TrackEnqueuedMsg) Model {
	if msg.Err != nil {
		m.statusBar.notify("Enqueue track failed: " + msg.Err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	entry := QueueEntry{
		ID:             msg.QueueID,
		Path:           msg.Track.Path,
		Hash:           msg.Track.Hash,
		Display:        trackDisplay(msg.Track.Artist, msg.Track.Title, msg.Track.Path),
		Artist:         msg.Track.Artist,
		Title:          msg.Track.Title,
		Album:          msg.Track.Album,
		DurationMs:     int64(msg.Track.Duration * 1000),
		SourcePlaylist: msg.SourcePlaylist,
	}
	var enqueueIdx int
	if msg.InsertAfter >= 0 && msg.InsertAfter <= len(m.queue.entries) {
		idx := msg.InsertAfter
		entries := make([]QueueEntry, len(m.queue.entries)+1)
		copy(entries, m.queue.entries[:idx])
		entries[idx] = entry
		copy(entries[idx+1:], m.queue.entries[idx:])
		m.queue.entries = entries
		enqueueIdx = idx
		slog.Debug("track play-next", "hash", msg.Track.Hash, "position", idx)
		if m.playback.currentIndex >= 0 && idx <= m.playback.currentIndex {
			m.playback.currentIndex++
		}
		m.syncPlayerQueue()
	} else {
		m.queue.entries = append(m.queue.entries, entry)
		enqueueIdx = len(m.queue.entries) - 1
		slog.Debug("track enqueued", "hash", msg.Track.Hash, "position", enqueueIdx)
		if appender, ok := m.player.(playerQueueAppender); ok {
			appender.AppendToQueue(queueTrackFromEntry(entry))
		}
	}
	m = m.preserveQueueSort()
	m.setUndoSlot(undoEnqueueAction{
		id:    msg.QueueID,
		entry: m.queue.entries[enqueueIdx],
		idx:   enqueueIdx,
	}, nil)
	m.queueDirty = true
	player.PlaySound(player.SoundEnqueued)
	if msg.WasEmpty && m.playback.state == PlaybackIdle {
		m = m.tryAutoPlay(msg.Track.Hash, msg.Track.Artist, msg.Track.Title, msg.SourcePlaylist)
	} else if m.playback.queueExhausted && m.playback.state == PlaybackIdle && m.player != nil {
		if err := m.player.PlayFromIndex(enqueueIdx); err == nil {
			m.playback.state = PlaybackPlaying
			m.playback.currentHash = msg.Track.Hash
			m.playback.currentIndex = enqueueIdx
			m.playback.elapsedSecs = 0
			m.playback.totalSecs = 0
			m.playback.peakElapsedSecs = 0
			m.playback.peakTotalSecs = 0
			m.playback.durationCorrected = false
			m.statusBar.artist = msg.Track.Artist
			m.statusBar.title = msg.Track.Title
			m.statusBar.playlistSource = queueSourceName(msg.SourcePlaylist)
		}
		m.playback.queueExhausted = false
	}
	return m
}

// handleBatchEnqueuedMsg adds a batch of enqueued tracks (e.g. from a playlist)
// to the queue and triggers playback if the queue was empty.
func (m Model) handleBatchEnqueuedMsg(msg BatchEnqueuedMsg) Model {
	if msg.Err != nil {
		m.statusBar.notify("Enqueue playlist failed: " + msg.Err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	if len(msg.Entries) == 0 {
		m.statusBar.notify("Playlist is empty")
		return m
	}
	batchStartIdx := len(m.queue.entries)
	m.queue.entries = append(m.queue.entries, msg.Entries...)
	m = m.preserveQueueSort()
	if appender, ok := m.player.(playerQueueAppender); ok {
		for _, entry := range msg.Entries {
			appender.AppendToQueue(queueTrackFromEntry(entry))
		}
	}
	if msg.WasEmpty && m.playback.state == PlaybackIdle {
		entry := msg.Entries[0]
		m = m.tryAutoPlay(entry.Hash, entry.Artist, entry.Title, msg.PlaylistName)
	} else if m.playback.queueExhausted && m.playback.state == PlaybackIdle && m.player != nil {
		if err := m.player.PlayFromIndex(batchStartIdx); err == nil {
			entry := msg.Entries[0]
			m.playback.state = PlaybackPlaying
			m.playback.currentHash = entry.Hash
			m.playback.currentIndex = batchStartIdx
			m.playback.elapsedSecs = 0
			m.playback.totalSecs = 0
			m.playback.peakElapsedSecs = 0
			m.playback.peakTotalSecs = 0
			m.playback.durationCorrected = false
			m.statusBar.artist = entry.Artist
			m.statusBar.title = entry.Title
			m.statusBar.playlistSource = queueSourceName(msg.PlaylistName)
		}
		m.playback.queueExhausted = false
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
	if err := m.player.PlayFromIndex(0); err != nil {
		m.playback.state = PlaybackIdle
		m.statusBar.notify("Play failed: " + err.Error())
		player.PlaySound(player.SoundError)
		return m
	}
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = hash
	m.playback.currentIndex = 0
	m.statusBar.artist = artist
	m.statusBar.title = title
	m.statusBar.playlistSource = queueSourceName(source)
	return m
}

// handlePlayQueueIndexMsg starts playback from a specific queue index.
// The actual player.PlayFromIndex call runs in a goroutine (via tea.Cmd)
// so that synchronous mpv IPC, DB writes, and tea.Program.Send() from
// inside the player never block the Update goroutine. A successful call
// is followed up by a TrackChangedMsg from the player's start-file
// event; a failure surfaces as playFromIndexResultMsg so the TUI can
// recover (including removal when the file disappeared mid-flight).
func (m Model) handlePlayQueueIndexMsg(msg PlayQueueIndexMsg) (Model, tea.Cmd) {
	if m.player == nil {
		return m, nil
	}
	if msg.Index < 0 || msg.Index >= len(m.queue.entries) {
		return m, nil
	}
	entry := m.queue.entries[msg.Index]
	// Optimistic UI update: show the track as playing immediately. The
	// player will either confirm via TrackChangedMsg or we'll reset
	// state from the result handler if PlayFromIndex fails.
	m.playback.state = PlaybackPlaying
	m.playback.currentHash = entry.Hash
	m.playback.currentIndex = msg.Index
	m.playback.elapsedSecs = 0
	m.playback.totalSecs = 0
	m.playback.peakElapsedSecs = 0
	m.playback.peakTotalSecs = 0
	m.playback.durationCorrected = false
	m.statusBar.artist = entry.Artist
	m.statusBar.title = entry.Title
	m.statusBar.playlistSource = queueSourceName(entry.SourcePlaylist)

	p := m.player
	idx := msg.Index
	path := entry.Path
	hash := entry.Hash
	return m, func() tea.Msg {
		err := p.PlayFromIndex(idx)
		return playFromIndexResultMsg{Index: idx, Path: path, Hash: hash, Err: err}
	}
}
