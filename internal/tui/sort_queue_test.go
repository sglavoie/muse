package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

func TestSortQueueIndices(t *testing.T) {
	entries := []QueueEntry{
		{Title: "Zulu", Artist: "Charlie", Album: "Red", DurationMs: 3000, SourcePlaylist: "Rock"},
		{Title: "Alpha", Artist: "Bravo", Album: "Blue", DurationMs: 1000, SourcePlaylist: "Jazz"},
		{Title: "Mike", Artist: "Alpha", Album: "", DurationMs: 2000, SourcePlaylist: ""},
	}

	t.Run("sort by title asc", func(t *testing.T) {
		order := SortQueueIndices(entries, sorting.SortTitle, sorting.SortAsc)
		if order == nil {
			t.Fatal("expected non-nil order")
		}
		if entries[order[0]].Title != "Alpha" {
			t.Errorf("first should be Alpha, got %q", entries[order[0]].Title)
		}
		if entries[order[1]].Title != "Mike" {
			t.Errorf("second should be Mike, got %q", entries[order[1]].Title)
		}
		if entries[order[2]].Title != "Zulu" {
			t.Errorf("third should be Zulu, got %q", entries[order[2]].Title)
		}
	})

	t.Run("sort by title desc", func(t *testing.T) {
		order := SortQueueIndices(entries, sorting.SortTitle, sorting.SortDesc)
		if entries[order[0]].Title != "Zulu" {
			t.Errorf("first should be Zulu, got %q", entries[order[0]].Title)
		}
		if entries[order[2]].Title != "Alpha" {
			t.Errorf("last should be Alpha, got %q", entries[order[2]].Title)
		}
	})

	t.Run("sort by artist asc", func(t *testing.T) {
		order := SortQueueIndices(entries, sorting.SortArtist, sorting.SortAsc)
		if entries[order[0]].Artist != "Alpha" {
			t.Errorf("first should be Alpha, got %q", entries[order[0]].Artist)
		}
	})

	t.Run("sort by duration asc", func(t *testing.T) {
		order := SortQueueIndices(entries, sorting.SortDuration, sorting.SortAsc)
		if entries[order[0]].DurationMs != 1000 {
			t.Errorf("first should have 1000ms, got %d", entries[order[0]].DurationMs)
		}
		if entries[order[2]].DurationMs != 3000 {
			t.Errorf("last should have 3000ms, got %d", entries[order[2]].DurationMs)
		}
	})

	t.Run("sort by album asc unknown last", func(t *testing.T) {
		order := SortQueueIndices(entries, sorting.SortAlbum, sorting.SortAsc)
		if entries[order[0]].Album != "Blue" {
			t.Errorf("first should be Blue, got %q", entries[order[0]].Album)
		}
		if entries[order[1]].Album != "Red" {
			t.Errorf("second should be Red, got %q", entries[order[1]].Album)
		}
		if entries[order[2]].Album != "" {
			t.Errorf("third should be empty, got %q", entries[order[2]].Album)
		}
	})

	t.Run("sort by source asc unknown last", func(t *testing.T) {
		order := SortQueueIndices(entries, sorting.SortSource, sorting.SortAsc)
		if entries[order[0]].SourcePlaylist != "Jazz" {
			t.Errorf("first should be Jazz, got %q", entries[order[0]].SourcePlaylist)
		}
		if entries[order[1]].SourcePlaylist != "Rock" {
			t.Errorf("second should be Rock, got %q", entries[order[1]].SourcePlaylist)
		}
		if entries[order[2]].SourcePlaylist != "" {
			t.Errorf("third should be empty, got %q", entries[order[2]].SourcePlaylist)
		}
	})

	t.Run("default returns nil", func(t *testing.T) {
		order := SortQueueIndices(entries, sorting.SortDefault, sorting.SortAsc)
		if order != nil {
			t.Error("SortDefault should return nil")
		}
	})

	t.Run("empty entries returns nil", func(t *testing.T) {
		order := SortQueueIndices(nil, sorting.SortTitle, sorting.SortAsc)
		if order != nil {
			t.Error("empty entries should return nil")
		}
	})

	t.Run("original entries not modified", func(t *testing.T) {
		_ = SortQueueIndices(entries, sorting.SortTitle, sorting.SortAsc)
		if entries[0].Title != "Zulu" {
			t.Error("original entries were modified")
		}
	})
}

