package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// TestPreviewDismissWithEsc verifies that pressing Esc while preview is
// active dismisses it.
func TestPreviewDismissWithEsc(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{}
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = sp

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)

	if nm.overlay.preview != nil {
		t.Fatal("expected preview to be nil after Esc")
	}
	if !sp.closeCalled {
		t.Fatal("expected Close to be called on preview player")
	}
}

// TestPreviewDismissWithP verifies that pressing P while preview is
// active dismisses it.
func TestPreviewDismissWithP(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{}
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = sp

	next, _ := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	nm := next.(Model)

	if nm.overlay.preview != nil {
		t.Fatal("expected preview to be nil after P")
	}
	if !sp.closeCalled {
		t.Fatal("expected Close to be called on preview player")
	}
}

// TestPreviewSpaceControlsPreview verifies that space toggles the preview
// player instead of affecting the main player.
func TestPreviewSpaceControlsPreview(t *testing.T) {
	sp := &stubPreviewer{}
	mp := &stubPreviewMainPlayer{}
	m := New(context.Background())
	m.player = mp
	m.playback.state = playback.StatePlaying
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = sp

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	nm := next.(Model)

	if sp.toggleCalls != 1 {
		t.Fatalf("expected 1 preview toggle call, got %d", sp.toggleCalls)
	}
	if mp.pauseCalls != 0 {
		t.Fatalf("expected main player pause to remain unused, got %d calls", mp.pauseCalls)
	}
	if nm.overlay.preview == nil {
		t.Fatal("expected preview to remain active after space")
	}
}

// TestPreviewSeekKeysControlPreview verifies that seek keys control the
// preview player and do not fall through to the main player.
func TestPreviewSeekKeysControlPreview(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		want float64
	}{
		{name: "left", msg: tea.KeyPressMsg{Code: tea.KeyLeft}, want: -5},
		{name: "right", msg: tea.KeyPressMsg{Code: tea.KeyRight}, want: 5},
		{name: "shift+left", msg: tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift}, want: -30},
		{name: "shift+right", msg: tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift}, want: 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sp := &stubPreviewer{}
			mp := &stubPreviewMainPlayer{}
			m := New(context.Background())
			m.player = mp
			m.cfg.SeekOffsetShort = 5
			m.cfg.SeekOffsetLong = 30
			m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
			m.overlay.previewPlayer = sp

			next, _ := m.Update(tt.msg)
			nm := next.(Model)

			if len(sp.seekCalls) != 1 {
				t.Fatalf("expected 1 preview seek call, got %d", len(sp.seekCalls))
			}
			if sp.seekCalls[0] != tt.want {
				t.Fatalf("preview seek = %v, want %v", sp.seekCalls[0], tt.want)
			}
			if len(mp.seekCalls) != 0 {
				t.Fatalf("expected main player seek to remain unused, got %v", mp.seekCalls)
			}
			if nm.overlay.preview == nil {
				t.Fatal("expected preview to remain active after seek")
			}
		})
	}
}

// TestPreviewSwallowsKeys verifies that while preview is active, unrelated
// keys are swallowed (not processed).
func TestPreviewSwallowsKeys(t *testing.T) {
	m := New(context.Background())
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = &stubPreviewer{}
	m.activeView = viewLibrary

	// Press '3' (switch to queue) — should be swallowed.
	next, _ := m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	nm := next.(Model)

	if nm.activeView != viewLibrary {
		t.Fatalf("expected view to stay at library while preview active, got %d", nm.activeView)
	}
	if nm.overlay.preview == nil {
		t.Fatal("preview should still be active after swallowed key")
	}
}

// TestPreviewCtrlCDismissesAndQuits verifies that ctrl+c during preview
// dismisses the preview and returns a quit command.
func TestPreviewCtrlCDismissesAndQuits(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{}
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = sp

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	nm := next.(Model)

	if nm.overlay.preview != nil {
		t.Fatal("expected preview to be dismissed on ctrl+c")
	}
	if !sp.closeCalled {
		t.Fatal("expected Close to be called on preview player")
	}
	if cmd == nil {
		t.Fatal("expected quit command from ctrl+c during preview")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected tea.QuitMsg from ctrl+c command")
	}
}

// TestPreviewDismissResumesPlayback verifies that dismissing preview
// resumes main playback when it was playing before preview.
func TestPreviewDismissResumesPlayback(t *testing.T) {
	sp := &stubPlayer{}
	m := New(context.Background())
	m.player = sp
	m.playback.state = playback.StatePaused
	m.overlay.previewWasPlaying = true
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = &stubPreviewer{}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)

	if nm.overlay.preview != nil {
		t.Fatal("expected preview to be dismissed")
	}
	if sp.playCalls != 1 {
		t.Fatalf("expected 1 Play call to resume playback, got %d", sp.playCalls)
	}
	if nm.overlay.previewWasPlaying {
		t.Fatal("expected previewWasPlaying to be reset to false")
	}
}

// TestPreviewDismissDoesNotResumeIfNotPlaying verifies that dismissing
// preview does not resume main playback when it was not playing.
func TestPreviewDismissDoesNotResumeIfNotPlaying(t *testing.T) {
	sp := &stubPlayer{}
	m := New(context.Background())
	m.player = sp
	m.playback.state = playback.StatePaused
	m.overlay.previewWasPlaying = false
	m.overlay.preview = &PreviewOverlay{artist: "A", title: "T"}
	m.overlay.previewPlayer = &stubPreviewer{}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)

	if sp.playCalls != 0 {
		t.Fatalf("expected 0 Play calls when not previously playing, got %d", sp.playCalls)
	}
	_ = nm
}

// TestPreviewOverlayRenderedInView verifies that when preview is active,
// View() renders the preview overlay content.
func TestPreviewOverlayRenderedInView(t *testing.T) {
	m := New(context.Background())
	m.overlay.preview = &PreviewOverlay{
		artist:  "Preview Artist",
		title:   "Preview Song",
		elapsed: 30,
		total:   120,
	}
	m.termWidth = 80
	m.termHeight = 24

	v := m.View()
	if v.Content == "" {
		t.Fatal("expected non-empty View content with preview active")
	}
}

// TestPreviewDismissResetsDurationFilledFlag verifies that dismissing a preview
// resets the previewDurationFilled flag for the next preview session.
func TestPreviewDismissResetsDurationFilledFlag(t *testing.T) {
	m := New(context.Background())
	m.overlay.preview = &PreviewOverlay{hash: "h1", artist: "A", title: "T"}
	m.overlay.previewPlayer = &stubPreviewer{}
	m.overlay.previewDurationFilled = true

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	nm := next.(Model)

	if nm.overlay.previewDurationFilled {
		t.Fatal("expected previewDurationFilled to be reset after dismiss")
	}
}
