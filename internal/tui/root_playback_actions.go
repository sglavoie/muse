package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

type playbackControl uint8

const (
	playbackControlPause playbackControl = iota
	playbackControlPlay
	playbackControlNext
	playbackControlPrevious
)

// playbackControlResultMsg reports an asynchronously ordered transport call.
// before/after let a failed optimistic pause toggle restore the state it
// replaced, provided no unrelated player event has moved the model elsewhere.
type playbackControlResultMsg struct {
	request uint64
	control playbackControl
	before  playback.State
	after   playback.State
	err     error
	ran     bool
}

// playbackControlCmd queues a transport operation behind every earlier player
// request. The wait occurs in this tea.Cmd, never on the Bubble Tea Update
// goroutine.
func (m *Model) playbackControlCmd(
	control playbackControl,
	call func() error,
	before playback.State,
	after playback.State,
) tea.Cmd {
	// Play, Next, and Previous all ask the player to put a row on; Pause asks for
	// the opposite. Recording the request here rather than at each key case keeps
	// it beside the result handler that withdraws it when the call is refused.
	if control != playbackControlPause {
		*m = m.radioNoteStartRequested()
	}
	gate := m.playback.requests
	req := gate.dispatchControl(m.playback.requestSeq)
	m.playback.requestSeq = req
	return func() tea.Msg {
		err, ran := gate.run(req, call)
		return playbackControlResultMsg{
			request: req,
			control: control,
			before:  before,
			after:   after,
			err:     err,
			ran:     ran,
		}
	}
}

func (m Model) handlePlaybackControlResultMsg(msg playbackControlResultMsg) (Model, tea.Cmd) {
	if msg.request != m.playback.requestSeq || !msg.ran {
		return m, nil
	}
	if msg.err != nil {
		if m.playback.state == msg.after {
			m.playback.state = msg.before
		}
		// No row went on, so the request this result answers is not the start that
		// lifts a deliberate stop's refill suspension. The identity guard above has
		// already discarded superseded results, so the request withdrawn here is
		// the one the failure belongs to.
		if msg.control != playbackControlPause {
			m = m.radioNoteStartRefused()
		}
		label := playbackControlLabel(msg.control)
		slog.Warn(label+" failed", "err", msg.err)
		m.statusBar.Notify(label + " failed: " + msg.err.Error())
		return m, nil
	}
	if msg.control == playbackControlPause {
		return m, m.sessionPositionCmd()
	}
	return m, nil
}

func playbackControlLabel(control playbackControl) string {
	switch control {
	case playbackControlPause:
		return "Pause"
	case playbackControlPlay:
		return "Play"
	case playbackControlNext:
		return "Next"
	case playbackControlPrevious:
		return "Previous"
	default:
		return "Playback control"
	}
}

