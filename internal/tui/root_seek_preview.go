package tui

import (
	"log/slog"
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// Seek preview timing. Pressing or holding a seek key moves a pending target
// that the progress bar shows at once; mpv only seeks once the keys go quiet
// for seekPreviewDebounce, so a held key costs one seek instead of dozens.
const (
	seekPreviewDebounce = 280 * time.Millisecond
	// seekStreakGap is the longest pause between two presses that still
	// counts as holding the key. It outlasts the debounce and a typical
	// key-repeat delay, so a held key keeps accelerating even when its
	// first press already committed before the repeats began.
	seekStreakGap = 650 * time.Millisecond
	// seekSettleWindow is how long, after a seek, position reports far from
	// the target are taken for mpv still finishing the seek and ignored, so
	// the bar never snaps back to where the song was.
	seekSettleWindow = 1500 * time.Millisecond
)

// seekAccelSteps scale the short seek step the longer a key is held: 5s
// becomes 10s after a second and 30s after two and a half (with the
// default seek_offset_short).
var seekAccelSteps = []struct {
	after time.Duration
	scale float64
}{
	{2500 * time.Millisecond, 6},
	{1000 * time.Millisecond, 2},
}

// seekPreviewState is the seek the user is still dialing in with the seek
// keys, plus the one mpv was last asked for while it settles.
type seekPreviewState struct {
	pending bool
	target  float64
	// hash and request pin the pending seek to the song it was aimed at:
	// a track change discards it rather than seeking the next song.
	hash    string
	request uint64
	// seq numbers every press; a commit tick only fires for the newest.
	seq uint64

	// streakStart and lastPress measure how long a key has been held.
	streakStart, lastPress time.Time

	// settleTarget and settleUntil describe the seek mpv was last given.
	settleTarget float64
	settleUntil  time.Time

	// clock reads the time; nil means time.Now. Tests inject one.
	clock func() time.Time
}

// seekCommitMsg fires seekPreviewDebounce after a seek key press.
type seekCommitMsg struct{ seq uint64 }

func (s seekPreviewState) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

// seekKeyOffset returns the signed offset a seek action asks for, and
// whether it is one that accelerates while held.
func (m Model) seekKeyOffset(action keys.Action) (offset float64, accel, ok bool) {
	switch action {
	case keys.ActionSeekBack:
		return -m.cfg.SeekOffsetShort, true, true
	case keys.ActionSeekForward:
		return m.cfg.SeekOffsetShort, true, true
	case keys.ActionSeekBackLong:
		return -m.cfg.SeekOffsetLong, false, true
	case keys.ActionSeekForwardLong:
		return m.cfg.SeekOffsetLong, false, true
	}
	return 0, false, false
}

// seekPreviewAvailable reports whether a seek key can preview its target:
// a song is loaded, its position is known, and a real player will act on
// the seek. Otherwise the key seeks at once, as it always has.
func (m Model) seekPreviewAvailable() bool {
	if _, ok := m.player.(commandPlayer); !ok {
		return false
	}
	if m.playback.state != playback.StatePlaying && m.playback.state != playback.StatePaused {
		return false
	}
	return m.playback.currentHash != "" && m.playback.totalSecs > 0
}

// seekPreviewLive reports whether a pending seek still belongs to the song
// that is playing.
func (m Model) seekPreviewLive() bool {
	s := m.seek
	return s.pending && s.hash == m.playback.currentHash && s.request == m.playback.playRequestSeq &&
		(m.playback.state == playback.StatePlaying || m.playback.state == playback.StatePaused)
}

// handleSeekKey moves the pending seek target for a seek key press and
// schedules the commit. ok is false for actions that are not seeks.
func (m Model) handleSeekKey(action keys.Action) (Model, tea.Cmd, bool) {
	offset, accel, ok := m.seekKeyOffset(action)
	if !ok {
		return m, nil, false
	}
	if !m.seekPreviewAvailable() {
		next, cmd, _ := m.handleSeek(offset)
		return next.(Model), cmd, true
	}
	now := m.seek.now()
	if m.seek.lastPress.IsZero() || now.Sub(m.seek.lastPress) > seekStreakGap {
		m.seek.streakStart = now
	}
	m.seek.lastPress = now
	if accel {
		offset *= seekAccelScale(now.Sub(m.seek.streakStart))
	}
	base := m.seekPreviewBase(now)
	m.seek.target = math.Max(0, math.Min(base+offset, m.playback.totalSecs))
	m.seek.pending = true
	m.seek.hash, m.seek.request = m.playback.currentHash, m.playback.playRequestSeq
	m.seek.seq++
	seq := m.seek.seq
	return m, tea.Tick(seekPreviewDebounce, func(time.Time) tea.Msg { return seekCommitMsg{seq: seq} }), true
}

// seekPreviewBase is where the next press moves from: the pending target,
// then a seek mpv has not reported reaching yet, then the shown position.
func (m Model) seekPreviewBase(now time.Time) float64 {
	switch {
	case m.seekPreviewLive():
		return m.seek.target
	case m.seekSettling(now):
		return m.seek.settleTarget
	default:
		return m.displayElapsed(time.Now())
	}
}

func seekAccelScale(held time.Duration) float64 {
	for _, step := range seekAccelSteps {
		if held >= step.after {
			return step.scale
		}
	}
	return 1
}

// handleSeekCommit seeks to the pending target once the newest press has
// gone quiet; older ticks are ignored.
func (m Model) handleSeekCommit(msg seekCommitMsg) (Model, tea.Cmd) {
	if msg.seq != m.seek.seq || !m.seek.pending {
		return m, nil
	}
	return m.commitSeekPreview()
}

// commitSeekPreview asks mpv for the pending target now. A pending seek
// aimed at a song that is no longer playing is dropped instead.
func (m Model) commitSeekPreview() (Model, tea.Cmd) {
	if !m.seek.pending {
		return m, nil
	}
	live := m.seekPreviewLive()
	target := m.seek.target
	m.seek.pending = false
	if !live {
		return m, nil
	}
	cp, ok := m.player.(commandPlayer)
	if !ok {
		return m, nil
	}
	if err := cp.SeekAbsolute(target); err != nil {
		slog.Warn("seek failed", "err", err)
		return m, nil
	}
	return m.noteSeekLanded(target), m.sessionPositionCmd()
}

// cancelSeekPreview forgets a pending seek, for a seek that supersedes it.
func (m Model) cancelSeekPreview() Model {
	m.seek.pending = false
	return m
}

// noteSeekLanded records a seek mpv was just given: the position jumps to
// the target, listening time does not count the jump, media controls hear
// of it, and position reports from before the seek are ignored for a while.
func (m Model) noteSeekLanded(target float64) Model {
	m.playback.elapsedSecs = target
	m.playback.lastSampleSecs = target
	m.control.seeks++
	m.seek.settleTarget = target
	m.seek.settleUntil = m.seek.now().Add(seekSettleWindow)
	return m
}

func (m Model) seekSettling(now time.Time) bool {
	return !m.seek.settleUntil.IsZero() && now.Before(m.seek.settleUntil)
}

// staleSeekReading reports whether a position mpv reported is from before
// the last seek finished; one near the target ends the settling early.
func (m Model) staleSeekReading(elapsed float64) (Model, bool) {
	if !m.seekSettling(m.seek.now()) {
		return m, false
	}
	if math.Abs(elapsed-m.seek.settleTarget) > maxListenDeltaSecs {
		return m, true
	}
	m.seek.settleUntil = time.Time{}
	return m, false
}

// seekPreviewArrow is the direction marker drawn beside a pending target.
func (m Model) seekPreviewArrow() string {
	if !m.seekPreviewLive() {
		return ""
	}
	if m.seek.target < m.playback.elapsedSecs {
		return "←"
	}
	return "→"
}

// commitSeekOnOtherKey sends a pending seek at once when a key press other
// than a seek key arrives, so the next action works from the new position.
func (m Model) commitSeekOnOtherKey(prev Model) (Model, tea.Cmd) {
	if !m.seek.pending || m.seek.seq != prev.seek.seq {
		return m, nil
	}
	return m.commitSeekPreview()
}
