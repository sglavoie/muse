package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

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
// passed to NewDestinationPicker appears in destinations(), after the queue
// and before the "New playlist..." entry.
func TestModalDestinationsIncludeAllPlaylists(t *testing.T) {
	names := []string{"Ambient", "Chill", "Focus", "Sleep"}
	m := NewDestinationPicker(fakePlaylistRows(names...))
	dests := m.destinations()

	// First entry must be Play Queue.
	if dests[0] != queueLabel {
		t.Fatalf("expected first destination %q, got %q", queueLabel, dests[0])
	}
	// Queue + playlists + "New playlist..." = len(names) + 2
	if len(dests) != len(names)+2 {
		t.Fatalf("expected %d destinations, got %d: %v", len(names)+2, len(dests), dests)
	}
	for i, name := range names {
		if dests[i+1] != name {
			t.Errorf("destination[%d]: expected %q, got %q", i+1, name, dests[i+1])
		}
	}
	// Last entry must be "New playlist...".
	if dests[len(dests)-1] != createLabel {
		t.Errorf("expected last destination %q, got %q", createLabel, dests[len(dests)-1])
	}
}

// TestModalCreateLabelAlwaysShown verifies that the "New playlist..." entry
// is always present as the last destination, even when playlists exist.
func TestModalCreateLabelAlwaysShown(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("OnlyOne"))
	dests := m.destinations()
	if dests[len(dests)-1] != createLabel {
		t.Errorf("expected last destination %q, got %v", createLabel, dests)
	}
}

// TestModalEnterOnCreateLabelWithPlaylistsDispatchesCreateMsg verifies that
// Enter on "New playlist..." dispatches DestinationPickerCreateMsg even when
// playlists already exist.
func TestModalEnterOnCreateLabelWithPlaylistsDispatchesCreateMsg(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha", "Beta"))
	// "New playlist..." is at index 3: [Play Queue, Alpha, Beta, New playlist...]
	m.cursor = 3

	_, cmd := m.HandleKey("enter")
	if cmd == nil {
		t.Fatal("enter on create label: expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(DestinationPickerCreateMsg); !ok {
		t.Fatalf("enter on create label: expected DestinationPickerCreateMsg, got %T", msg)
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

	// Expect: [Play Queue, Ambient, Chill, Workout, New playlist...]
	expected := []string{queueLabel, "Ambient", "Chill", "Workout", createLabel}
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

// TestModalEnterWithCursorAboveRangeClampsToQueue verifies enter remains safe
// when cursor is below the valid range.
func TestModalEnterWithCursorAboveRangeClampsToQueue(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha", "Beta"))
	m.cursor = -10

	_, cmd := m.HandleKey("enter")
	if cmd == nil {
		t.Fatal("enter with negative cursor: expected non-nil cmd")
	}
	msg := cmd()
	picked, ok := msg.(DestinationPickedMsg)
	if !ok {
		t.Fatalf("enter with negative cursor: expected DestinationPickedMsg, got %T", msg)
	}
	if !picked.IsQueue {
		t.Errorf("enter with negative cursor: expected IsQueue=true")
	}
}

// TestModalEnterWithCursorPastEndClampsToCreate verifies enter remains safe
// when cursor is above the valid range.
func TestModalEnterWithCursorPastEndClampsToCreate(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha", "Beta"))
	m.cursor = 99

	_, cmd := m.HandleKey("enter")
	if cmd == nil {
		t.Fatal("enter with out-of-range cursor: expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(DestinationPickerCreateMsg); !ok {
		t.Fatalf("enter with out-of-range cursor: expected DestinationPickerCreateMsg, got %T", msg)
	}
}