// handlePlaybackKeyAction handles playback-related key actions (play/pause,
// stop, next, previous, seek, volume, repeat, shuffle, preview, now-playing).
// It returns true in the third value when the action was handled.
func (m Model) handlePlaybackKeyAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionTrackInfo:
		if m.overlay.trackInfo != nil {
			m.overlay.trackInfo = nil
			return m, nil, true
		}
		track, ok := m.focusedTrackForPreview()
		if !ok {
			m.statusBar.Notify("No track to inspect")
			return m, nil, true
		}
		if track.Path == "" {
			if t, ok := m.library.Index.FindByHash(track.Hash); ok {
				track = t
			}
		}
		if track.Path == "" {
			m.statusBar.Notify("Track path unknown")
			return m, nil, true
		}
		return m, overlay.TrackInfoCmd(m.appCtx, track.Path, track.Hash, track.PlayCount), true
	case keys.ActionNowPlayingInfo:
		if m.overlay.nowPlaying != nil {
			m.overlay.nowPlaying = nil
			return m, nil, true
		}
		if m.playback.state == playback.StateStopped || m.playback.state == playback.StateIdle {
			m.statusBar.Notify("No track playing")
			return m, nil, true
		}
		overlay := overlay.NowPlayingOverlay{
			Artist:   m.statusBar.Artist,
			Title:    m.statusBar.Title,
			Elapsed:  m.playback.elapsedSecs,
			Total:    m.playback.totalSecs,
			State:    m.playback.state,
			TagsLine: m.buildTagsLine(m.playback.currentHash),
		}
		if m.playback.currentHash != "" {
			if t, ok := m.library.Index.FindByHash(m.playback.currentHash); ok {
				if overlay.Artist == "" {
					overlay.Artist = t.Artist
				}
				if overlay.Title == "" {
					overlay.Title = t.Title
				}
				overlay.Album = t.Album
				overlay.PlayCount = t.PlayCount
			}
		}
		m.overlay.nowPlaying = &overlay
		return m, nil, true
	case keys.ActionPreview:
		if m.overlay.preview != nil {
			m, cmd := m.dismissPreview()
			return m, cmd, true
		}
		if m.overlay.previewStarting {
			return m, nil, true
		}
		track, ok := m.focusedTrackForPreview()
		if !ok {
			m.statusBar.Notify("No track to preview")
			return m, nil, true
		}
		m.overlay.previewStarting = true
		m.overlay.previewWasPlaying = m.playback.state == playback.StatePlaying
		if m.overlay.previewWasPlaying && m.player != nil {
			m.playback.state = playback.StatePaused
			return m, m.pauseForPreviewCmd(track), true
		}
		return m, m.startPreviewCmd(track, 0), true
	case keys.ActionPlayPause:
		if m.player != nil {
			switch {
			case m.playback.state == playback.StatePlaying:
				before := m.playback.state
				m.playback.state = playback.StatePaused
				cmd := m.playbackControlCmd(
					playbackControlPause,
					m.player.Pause,
					before,
					m.playback.state,
				)
				return m, cmd, true
			case len(m.queue.Entries) == 0:
				m.statusBar.Notify("Queue is empty")
				return m, nil, true
			default:
				if m.playback.state == playback.StatePaused {
					before := m.playback.state
					m.playback.state = playback.StatePlaying
					cmd := m.playbackControlCmd(
						playbackControlPlay,
						m.player.Play,
						before,
						m.playback.state,
					)
					return m, cmd, true
				} else {
					idx := m.playback.currentIndex
					if idx < 0 || idx >= len(m.queue.Entries) {
						idx = 0
					}
					return m, func() tea.Msg { return playback.PlayQueueIndexMsg{Index: idx} }, true
				}
			}
		}
		return m, nil, true
	case keys.ActionStop:
		if cp, ok := m.player.(commandPlayer); ok {
			// An explicit stop supersedes every selection still queued behind it,
			// whether or not the stop itself succeeds: a dispatched-but-unrun
			// selection reaching the player after this would start audio the user
			// has just asked to end.
			if err := m.playRequestExclusive(cp.Stop); err != nil {
				slog.Warn("stop failed", "err", err)
				m.statusBar.Notify("Stop failed: " + err.Error())
			} else {
				m = m.endCurrentListen()
				m.playback.state = playback.StateStopped
				m.playback.elapsedSecs = 0
				m.playback.totalSecs = 0
				// An explicit stop abandons any transition still in flight, so
				// nothing is waiting on a selected row's start-file any more. The
				// stop can beat that event out of mpv, and leaving the barrier up
				// for a report that will never come would refuse the next one.
				m.playback.lowerSelectionBarrier()
				// An explicit stop never sets stopAfterCurrent, so the refill gate
				// would read an idle player with a short queue as ordinary
				// exhaustion. Retiring the refill here is what keeps the next tick
				// from restarting audio the user just ended.
				m = m.radioNoteDeliberateStop()
				if m.playback.stopAfterCurrent {
					m.playback.stopAfterCurrent = false
					if sat, ok := m.player.(stopAfterToggler); ok {
						sat.SetStopAfterCurrent(false)
					}
				}
			}
		}
		return m, nil, true
	case keys.ActionResetPosition:
		if cp, ok := m.player.(commandPlayer); ok {
			if err := cp.SeekAbsolute(0); err != nil {
				slog.Warn("reset position failed", "err", err)
			} else {
				m.playback.elapsedSecs = 0
			}
		}
		return m, m.sessionPositionCmd(), true
	case keys.ActionNext:
		if cp, ok := m.player.(commandPlayer); ok {
			cmd := m.playbackControlCmd(
				playbackControlNext,
				cp.Next,
				m.playback.state,
				m.playback.state,
			)
			return m, cmd, true
		}
		return m, nil, true
	case keys.ActionPrevious:
		if cp, ok := m.player.(commandPlayer); ok {
			cmd := m.playbackControlCmd(
				playbackControlPrevious,
				cp.Previous,
				m.playback.state,
				m.playback.state,
			)
			return m, cmd, true
		}
		return m, nil, true
	case keys.ActionSeekBack:
		return m.handleSeek(-m.cfg.SeekOffsetShort)
	case keys.ActionSeekForward:
		return m.handleSeek(m.cfg.SeekOffsetShort)
	case keys.ActionSeekBackLong:
		return m.handleSeek(-m.cfg.SeekOffsetLong)
	case keys.ActionSeekForwardLong:
		return m.handleSeek(m.cfg.SeekOffsetLong)
	case keys.ActionVolumeUp:
		return m.handleVolume(1)
	case keys.ActionVolumeDown:
		return m.handleVolume(-1)
	case keys.ActionRepeatCycle:
		switch m.playback.repeatMode {
		case playback.RepeatOff:
			m.playback.repeatMode = playback.RepeatAll
		case playback.RepeatAll:
			m.playback.repeatMode = playback.RepeatOne
		case playback.RepeatOne:
			m.playback.repeatMode = playback.RepeatOff
		}
		if rs, ok := m.player.(repeatShuffler); ok {
			if err := rs.SetRepeat(m.playback.repeatMode.String()); err != nil {
				slog.Warn("set repeat failed", "err", err)
			}
		}
		if m.sessionWriter != nil {
			if err := m.sessionWriter.WriteRepeatMode(m.playback.repeatMode.String()); err != nil {
				slog.Warn("write repeat mode failed", "err", err)
			}
		}
		m.statusBar.PushMsg(m.modeToggleMsg("Repeat", playback.ModeIndexRepeat), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, statusbar.NotifInfo)
		return m, nil, true
	case keys.ActionShuffleToggle:
		m.playback.shuffle = !m.playback.shuffle
		label := playback.ModeState(m.playback.shuffle)
		// Surface a runtime failure to the user: the traversal order did not
		// change, so revert the toggle and do not persist a mismatched flag.
		if rs, ok := m.player.(repeatShuffler); ok {
			if err := rs.SetShuffle(m.playback.shuffle); err != nil {
				slog.Warn("set shuffle failed", "err", err)
				m.playback.shuffle = !m.playback.shuffle
				m.statusBar.Notify("Shuffle failed: " + err.Error())
				return m, nil, true
			}
		}
		// A persistence failure leaves runtime shuffle active but the flag will
		// not survive a restart — tell the user rather than failing silently.
		if m.sessionWriter != nil {
			if err := m.sessionWriter.WriteShuffle(m.playback.shuffle); err != nil {
				slog.Warn("write shuffle failed", "err", err)
				m.statusBar.Notify(fmt.Sprintf("Shuffle %s (not saved — won't survive restart)", label))
				return m, nil, true
			}
		}
		m.statusBar.PushMsg(m.modeToggleMsg("Shuffle", playback.ModeIndexShuffle), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, statusbar.NotifInfo)
		return m, nil, true
	case keys.ActionStopAfterCurrent:
		if m.playback.state != playback.StatePlaying && m.playback.state != playback.StatePaused {
			m.statusBar.Notify("No track playing")
			return m, nil, true
		}
		m.playback.stopAfterCurrent = !m.playback.stopAfterCurrent
		if sat, ok := m.player.(stopAfterToggler); ok {
			sat.SetStopAfterCurrent(m.playback.stopAfterCurrent)
		}
		m.statusBar.PushMsg(m.modeToggleMsg("Stop after current", playback.ModeIndexStopAfter), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, statusbar.NotifInfo)
		return m, nil, true
	case keys.ActionRadioToggle:
		return m.radioToggle()
	}
	return m, nil, false
}

