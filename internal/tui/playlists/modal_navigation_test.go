package playlists

import "testing"

// --- cursor navigation ---

// TestModalCursorMovesDownWithJ verifies that pressing 'j' increments the
// cursor from the initial position.
func TestModalCursorMovesDownWithJ(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha", "Beta"))

	m2, cmd := m.HandleKey("j")
	if cmd != nil {
		t.Fatalf("j: expected nil cmd (navigation only), got %T", cmd)
	}
	if m2.cursor != 1 {
		t.Errorf("j: expected cursor=1, got %d", m2.cursor)
	}
}

// TestModalCursorMovesUpWithK verifies that pressing 'k' decrements the
// cursor when it is above index 0.
func TestModalCursorMovesUpWithK(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha"))
	m.cursor = 1

	m2, cmd := m.HandleKey("k")
	if cmd != nil {
		t.Fatalf("k: expected nil cmd (navigation only), got %T", cmd)
	}
	if m2.cursor != 0 {
		t.Errorf("k: expected cursor=0, got %d", m2.cursor)
	}
}

// TestModalDownArrowMovesDown verifies that the "down" key also moves the
// cursor down (alias for 'j').
func TestModalDownArrowMovesDown(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha"))

	m2, _ := m.HandleKey("down")
	if m2.cursor != 1 {
		t.Errorf("down: expected cursor=1, got %d", m2.cursor)
	}
}

// TestModalUpArrowMovesUp verifies that the "up" key also moves the cursor
// up (alias for 'k').
func TestModalUpArrowMovesUp(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha"))
	m.cursor = 1

	m2, _ := m.HandleKey("up")
	if m2.cursor != 0 {
		t.Errorf("up: expected cursor=0, got %d", m2.cursor)
	}
}

// TestModalCursorClampsAtBottom verifies that pressing 'j' when the cursor is
// already on the last entry does not move it past the end.
func TestModalCursorClampsAtBottom(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha", "Beta"))
	// destinations: [Play Queue, Alpha, Beta, New playlist…] — last index = 3
	m.cursor = 3

	m2, _ := m.HandleKey("j")
	if m2.cursor != 3 {
		t.Errorf("j at bottom: expected cursor to clamp at 3, got %d", m2.cursor)
	}
}

// TestModalCursorClampsAtTop verifies that pressing 'k' when the cursor is
// already on the first entry does not move it above 0.
func TestModalCursorClampsAtTop(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha"))
	// cursor starts at 0

	m2, _ := m.HandleKey("k")
	if m2.cursor != 0 {
		t.Errorf("k at top: expected cursor to clamp at 0, got %d", m2.cursor)
	}
}

// TestModalNavigationFullRange navigates through all destinations and verifies
// the cursor visits each index exactly once in sequence.
func TestModalNavigationFullRange(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("A", "B", "C"))
	// destinations: [Play Queue, A, B, C, New playlist…] — 5 entries
	n := len(m.destinations())
	if n != 5 {
		t.Fatalf("expected 5 destinations, got %d", n)
	}

	for i := 1; i < n; i++ {
		m, _ = m.HandleKey("j")
		if m.cursor != i {
			t.Errorf("step %d: expected cursor=%d, got %d", i, i, m.cursor)
		}
	}
	// Now press j once more — must stay at last entry.
	m, _ = m.HandleKey("j")
	if m.cursor != n-1 {
		t.Errorf("past bottom: expected cursor=%d, got %d", n-1, m.cursor)
	}
}

// TestModalHandleKeyIsImmutable verifies that HandleKey does not mutate the
// receiver: the original modal's cursor must be unchanged after a call.
func TestModalHandleKeyIsImmutable(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha"))
	original := m.cursor

	m.HandleKey("j")
	if m.cursor != original {
		t.Errorf("HandleKey must not mutate receiver; cursor changed from %d to %d", original, m.cursor)
	}
}
