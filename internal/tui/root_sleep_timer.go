package tui

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// This file wires the sleep timer (sleep_timer.go) into the Model: the
// "Sleep timer…" picker, the countdown's own tick, the fade into pause, and
// the gestures that cancel it.
//
// The countdown never rides the motion frame loop. It schedules one tick for
// the moment its label next changes — once a minute, then once a second for
// the last minute — so an armed timer costs at most one redraw a second and
// leaves the motion loop free to stop. The timer is session state only: it
// survives view changes but not a restart.

// sleepFader is implemented by player.Manager: a long fade to silence that ends
// paused at the user's volume, cancellable through ctx.
type sleepFader interface {
	FadeOutAndPause(ctx context.Context, d time.Duration) error
}

// sleepState is the Model's sleep timer: the state machine, the clock it is
// read against (time.Now unless a test injects one), and the cancel function of
// the fade in flight.
type sleepState struct {
	timer      sleepTimer
	now        func() time.Time
	cancelFade context.CancelFunc
}

// sleepTimerMsg is every message this file handles, so updateInner needs one
// route for all of them.
type sleepTimerMsg interface{ isSleepTimerMsg() }

// openSleepTimerPickerMsg opens the picker of sleep timer choices.
type openSleepTimerPickerMsg struct{}

// sleepTimerChoiceMsg applies one picker choice.
type sleepTimerChoiceMsg struct{ choice sleepChoice }

// sleepTickMsg wakes the countdown when its label next changes.
type sleepTickMsg struct{ gen uint64 }

// sleepFadeDoneMsg reports the end of the fade the timer started.
type sleepFadeDoneMsg struct {
	gen uint64
	err error
}

// stopAfterCueResultMsg reports the cue that follows a stop-after-current; see
// root_stop_after.go.
type stopAfterCueResultMsg struct {
	request uint64
	err     error
	ran     bool
}

func (openSleepTimerPickerMsg) isSleepTimerMsg() {}
func (sleepTimerChoiceMsg) isSleepTimerMsg()     {}
func (sleepTickMsg) isSleepTimerMsg()            {}
func (sleepFadeDoneMsg) isSleepTimerMsg()        {}
func (stopAfterCueResultMsg) isSleepTimerMsg()   {}

// handleSleepTimerMsg routes the sleep timer's messages.
func (m Model) handleSleepTimerMsg(msg sleepTimerMsg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case openSleepTimerPickerMsg:
		return m.openSleepTimerPicker()
	case sleepTimerChoiceMsg:
		return m.applySleepChoice(msg.choice)
	case sleepTickMsg:
		return m.handleSleepTick(msg)
	case sleepFadeDoneMsg:
		return m.handleSleepFadeDone(msg)
	case stopAfterCueResultMsg:
		return m.handleStopAfterCueResult(msg)
	}
	return m, nil
}

// sleepNow is the clock the timer is read against.
func (m Model) sleepNow() time.Time {
	if m.sleep.now != nil {
		return m.sleep.now()
	}
	return time.Now()
}

// sleepTimerView is the timer as it should be shown. "End of this song" is
// carried by stop-after-current, so once that flag has gone — fired, toggled
// off, or cleared by a skip — the timer has nothing left to wait for.
func (m Model) sleepTimerView() sleepTimer {
	t := m.sleep.timer
	if t.mode == sleepEndOfSong && !m.playback.stopAfterCurrent {
		return t.off()
	}
	return t
}

// sleepTimerLabel is the marker for the status bar and Now Playing.
func (m Model) sleepTimerLabel() string {
	return m.sleepTimerView().label(m.sleepNow())
}

// sleepTimerPaletteCommand is the "Sleep timer…" palette entry. It is
// palette-only, like the metadata commands: every key that would suit it is
// already bound.
func (m Model) sleepTimerPaletteCommand() palette.PaletteCommand {
	cmd := paletteMessageCmd("sleep.timer", "play", "Sleep timer…", "", openSleepTimerPickerMsg{})
	cmd.Aliases = []string{"timer", "bedtime", "pause later"}
	cmd.Detail = "Pause after a while, at the end of this song, or at the end of the queue"
	if label := m.sleepTimerLabel(); label != "" {
		cmd.CurrentState = label
	}
	return cmd
}