func (m Model) handleSeek(seconds float64) (tea.Model, tea.Cmd, bool) {
	if cp, ok := m.player.(commandPlayer); ok {
		if err := cp.SeekRelative(seconds); err != nil {
			slog.Warn("seek failed", "err", err)
		}
		return m, m.sessionPositionCmd(), true
	}
	return m, nil, true
}

func (m Model) handleVolume(sign int) (tea.Model, tea.Cmd, bool) {
	cp, ok := m.player.(commandPlayer)
	if !ok {
		return m, nil, true
	}
	delta := sign * m.cfg.VolumeIncrement
	if err := cp.SetVolumeRelative(float64(delta)); err != nil {
		slog.Warn("volume change failed", "err", err)
	} else {
		m.playback.volume = max(0, min(m.playback.volume+delta, 100))
		if m.sessionWriter != nil {
			if err := m.sessionWriter.WriteVolume(m.playback.volume); err != nil {
				slog.Warn("write volume failed", "err", err)
			}
		}
	}
	m.statusBar.PushMsg(fmt.Sprintf("Volume: %d%%", m.playback.volume), time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, statusbar.NotifInfo)
	return m, nil, true
}

// modeToggleMsg renders the feedback for one mode toggle as
// "<label>: <state> (<glyph>)" — e.g. "Shuffle: on (S)" — so the notification
// teaches the letter the status-bar cluster shows. State and glyph are read
// from modeIndicators, which is why they cannot drift from the cluster; the
// label stays caller-supplied because these notifications predate the indicator
// names and say "Radio mode" where the indicator says "Radio (Auto-DJ)".
func (m Model) modeToggleMsg(label string, index int) string {
	ind := playback.ModeIndicators(m.playback.repeatMode, m.playback.shuffle, m.playback.stopAfterCurrent, m.radio.enabled)[index]
	return label + ": " + ind.State + " (" + ind.Glyph + ")"
}
