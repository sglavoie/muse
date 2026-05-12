package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestEqualKeyFiresVolumeUp verifies that pressing '=' calls SetVolumeRelative
// with a positive delta on the player, updates the volume field, and sets a
// transient notification.
func TestEqualKeyFiresVolumeUp(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.volume = 50

	next, _ := m.Update(tea.KeyPressMsg{Code: '=', Text: "="})
	nm := next.(Model)

	if p.volumeDelta != 5 {
		t.Fatalf("= key: expected SetVolumeRelative(5), got delta=%v", p.volumeDelta)
	}
	if nm.playback.volume != 55 {
		t.Fatalf("= key: expected volume=55, got %d", nm.playback.volume)
	}
	if nm.statusBar.topNotification().msg != "Volume: 55%" {
		t.Fatalf("= key: expected notification 'Volume: 55%%', got %q", nm.statusBar.topNotification().msg)
	}
}

// TestVolumeUpDownCallsWriteVolume verifies that pressing '=' (volume up) and
// '-' (volume down) each call sessionWriter.WriteVolume with the updated
// volume percentage.
func TestVolumeUpDownCallsWriteVolume(t *testing.T) {
	p := &stubVolumePlayer{}
	sw := &stubSessionWriter{}
	m := New(context.Background(), WithPlayer(p), WithSessionWriter(sw))
	m.playback.volume = 50

	// Press '=' — volume up by volumeIncrement (5).
	next, _ := m.Update(tea.KeyPressMsg{Code: '=', Text: "="})
	nm := next.(Model)

	if len(sw.volumeCalls) != 1 {
		t.Fatalf("volume up: expected 1 WriteVolume call, got %d", len(sw.volumeCalls))
	}
	if sw.volumeCalls[0] != nm.playback.volume {
		t.Errorf("volume up: WriteVolume(%d), model volume %d", sw.volumeCalls[0], nm.playback.volume)
	}

	// Press '-' — volume down by volumeIncrement (5).
	next, _ = nm.Update(tea.KeyPressMsg{Code: '-', Text: "-"})
	nm = next.(Model)

	if len(sw.volumeCalls) != 2 {
		t.Fatalf("volume down: expected 2 WriteVolume calls, got %d", len(sw.volumeCalls))
	}
	if sw.volumeCalls[1] != nm.playback.volume {
		t.Errorf("volume down: WriteVolume(%d), model volume %d", sw.volumeCalls[1], nm.playback.volume)
	}
}

// TestMinusKeyFiresVolumeDown verifies that pressing '-' calls
// SetVolumeRelative(-5) on the player, updates the volume field, and sets a
// transient notification.
func TestMinusKeyFiresVolumeDown(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.volume = 50

	next, _ := m.Update(tea.KeyPressMsg{Code: '-', Text: "-"})
	nm := next.(Model)

	if p.volumeDelta != -5 {
		t.Fatalf("- key: expected SetVolumeRelative(-5), got delta=%v", p.volumeDelta)
	}
	if nm.playback.volume != 45 {
		t.Fatalf("- key: expected volume=45, got %d", nm.playback.volume)
	}
	if nm.statusBar.topNotification().msg != "Volume: 45%" {
		t.Fatalf("- key: expected notification 'Volume: 45%%', got %q", nm.statusBar.topNotification().msg)
	}
}

// TestVolumeUpClampsAt100 verifies that volume up from 95 clamps to 100.
func TestVolumeUpClampsAt100(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.volume = 95

	next, _ := m.Update(tea.KeyPressMsg{Code: '=', Text: "="})
	nm := next.(Model)

	if nm.playback.volume != 100 {
		t.Fatalf("volume up from 95: expected 100, got %d", nm.playback.volume)
	}
	if nm.statusBar.topNotification().msg != "Volume: 100%" {
		t.Fatalf("expected notification 'Volume: 100%%', got %q", nm.statusBar.topNotification().msg)
	}
}

// TestVolumeUpStaysAt100 verifies that volume up from 100 stays at 100.
func TestVolumeUpStaysAt100(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.volume = 100

	next, _ := m.Update(tea.KeyPressMsg{Code: '=', Text: "="})
	nm := next.(Model)

	if nm.playback.volume != 100 {
		t.Fatalf("volume up from 100: expected 100, got %d", nm.playback.volume)
	}
}

// TestVolumeDownClampsAt0 verifies that volume down from 5 clamps to 0.
func TestVolumeDownClampsAt0(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.volume = 5

	next, _ := m.Update(tea.KeyPressMsg{Code: '-', Text: "-"})
	nm := next.(Model)

	if nm.playback.volume != 0 {
		t.Fatalf("volume down from 5: expected 0, got %d", nm.playback.volume)
	}
	if nm.statusBar.topNotification().msg != "Volume: 0%" {
		t.Fatalf("expected notification 'Volume: 0%%', got %q", nm.statusBar.topNotification().msg)
	}
}

// TestVolumeNotificationUsesShorterExpiry verifies that volume notifications
// use volumeFeedbackDuration (2s), not the default transientDuration (5s).
func TestVolumeNotificationUsesShorterExpiry(t *testing.T) {
	p := &stubVolumePlayer{}
	m := New(context.Background(), WithPlayer(p))
	m.playback.volume = 50

	next, _ := m.Update(tea.KeyPressMsg{Code: '=', Text: "="})
	nm := next.(Model)

	expiry := nm.statusBar.topNotification().expiresAt
	// The expiry should be roughly 2s from now, not 5s. Allow 1s tolerance.
	if d := time.Until(expiry); d > 3*time.Second || d < 1*time.Second {
		t.Fatalf("volume notification expiry: expected ~2s from now, got %v", d)
	}
}
