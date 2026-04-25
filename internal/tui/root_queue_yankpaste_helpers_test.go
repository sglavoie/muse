package tui

// sampleQueueEntries returns a slice of queue entries for yank/paste tests.
func sampleQueueEntries() []QueueEntry {
	return []QueueEntry{
		{ID: 101, Path: "/a.mp3", Hash: "h1", Display: "Artist - A", Artist: "Artist", Title: "A", Album: "Album", DurationMs: 180000},
		{ID: 102, Path: "/b.mp3", Hash: "h2", Display: "Artist - B", Artist: "Artist", Title: "B", Album: "Album", DurationMs: 200000},
		{ID: 103, Path: "/c.mp3", Hash: "h3", Display: "Artist - C", Artist: "Artist", Title: "C", Album: "Album", DurationMs: 220000},
		{ID: 104, Path: "/d.mp3", Hash: "h4", Display: "Artist - D", Artist: "Artist", Title: "D", Album: "Album", DurationMs: 240000},
	}
}