func TestQueueEntryIndex(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{Title: "A"},
			{Title: "B"},
			{Title: "C"},
		},
	}

	t.Run("no display order returns identity", func(t *testing.T) {
		if v.entryIndex(0) != 0 {
			t.Error("expected 0")
		}
		if v.entryIndex(2) != 2 {
			t.Error("expected 2")
		}
	})

	t.Run("with display order translates", func(t *testing.T) {
		v.displayOrder = []int{2, 0, 1}
		if v.entryIndex(0) != 2 {
			t.Errorf("display 0 should map to entry 2, got %d", v.entryIndex(0))
		}
		if v.entryIndex(1) != 0 {
			t.Errorf("display 1 should map to entry 0, got %d", v.entryIndex(1))
		}
		if v.entryIndex(2) != 1 {
			t.Errorf("display 2 should map to entry 1, got %d", v.entryIndex(2))
		}
	})
}

func TestQueueMoveDisabledWhenSorted(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{Title: "A"},
			{Title: "B"},
		},
		cursor:       0,
		displayOrder: []int{1, 0},
	}

	v2, cmd := v.HandleAction(keys.ActionMoveTrackDown)
	if cmd == nil {
		t.Error("move down when sorted should return a TransientNotifyMsg cmd")
	} else if _, ok := cmd().(TransientNotifyMsg); !ok {
		t.Error("move down when sorted: expected TransientNotifyMsg")
	}
	_ = v2

	v.cursor = 1
	v3, cmd := v.HandleAction(keys.ActionMoveTrackUp)
	if cmd == nil {
		t.Error("move up when sorted should return a TransientNotifyMsg cmd")
	} else if _, ok := cmd().(TransientNotifyMsg); !ok {
		t.Error("move up when sorted: expected TransientNotifyMsg")
	}
	_ = v3
}

func TestQueueSortDequeueCorrectEntry(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{ID: 10, Title: "Zulu", Artist: "A"},
			{ID: 20, Title: "Alpha", Artist: "B"},
			{ID: 30, Title: "Mike", Artist: "C"},
		},
		sortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
	}
	v = v.recomputeDisplayOrder()

	if v.displayOrder == nil {
		t.Fatal("displayOrder should not be nil")
	}

	v.cursor = 0
	_, cmd := v.HandleAction(keys.ActionDequeue)
	if cmd == nil {
		t.Fatal("expected a cmd for dequeue")
	}
	msg := cmd()
	deqMsg, ok := msg.(DequeueTrackMsg)
	if !ok {
		t.Fatalf("expected DequeueTrackMsg, got %T", msg)
	}
	if v.entries[deqMsg.Index].Title != "Alpha" {
		t.Errorf("expected to dequeue Alpha (entry 1), got %q at index %d",
			v.entries[deqMsg.Index].Title, deqMsg.Index)
	}

	v.cursor = 2
	_, cmd = v.HandleAction(keys.ActionDequeue)
	msg = cmd()
	deqMsg = msg.(DequeueTrackMsg)
	if v.entries[deqMsg.Index].Title != "Zulu" {
		t.Errorf("expected to dequeue Zulu (entry 0), got %q at index %d",
			v.entries[deqMsg.Index].Title, deqMsg.Index)
	}
}

func TestQueueSortDisablesMoveUpDown(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{Title: "A"},
			{Title: "B"},
			{Title: "C"},
		},
		displayOrder: []int{2, 1, 0},
		cursor:       0,
	}

	_, cmd := v.HandleAction(keys.ActionMoveTrackDown)
	if cmd == nil {
		t.Error("move down when sorted should return a TransientNotifyMsg cmd")
	} else if _, ok := cmd().(TransientNotifyMsg); !ok {
		t.Error("move down when sorted: expected TransientNotifyMsg")
	}

	v.cursor = 2
	_, cmd = v.HandleAction(keys.ActionMoveTrackUp)
	if cmd == nil {
		t.Error("move up when sorted should return a TransientNotifyMsg cmd")
	} else if _, ok := cmd().(TransientNotifyMsg); !ok {
		t.Error("move up when sorted: expected TransientNotifyMsg")
	}
}

