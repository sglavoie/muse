package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
)

// stubRepeatShuffler implements repeatShuffler (SetRepeat/SetShuffle) on top of
// playPauser so the shuffle-toggle action can be exercised. SetShuffle returns
// shuffleErr when set, to test runtime-failure surfacing.
type stubRepeatShuffler struct {
	stubPlayer
	shuffleCalls []bool
	shuffleErr   error
}

func (s *stubRepeatShuffler) SetRepeat(string) error { return nil }

func (s *stubRepeatShuffler) SetShuffle(enabled bool) error {
	if s.shuffleErr != nil {
		return s.shuffleErr
	}
	s.shuffleCalls = append(s.shuffleCalls, enabled)
	return nil
}

// stubErrSessionWriter implements db.SessionWriter and fails WriteShuffle, to
// test persistence-failure surfacing.
type stubErrSessionWriter struct {
	writeShuffleErr error
}

func (s *stubErrSessionWriter) WriteRepeatMode(string) error { return nil }
func (s *stubErrSessionWriter) WriteShuffle(bool) error      { return s.writeShuffleErr }
func (s *stubErrSessionWriter) WriteRadioMode(bool) error    { return nil }
func (s *stubErrSessionWriter) UpdateSessionPosition(string, int64) error {
	return nil
}
func (s *stubErrSessionWriter) WriteVolume(int) error { return nil }

// TestShuffleToggle_Success enables shuffle, propagating to the player and
// persisting the flag, and shows the "Shuffle: on (S)" feedback naming the
// status-bar glyph.
func TestShuffleToggle_Success(t *testing.T) {
	p := &stubRepeatShuffler{}
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))

	next, _, handled := m.handlePlaybackKeyAction(keys.ActionShuffleToggle)
	if !handled {
		t.Fatal("actionShuffleToggle not handled")
	}
	nm := next.(Model)

	if !nm.playback.shuffle {
		t.Error("playback.shuffle = false, want true after toggle")
	}
	if len(p.shuffleCalls) != 1 || !p.shuffleCalls[0] {
		t.Errorf("SetShuffle calls = %v, want [true]", p.shuffleCalls)
	}
	if len(sw.shuffleCalls) != 1 || !sw.shuffleCalls[0] {
		t.Errorf("WriteShuffle calls = %v, want [true]", sw.shuffleCalls)
	}
	if msg := nm.statusBar.topNotification().msg; !strings.Contains(msg, "Shuffle: on (S)") {
		t.Errorf("status = %q, want it to contain 'Shuffle: on (S)'", msg)
	}
}

// TestShuffleToggle_RuntimeFailureReverts verifies that when the player rejects
// SetShuffle, the toggle is reverted, the failure is surfaced, and the flag is
// NOT persisted (runtime and persisted state must not diverge).
func TestShuffleToggle_RuntimeFailureReverts(t *testing.T) {
	p := &stubRepeatShuffler{shuffleErr: errors.New("mpv unreachable")}
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))

	next, _, _ := m.handlePlaybackKeyAction(keys.ActionShuffleToggle)
	nm := next.(Model)

	if nm.playback.shuffle {
		t.Error("playback.shuffle = true, want reverted to false after runtime failure")
	}
	if len(sw.shuffleCalls) != 0 {
		t.Errorf("WriteShuffle calls = %v, want none after runtime failure", sw.shuffleCalls)
	}
	if msg := nm.statusBar.topNotification().msg; !strings.Contains(msg, "Shuffle failed") {
		t.Errorf("status = %q, want it to contain 'Shuffle failed'", msg)
	}
}

// TestShuffleToggle_PersistenceFailureWarnsNotSaved verifies that when the flag
// cannot be persisted, runtime shuffle stays active but the user is told it
// will not survive a restart.
func TestShuffleToggle_PersistenceFailureWarnsNotSaved(t *testing.T) {
	p := &stubRepeatShuffler{}
	sw := &stubErrSessionWriter{writeShuffleErr: errors.New("disk full")}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))

	next, _, _ := m.handlePlaybackKeyAction(keys.ActionShuffleToggle)
	nm := next.(Model)

	// Runtime shuffle remains enabled even though persistence failed.
	if !nm.playback.shuffle {
		t.Error("playback.shuffle = false, want true (runtime shuffle stays active)")
	}
	if len(p.shuffleCalls) != 1 || !p.shuffleCalls[0] {
		t.Errorf("SetShuffle calls = %v, want [true]", p.shuffleCalls)
	}
	if msg := nm.statusBar.topNotification().msg; !strings.Contains(msg, "won't survive restart") {
		t.Errorf("status = %q, want it to warn 'won't survive restart'", msg)
	}
}