// openSleepTimerPicker reuses the command palette as the picker, listing the
// choices in their natural order.
func (m Model) openSleepTimerPicker() (tea.Model, tea.Cmd) {
	choices := sleepChoices()
	commands := make([]palette.PaletteCommand, 0, len(choices))
	current := m.sleepTimerView()
	for _, choice := range choices {
		command := paletteMessageCmd(choice.id, "play", choice.name, "", sleepTimerChoiceMsg{choice: choice})
		command.Aliases = []string{"sleep"}
		if choice.mode == sleepOff {
			command.Enabled = current.armed
			command.DisabledReason = "The sleep timer is already off"
		}
		if choice.mode == sleepEndOfSong {
			command.Enabled = m.playbackHasCurrentTrack
			command.DisabledReason = "Start a song first"
		}
		commands = append(commands, command)
	}
	cp, cmd := palette.NewCommandPalette(commands, nil)
	m.overlay.commandPalette = &cp
	return m, cmd
}

// playbackHasCurrentTrack reports whether there is a song for "end of this
// song" to wait for.
func (m Model) playbackHasCurrentTrack() bool {
	return m.playback.state == playback.StatePlaying || m.playback.state == playback.StatePaused
}

// applySleepChoice arms or disarms the timer. Every choice replaces the one
// before it, including a fade already under way.
func (m Model) applySleepChoice(choice sleepChoice) (tea.Model, tea.Cmd) {
	prev := m.sleepTimerView()
	m = m.stopSleepFade()
	if prev.mode == sleepEndOfSong && choice.mode != sleepEndOfSong {
		m = m.setStopAfterCurrent(false)
	}
	switch choice.mode {
	case sleepTimed:
		m.sleep.timer = m.sleep.timer.armTimed(m.sleepNow(), choice.duration)
		m.statusBar.Notify("Sleep timer: pausing in " + formatSleepMinutes(choice.duration))
		return m, m.sleepTickCmd()
	case sleepEndOfSong:
		return m.armSleepAtEndOfSong("Sleep timer: pausing at the end of this song"), nil
	case sleepEndOfQueue:
		if m.playback.repeatMode == playback.RepeatOne {
			// Repeat-one never reaches the end of the queue, so the song that
			// is looping is the only end there is.
			return m.armSleepAtEndOfSong("Sleep timer: repeat one is on, so pausing at the end of this song"), nil
		}
		m.sleep.timer = m.sleep.timer.armUntil(sleepEndOfQueue)
		m.statusBar.Notify("Sleep timer: stopping at the end of the queue")
		return m, nil
	default:
		m.sleep.timer = m.sleep.timer.off()
		if prev.armed() {
			m.statusBar.Notify("Sleep timer off")
		}
		return m, nil
	}
}

// armSleepAtEndOfSong arms stop-after-current on the timer's behalf.
func (m Model) armSleepAtEndOfSong(notice string) Model {
	if !m.playbackHasCurrentTrack() {
		m.sleep.timer = m.sleep.timer.off()
		m.statusBar.Notify("No track playing")
		return m
	}
	m = m.setStopAfterCurrent(true)
	m.sleep.timer = m.sleep.timer.armUntil(sleepEndOfSong)
	m.statusBar.Notify(notice)
	return m
}

// formatSleepMinutes renders a picker duration for the confirmation.
func formatSleepMinutes(d time.Duration) string {
	minutes := int(d / time.Minute)
	if minutes == 1 {
		return "1 minute"
	}
	return strconv.Itoa(minutes) + " minutes"
}

// sleepTickCmd schedules the countdown's next wake-up.
func (m Model) sleepTickCmd() tea.Cmd {
	t := m.sleep.timer
	if t.mode != sleepTimed || t.fading {
		return nil
	}
	gen := t.gen
	return tea.Tick(t.nextTick(m.sleepNow()), func(time.Time) tea.Msg { return sleepTickMsg{gen: gen} })
}

// handleSleepTick redraws the countdown and, once it has run out, starts the
// fade. A tick from an abandoned arming is dropped, which is what ends its
// chain.
func (m Model) handleSleepTick(msg sleepTickMsg) (tea.Model, tea.Cmd) {
	t := m.sleep.timer
	if msg.gen != t.gen || t.mode != sleepTimed || t.fading {
		return m, nil
	}
	if !t.due(m.sleepNow()) {
		return m, m.sleepTickCmd()
	}
	if m.playback.state != playback.StatePlaying {
		// Nothing is playing, so there is nothing to fade or pause.
		m.sleep.timer = t.off()
		m.statusBar.Notify("Sleep timer ended")
		return m, nil
	}
	return m.startSleepFade()
}

