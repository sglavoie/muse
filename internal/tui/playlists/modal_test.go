package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// fakePlaylistRows returns a slice of PlaylistRow values as if returned by
// db.ReadPlaylists, pre-sorted alphabetically.
func fakePlaylistRows(names ...string) []db.PlaylistRow {
	rows := make([]db.PlaylistRow, len(names))
	for i, n := range names {
		rows[i] = db.PlaylistRow{ID: int64(i + 1), Name: n}
	}
	return rows
}

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
	// destinations: [Play Queue, Alpha, Beta] — last index = 2
	m.cursor = 2

	m2, _ := m.HandleKey("j")
	if m2.cursor != 2 {
		t.Errorf("j at bottom: expected cursor to clamp at 2, got %d", m2.cursor)
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
	// destinations: [Play Queue, A, B, C] — 4 entries
	n := len(m.destinations())
	if n != 4 {
		t.Fatalf("expected 4 destinations, got %d", n)
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

// --- Enter confirms selection ---

// TestModalEnterOnQueueReturnsQueueMsg verifies that pressing Enter when the
// cursor is on the Play Queue entry dispatches DestinationPickedMsg{IsQueue:true}.
func TestModalEnterOnQueueReturnsQueueMsg(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha"))
	// cursor=0 → Play Queue

	_, cmd := m.HandleKey("enter")
	if cmd == nil {
		t.Fatal("enter on queue: expected non-nil cmd")
	}
	msg := cmd()
	picked, ok := msg.(DestinationPickedMsg)
	if !ok {
		t.Fatalf("enter on queue: expected DestinationPickedMsg, got %T", msg)
	}
	if !picked.IsQueue {
		t.Errorf("enter on queue: expected IsQueue=true, got false")
	}
	if picked.PlaylistName != "" {
		t.Errorf("enter on queue: expected empty PlaylistName, got %q", picked.PlaylistName)
	}
}

// TestModalEnterOnFirstPlaylistReturnsPlaylistMsg verifies that Enter on the
// first playlist entry (cursor=1) returns the correct playlist name.
func TestModalEnterOnFirstPlaylistReturnsPlaylistMsg(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Chill", "Workout"))
	m.cursor = 1 // "Chill"

	_, cmd := m.HandleKey("enter")
	if cmd == nil {
		t.Fatal("enter on playlist: expected non-nil cmd")
	}
	msg := cmd()
	picked, ok := msg.(DestinationPickedMsg)
	if !ok {
		t.Fatalf("enter on playlist: expected DestinationPickedMsg, got %T", msg)
	}
	if picked.IsQueue {
		t.Errorf("enter on playlist: expected IsQueue=false")
	}
	if picked.PlaylistName != "Chill" {
		t.Errorf("enter on playlist: expected PlaylistName='Chill', got %q", picked.PlaylistName)
	}
}

// TestModalEnterOnSecondPlaylistReturnsPlaylistMsg verifies Enter on the second
// playlist entry (cursor=2) returns the correct name.
func TestModalEnterOnSecondPlaylistReturnsPlaylistMsg(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Chill", "Workout"))
	m.cursor = 2 // "Workout"

	_, cmd := m.HandleKey("enter")
	if cmd == nil {
		t.Fatal("enter on second playlist: expected non-nil cmd")
	}
	msg := cmd()
	picked, ok := msg.(DestinationPickedMsg)
	if !ok {
		t.Fatalf("enter on second playlist: expected DestinationPickedMsg, got %T", msg)
	}
	if picked.PlaylistName != "Workout" {
		t.Errorf("enter on second playlist: expected PlaylistName='Workout', got %q", picked.PlaylistName)
	}
}

// TestModalEnterOnEachPlaylistReturnsCorrectName iterates all playlist entries
// and verifies that Enter on each returns the matching name.
func TestModalEnterOnEachPlaylistReturnsCorrectName(t *testing.T) {
	names := []string{"Ambient", "Jazz", "Rock"}
	m := NewDestinationPicker(fakePlaylistRows(names...))

	for i, name := range names {
		m.cursor = i + 1 // index 0 is Play Queue
		_, cmd := m.HandleKey("enter")
		if cmd == nil {
			t.Fatalf("playlist %q (cursor=%d): expected non-nil cmd", name, m.cursor)
		}
		msg := cmd()
		picked, ok := msg.(DestinationPickedMsg)
		if !ok {
			t.Fatalf("playlist %q: expected DestinationPickedMsg, got %T", name, msg)
		}
		if picked.PlaylistName != name {
			t.Errorf("playlist %q: expected PlaylistName=%q, got %q", name, name, picked.PlaylistName)
		}
		if picked.IsQueue {
			t.Errorf("playlist %q: expected IsQueue=false", name)
		}
	}
}

// --- Esc dismisses ---

// TestModalEscDismissesWithoutSelection verifies that pressing Esc dispatches
// DestinationPickerDismissedMsg and does not select any destination.
func TestModalEscDismissesWithoutSelection(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha", "Beta"))
	m.cursor = 1 // positioned on a playlist, should not matter

	_, cmd := m.HandleKey("esc")
	if cmd == nil {
		t.Fatal("esc: expected non-nil cmd (dismiss)")
	}
	msg := cmd()
	if _, ok := msg.(DestinationPickerDismissedMsg); !ok {
		t.Fatalf("esc: expected DestinationPickerDismissedMsg, got %T", msg)
	}
}

// TestModalEscFromQueuePositionDismisses verifies Esc works when the cursor is
// on the first entry (Play Queue).
func TestModalEscFromQueuePositionDismisses(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha"))
	// cursor=0 (Play Queue)

	_, cmd := m.HandleKey("esc")
	if cmd == nil {
		t.Fatal("esc from queue position: expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(DestinationPickerDismissedMsg); !ok {
		t.Fatalf("esc from queue position: expected DestinationPickerDismissedMsg, got %T", msg)
	}
}

// --- empty playlist list shows create prompt ---

// TestModalEmptyPlaylistListShowsCreateLabel verifies that when no playlists
// are provided, the destinations slice contains the "New playlist..." entry as
// the second item (after the Play Queue).
func TestModalEmptyPlaylistListShowsCreateLabel(t *testing.T) {
	m := NewDestinationPicker(nil)
	dests := m.destinations()

	if len(dests) != 2 {
		t.Fatalf("empty playlists: expected 2 destinations (queue + create), got %d: %v", len(dests), dests)
	}
	if dests[0] != queueLabel {
		t.Errorf("empty playlists: expected dests[0]=%q, got %q", queueLabel, dests[0])
	}
	if dests[1] != createLabel {
		t.Errorf("empty playlists: expected dests[1]=%q, got %q", createLabel, dests[1])
	}
}

// TestModalEnterOnCreateLabelDispatchesCreateMsg verifies that Enter when the
// cursor points to the "New playlist..." entry dispatches DestinationPickerCreateMsg.
func TestModalEnterOnCreateLabelDispatchesCreateMsg(t *testing.T) {
	m := NewDestinationPicker(nil)
	m.cursor = 1 // "New playlist..."

	_, cmd := m.HandleKey("enter")
	if cmd == nil {
		t.Fatal("enter on create label: expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(DestinationPickerCreateMsg); !ok {
		t.Fatalf("enter on create label: expected DestinationPickerCreateMsg, got %T", msg)
	}
}

// TestModalEnterOnQueueWithNoPlaylistsReturnsQueueMsg verifies that even when
// no playlists exist, Enter on the Play Queue entry still dispatches
// DestinationPickedMsg{IsQueue:true}.
func TestModalEnterOnQueueWithNoPlaylistsReturnsQueueMsg(t *testing.T) {
	m := NewDestinationPicker(nil)
	// cursor=0 → Play Queue

	_, cmd := m.HandleKey("enter")
	if cmd == nil {
		t.Fatal("enter on queue (no playlists): expected non-nil cmd")
	}
	msg := cmd()
	picked, ok := msg.(DestinationPickedMsg)
	if !ok {
		t.Fatalf("enter on queue (no playlists): expected DestinationPickedMsg, got %T", msg)
	}
	if !picked.IsQueue {
		t.Errorf("enter on queue (no playlists): expected IsQueue=true")
	}
}

// --- destinations list order ---

// TestModalDestinationsQueueFirst verifies that Play Queue is always the first
// destination regardless of how many playlists are present.
func TestModalDestinationsQueueFirst(t *testing.T) {
	for _, rows := range [][]db.PlaylistRow{
		nil,
		fakePlaylistRows("Only"),
		fakePlaylistRows("A", "B", "C"),
	} {
		m := NewDestinationPicker(rows)
		dests := m.destinations()
		if len(dests) == 0 || dests[0] != queueLabel {
			t.Errorf("destinations must start with %q; got %v", queueLabel, dests)
		}
	}
}

// TestModalDestinationsIncludeAllPlaylists verifies that every playlist name
// passed to NewDestinationPicker appears in destinations(), after the queue.
func TestModalDestinationsIncludeAllPlaylists(t *testing.T) {
	names := []string{"Ambient", "Chill", "Focus", "Sleep"}
	m := NewDestinationPicker(fakePlaylistRows(names...))
	dests := m.destinations()

	// First entry must be Play Queue.
	if dests[0] != queueLabel {
		t.Fatalf("expected first destination %q, got %q", queueLabel, dests[0])
	}
	// Remaining entries must match names in order.
	if len(dests) != len(names)+1 {
		t.Fatalf("expected %d destinations, got %d: %v", len(names)+1, len(dests), dests)
	}
	for i, name := range names {
		if dests[i+1] != name {
			t.Errorf("destination[%d]: expected %q, got %q", i+1, name, dests[i+1])
		}
	}
}

// TestModalCreateLabelNotShownWhenPlaylistsExist verifies that the "New
// playlist..." entry is absent when at least one playlist is present.
func TestModalCreateLabelNotShownWhenPlaylistsExist(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("OnlyOne"))
	dests := m.destinations()
	for _, d := range dests {
		if d == createLabel {
			t.Errorf("create label must not appear when playlists exist; destinations: %v", dests)
		}
	}
}

// --- track set passed to modal (integration smoke test) ---

// TestModalReceivesPreSortedPlaylistsFromFakeReader simulates the pattern used
// by the root model: read playlists from a fake source, construct the modal,
// and verify that the modal's destinations match the input in the correct order.
// This mirrors how LibraryView's 'a' handler calls NewDestinationPicker with
// db.ReadPlaylists output.
func TestModalReceivesPreSortedPlaylistsFromFakeReader(t *testing.T) {
	// Simulate what db.ReadPlaylists would return: alphabetically sorted rows.
	fakeRead := []db.PlaylistRow{
		{ID: 3, Name: "Ambient", TrackCount: 10},
		{ID: 1, Name: "Chill", TrackCount: 5},
		{ID: 2, Name: "Workout", TrackCount: 8},
	}

	m := NewDestinationPicker(fakeRead)
	dests := m.destinations()

	// Expect: [Play Queue, Ambient, Chill, Workout]
	expected := []string{queueLabel, "Ambient", "Chill", "Workout"}
	if len(dests) != len(expected) {
		t.Fatalf("expected %d destinations, got %d: %v", len(expected), len(dests), dests)
	}
	for i, want := range expected {
		if dests[i] != want {
			t.Errorf("destination[%d]: expected %q, got %q", i, want, dests[i])
		}
	}

	// Confirm that Enter on "Chill" (cursor=2) returns the correct name.
	m.cursor = 2
	_, cmd := m.HandleKey("enter")
	if cmd == nil {
		t.Fatal("expected non-nil cmd for Enter on Chill")
	}
	msg := cmd()
	picked, ok := msg.(DestinationPickedMsg)
	if !ok {
		t.Fatalf("expected DestinationPickedMsg, got %T", msg)
	}
	if picked.PlaylistName != "Chill" {
		t.Errorf("expected PlaylistName='Chill', got %q", picked.PlaylistName)
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
