package tui

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// minListenSecs is the floor below which a listen is discarded entirely. mpv's
// gapless prefetch and reconnect churn land at 0-1s, and next-next-next
// browsing is navigation rather than judgment; neither is evidence about the
// track. A 5s bail still records completion_rate ~= 0.02.
const minListenSecs = 3.0

// pendingListen is the capture state for one listen. It is deliberately
// independent of the peak/elapsed progress fields, which five scattered reset
// sites zero, because armed must survive until exactly one end site consumes
// it.
type pendingListen struct {
	hash         string
	artist       string
	listenedSecs float64
	totalSecs    float64
	armed        bool
}

func (p pendingListen) record(playedAt int64) db.PlayRecord {
	return db.PlayRecord{
		Hash:         p.hash,
		PlayedAt:     playedAt,
		ListenedSecs: int(p.listenedSecs),
		DurationSecs: p.totalSecs,
		Counted:      shouldCountPlay(p.listenedSecs, p.totalSecs),
	}
}

// armPendingListen starts capturing a listen for hash. artist is snapshotted
// here because the session-stat bump happens at flush time, by which point the
// status bar already carries the next track's metadata.
func (m Model) armPendingListen(hash, artist string) Model {
	m.playback.pending = pendingListen{hash: hash, artist: artist, armed: hash != ""}
	return m
}

// endCurrentListen snapshots the in-flight listen into the flush queue and
// disarms. A second call before the next arm is a no-op, which is what makes
// capture exactly-once across the end sites — notably the repeat-all wrap,
// which leaves playback.currentHash pointing at the track it just finished.
func (m Model) endCurrentListen() Model {
	if !m.playback.pending.armed {
		return m
	}
	p := m.playback.pending
	p.listenedSecs = m.playback.listenedSecs
	p.totalSecs = m.playback.peakTotalSecs
	m.playback.pending = pendingListen{}
	m.playback.listenedSecs = 0
	m.playback.lastSampleSecs = 0
	if p.listenedSecs < minListenSecs {
		return m
	}
	m.playback.flushes = append(m.playback.flushes, p)
	return m
}

// drainListenFlushes turns every queued listen into a persistence command.
// Update calls it once per message because resetPlaybackState, resetQueueState
// and handleDequeueTrackMsg are value receivers returning only Model, so they
// have no way to hand a tea.Cmd back themselves.
func (m Model) drainListenFlushes() (Model, tea.Cmd) {
	if len(m.playback.flushes) == 0 {
		return m, nil
	}
	queued := m.playback.flushes
	m.playback.flushes = nil
	cmds := make([]tea.Cmd, 0, len(queued))
	for _, p := range queued {
		var cmd tea.Cmd
		m, cmd = m.recordListenCmd(p)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// recordListenCmd applies the in-memory effects of a finished listen and
// returns the command that persists it. Everything that mirrors
// tracks.play_count is gated on Counted so the in-memory index cannot drift
// above what RecordListen wrote.
func (m Model) recordListenCmd(p pendingListen) (Model, tea.Cmd) {
	rec := p.record(time.Now().Unix())
	if rec.Counted {
		m.mixes.playsSinceRefresh++
		m.sessionTracksPlayed++
		m.sessionListenSecs += p.listenedSecs
		m.bumpSessionArtist(p.artist)
		m.library.index.IncrementPlayCount(rec.Hash, rec.PlayedAt)
	}
	if m.historyRecorder != nil {
		hr := m.historyRecorder
		return m, func() tea.Msg {
			if err := hr.RecordListen(rec); err != nil {
				slog.Warn("listen capture: RecordListen failed", "hash", rec.Hash, "err", err)
				return nil
			}
			return playRecordedMsg{Hash: rec.Hash, PlayedAt: rec.PlayedAt, Counted: rec.Counted}
		}
	}
	// The fallback writer can only bump play_count, so an uncounted listen has
	// nothing it can durably record.
	if rec.Counted && m.playCountWriter != nil {
		pcw := m.playCountWriter
		return m, func() tea.Msg {
			if err := pcw.IncrementPlayCount(rec.Hash); err != nil {
				slog.Warn("listen capture: IncrementPlayCount failed", "hash", rec.Hash, "err", err)
			}
			return nil
		}
	}
	return m, nil
}

// TakePendingListen consumes the in-flight listen so the quit path can persist
// it synchronously. Quit is the one end site that leaves the listen armed:
// Bubble Tea has already exited by then, so a tea.Cmd would never run.
func (m *Model) TakePendingListen() (db.PlayRecord, bool) {
	*m = m.endCurrentListen()
	if len(m.playback.flushes) == 0 {
		return db.PlayRecord{}, false
	}
	p := m.playback.flushes[0]
	m.playback.flushes = nil
	return p.record(time.Now().Unix()), true
}
