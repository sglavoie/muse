package player

import (
	"context"
	"errors"
	"time"
)

// This file holds the two ways playback is brought to rest on purpose rather
// than by running out of queue: cueing the successor paused after a
// stop-after-current has fired, and the sleep timer's long fade into a pause.

// sleepFadeStep is the interval between volume steps of a sleep-timer fade.
// A quarter second is fine enough that a 15-second fade sounds continuous and
// coarse enough that it costs a few dozen IPC round trips.
const sleepFadeStep = 250 * time.Millisecond

// sleepFadeResumeSteps is how many pauseFadeStep steps a cancelled fade takes
// to come back to the full level, so cancelling sounds like a quick swell
// rather than a jump.
const sleepFadeResumeSteps = 4

// CueNext loads the entry that would have played next, paused at its start, so
// a stop-after-current leaves the queue ready to resume rather than finished.
//
// "Next" is what the auto-advance would have chosen: the following row of the
// sequential traversal, or the next identity of the shuffle cycle. Under
// repeat-one it is the current row again, and under repeat-all an exhausted
// traversal restarts the cycle. A queue with nothing left reports
// ErrPlaybackFailed and loads nothing, which the caller treats as an ordinary
// end.
//
// The cue is an explicit queue-owned load like any other, so it goes through
// the same playlist transaction and makes the traversal live again; only the
// pause option differs, and only for loads made inside this transaction.
func (m *Manager) CueNext() error {
	m.mu.RLock()
	idx := m.currentIndex
	mode := m.repeatMode
	notStarted := m.client == nil && m.cmdQueue == nil
	m.mu.RUnlock()
	if notStarted {
		return ErrNotConnected
	}
	_, err := m.runPlaylistTxnIndex(func() (int, error) {
		m.cuePaused = true
		defer func() { m.cuePaused = false }()
		return m.cueLocked(idx, mode)
	})
	return err
}

// cueLocked is CueNext's transaction body.
//
// The caller must hold m.playlistMu.
func (m *Manager) cueLocked(idx int, mode string) (int, error) {
	if mode == "one" {
		err := m.cueCurrentLocked(idx)
		if err == nil {
			return idx, nil
		}
		if !errors.Is(err, ErrMissingFile) {
			return -1, err
		}
		// The looping song is gone from disk: fall through to its successor.
	}
	next, err := m.advanceAfterCurrentLocked(idx + 1)
	if errors.Is(err, errTraversalExhausted) && mode == "all" {
		return -1, m.restartQueueCycleLocked()
	}
	return next, err
}

// cueCurrentLocked reloads the row at idx without moving the traversal, which
// is repeat-one's "next song". A row that no longer exists reports
// ErrMissingFile, like a file missing on disk, so the caller moves on.
//
// The caller must hold m.playlistMu.
func (m *Manager) cueCurrentLocked(idx int) error {
	m.mu.RLock()
	ok := idx >= 0 && idx < len(m.queue)
	var entry QueueTrack
	if ok {
		entry = m.queue[idx]
	}
	marker := m.marker
	m.mu.RUnlock()
	if !ok {
		return ErrMissingFile
	}
	return m.replaceLocked(entry, marker, nil)
}

// loadPauseOption is the per-file pause option of a queue-owned load.
func loadPauseOption(paused bool) string {
	if paused {
		return "pause=yes"
	}
	return "pause=no"
}

// settleLoadPause mirrors the pause state a queue-owned load left mpv in: a
// cue stays paused, every other load is unpaused in case mpv was paused before.
func (m *Manager) settleLoadPause(c *IPCClient, paused bool) {
	if paused {
		m.mirrorPaused()
		return
	}
	m.bestEffortUnpauseWith(c)
}

// loopMode is the repeat mode mpv's loop properties should express: the one
// SetRepeat recorded, except that an armed stop-after-current suspends
// repeat-one's file loop, because a file that loops never reaches the end the
// stop is waiting for.
func (m *Manager) loopMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopAfterCurrent && m.repeatMode == "one" {
		return "none"
	}
	return m.repeatMode
}

// reapplyLoopMode re-sends mpv's loop properties after stop-after-current was
// armed, cleared, or fired. Only repeat-one depends on the flag, so every other
// mode is left alone and costs no IPC.
func (m *Manager) reapplyLoopMode() {
	m.mu.RLock()
	repeatOne := m.repeatMode == "one"
	m.mu.RUnlock()
	if !repeatOne {
		return
	}
	_ = m.admit(func(c *IPCClient) error {
		return sendRepeat(c, m.loopMode())
	})
}

// mirrorPaused records a pause mpv has been told to apply, so an auto-restart
// firing before the property-change event arrives restores it paused.
func (m *Manager) mirrorPaused() {
	m.mu.Lock()
	m.lastPaused = true
	m.mu.Unlock()
}

// FadeOutAndPause lowers the volume to silence over d, pauses, and then puts
// the volume back, so the next play starts at the level the user chose. It is
// the sleep timer's ending.
//
// Each step reads the user's level afresh, so a volume change made during the
// fade is honored. Cancelling ctx stops the fade, brings the volume back up
// quickly and leaves playback running; it returns ctx's error. Without a known
// absolute volume there is nothing to scale, so it pauses straight away.
func (m *Manager) FadeOutAndPause(ctx context.Context, d time.Duration) error {
	m.mu.RLock()
	known := m.volumeKnown
	m.mu.RUnlock()
	if known {
		if err := m.fadeOut(ctx, d); err != nil {
			return err
		}
	}
	c, err := m.ipc()
	if err != nil {
		return err
	}
	_, pauseErr := c.SendCommand(cmdSetProperty, propPause, true)
	if pauseErr == nil {
		m.mirrorPaused()
	}
	if known {
		// Paused, so restoring the level is silent.
		m.restoreVolume(c)
	}
	return pauseErr
}

// fadeOut ramps the volume from the user's level to silence over d. A
// cancelled or failed fade restores the level before returning.
func (m *Manager) fadeOut(ctx context.Context, d time.Duration) error {
	steps := max(1, int(d/sleepFadeStep))
	timer := time.NewTimer(sleepFadeStep)
	defer timer.Stop()
	frac := 1.0
	for i := 1; i <= steps; i++ {
		select {
		case <-ctx.Done():
			m.resumeFromFade(frac)
			return ctx.Err()
		case <-timer.C:
		}
		timer.Reset(sleepFadeStep)
		c, err := m.ipc()
		if err != nil {
			return err
		}
		frac = 1 - float64(i)/float64(steps)
		if _, err := c.SendCommand(cmdSetProperty, propVolume, m.currentLevel()*frac); err != nil {
			m.restoreVolume(c)
			return err
		}
	}
	return ctx.Err()
}

// resumeFromFade brings a cancelled fade back from frac of the user's level to
// all of it.
func (m *Manager) resumeFromFade(frac float64) {
	c, err := m.ipc()
	if err != nil {
		return
	}
	for i := 1; i <= sleepFadeResumeSteps; i++ {
		f := frac + (1-frac)*float64(i)/sleepFadeResumeSteps
		if _, err := c.SendCommand(cmdSetProperty, propVolume, m.currentLevel()*f); err != nil {
			return
		}
		if i < sleepFadeResumeSteps {
			time.Sleep(pauseFadeStep)
		}
	}
}

// restoreVolume puts mpv back at the level the Manager last applied.
func (m *Manager) restoreVolume(c *IPCClient) {
	_, _ = c.SendCommand(cmdSetProperty, propVolume, m.currentLevel())
}

func (m *Manager) currentLevel() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastVolume
}