// startSleepFade begins the fade into pause. A player without a fade simply
// pauses.
func (m Model) startSleepFade() (Model, tea.Cmd) {
	m.sleep.timer = m.sleep.timer.beginFade()
	gen := m.sleep.timer.gen
	ctx, cancel := context.WithCancel(m.appContext())
	m.sleep.cancelFade = cancel
	player := m.player
	m.statusBar.Notify("Sleep timer: fading out — press play to keep listening")
	return m, func() tea.Msg {
		var err error
		switch p := player.(type) {
		case sleepFader:
			err = p.FadeOutAndPause(ctx, sleepFadeDuration)
		case nil:
		default:
			err = p.Pause()
		}
		return sleepFadeDoneMsg{gen: gen, err: err}
	}
}

// appContext is the application context, or Background for models built
// without one.
func (m Model) appContext() context.Context {
	if m.appCtx != nil {
		return m.appCtx
	}
	return context.Background()
}

// handleSleepFadeDone settles a finished fade: playback is paused and the timer
// is spent.
func (m Model) handleSleepFadeDone(msg sleepFadeDoneMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.sleep.timer.gen || !m.sleep.timer.fading {
		return m, nil
	}
	if m.sleep.cancelFade != nil {
		m.sleep.cancelFade()
		m.sleep.cancelFade = nil
	}
	m.sleep.timer = m.sleep.timer.off()
	if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
		slog.Warn("sleep timer fade failed", "err", msg.err)
		m.statusBar.NotifyError("Sleep timer could not pause: " + msg.err.Error())
		return m, nil
	}
	if m.playback.state == playback.StatePlaying {
		m.playback.state = playback.StatePaused
	}
	m.statusBar.Notify("Sleep timer: paused")
	return m, m.sessionPositionCmd()
}

// stopSleepFade cancels a fade in flight, which brings the volume back and
// leaves playback running. The timer itself is left to the caller.
func (m Model) stopSleepFade() Model {
	if m.sleep.cancelFade != nil {
		m.sleep.cancelFade()
		m.sleep.cancelFade = nil
	}
	return m
}

// cancelSleepTimer disarms the timer and cancels any fade, notifying with
// notice when it is non-empty.
func (m Model) cancelSleepTimer(notice string) Model {
	if !m.sleep.timer.armed() {
		return m
	}
	m = m.stopSleepFade()
	m.sleep.timer = m.sleep.timer.off()
	if notice != "" {
		m.statusBar.Notify(notice)
	}
	return m
}

// sleepFading reports whether the timer is fading playback out right now.
func (m Model) sleepFading() bool {
	return m.sleep.timer.mode == sleepTimed && m.sleep.timer.fading
}

// sleepHoldsQueueEnd reports whether the timer is waiting for the end of the
// queue, which means neither Auto-DJ nor repeat-all may extend it.
func (m Model) sleepHoldsQueueEnd() bool {
	return m.sleep.timer.mode == sleepEndOfQueue
}

// settleSleepAtPlaybackEnd spends the timer when the end it was waiting for
// arrives: the song (through stop-after-current) or the queue. A fade still
// running when the queue runs out has nothing left to fade.
func (m Model) settleSleepAtPlaybackEnd(wasStopAfter bool) Model {
	switch m.sleep.timer.mode {
	case sleepEndOfSong:
		if wasStopAfter {
			m.sleep.timer = m.sleep.timer.off()
			m.statusBar.Notify("Sleep timer: paused at the end of the song")
		}
	case sleepEndOfQueue:
		m.sleep.timer = m.sleep.timer.off()
	case sleepTimed:
		if m.sleep.timer.fading {
			m = m.cancelSleepTimer("")
		}
	}
	return m
}

// interceptSleepFade lets a play gesture during the fade keep the music going:
// it cancels the fade, which brings the volume back, and disarms the timer. It
// reports whether the gesture was consumed.
func (m Model) interceptSleepFade(action keys.Action) (Model, bool) {
	if !m.sleepFading() {
		return m, false
	}
	switch action {
	case keys.ActionPlayPause:
		return m.cancelSleepTimer("Sleep timer cancelled — still playing"), true
	case keys.ActionStop:
		return m.cancelSleepTimer("Sleep timer cancelled"), false
	}
	return m, false
}