func TestQueueSortSelectCorrectEntry(t *testing.T) {
	stubFileExistsTrue(t)
	v := QueueView{
		entries: []QueueEntry{
			{ID: 1, Hash: "h1", Title: "Zulu"},
			{ID: 2, Hash: "h2", Title: "Alpha"},
		},
		sortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
	}
	v = v.recomputeDisplayOrder()

	v.cursor = 0
	_, cmd := v.HandleAction(keys.ActionSelect)
	if cmd == nil {
		t.Fatal("expected cmd for select")
	}
	msg := cmd()
	playMsg, ok := msg.(playback.PlayQueueIndexMsg)
	if !ok {
		t.Fatalf("expected PlayQueueIndexMsg, got %T", msg)
	}
	if playMsg.Index != 1 {
		t.Errorf("expected play index 1 (Alpha), got %d", playMsg.Index)
	}
}

func TestQueueBulkSelectionToggleWithSort(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{ID: 1, Title: "Zulu"},
			{ID: 2, Title: "Alpha"},
		},
		sortState: sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc},
	}
	v = v.recomputeDisplayOrder()

	v.cursor = 0
	_, cmd := v.HandleAction(keys.ActionBulkSelectionToggle)
	if cmd == nil {
		t.Fatal("expected cmd for bulk selection toggle")
	}
	msg := cmd()
	bulkMsg, ok := msg.(BulkSelectionToggleMsg)
	if !ok {
		t.Fatalf("expected BulkSelectionToggleMsg, got %T", msg)
	}
	if bulkMsg.Index != 1 {
		t.Errorf("expected toggle index 1 (Alpha), got %d", bulkMsg.Index)
	}
}

func TestQueueFindEntryDisplayPosition(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{{Title: "A"}, {Title: "B"}, {Title: "C"}},
	}

	if v.findEntryDisplayPosition(2) != 2 {
		t.Error("expected identity mapping without display order")
	}

	v.displayOrder = []int{2, 0, 1}
	if got := v.findEntryDisplayPosition(0); got != 1 {
		t.Errorf("entry 0 should be at display 1, got %d", got)
	}
	if got := v.findEntryDisplayPosition(2); got != 0 {
		t.Errorf("entry 2 should be at display 0, got %d", got)
	}
	if got := v.findEntryDisplayPosition(99); got != 0 {
		t.Errorf("missing entry should return 0, got %d", got)
	}
}

func TestQueueEntryIndexBoundsCheck(t *testing.T) {
	v := QueueView{
		entries:      []QueueEntry{{Title: "A"}, {Title: "B"}},
		displayOrder: []int{1, 0},
	}

	if got := v.entryIndex(-1); got != -1 {
		t.Errorf("negative index: got %d, want -1", got)
	}
	if got := v.entryIndex(5); got != 5 {
		t.Errorf("over-bounds index: got %d, want 5", got)
	}
}

func TestSortQueueIndicesZeroDurationSortsLast(t *testing.T) {
	entries := []QueueEntry{
		{Title: "A", DurationMs: 0},
		{Title: "B", DurationMs: 5000},
		{Title: "C", DurationMs: 2000},
	}
	t.Run("asc", func(t *testing.T) {
		order := SortQueueIndices(entries, sorting.SortDuration, sorting.SortAsc)
		if entries[order[0]].Title != "C" {
			t.Errorf("first should be C (2000ms), got %s", entries[order[0]].Title)
		}
		if entries[order[1]].Title != "B" {
			t.Errorf("second should be B (5000ms), got %s", entries[order[1]].Title)
		}
		if entries[order[2]].DurationMs != 0 {
			t.Error("zero-duration entry should be last")
		}
	})
	t.Run("desc", func(t *testing.T) {
		order := SortQueueIndices(entries, sorting.SortDuration, sorting.SortDesc)
		if entries[order[0]].Title != "B" {
			t.Errorf("first should be B (5000ms), got %s", entries[order[0]].Title)
		}
		if entries[order[1]].Title != "C" {
			t.Errorf("second should be C (2000ms), got %s", entries[order[1]].Title)
		}
		if entries[order[2]].DurationMs != 0 {
			t.Error("zero-duration entry should be last")
		}
	})
}
