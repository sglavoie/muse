package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// sampleLibraryTracks returns a set of sorted tracks suitable for view tests.
// The tracks are sorted by artist as LibraryIndex provides.
func sampleLibraryTracks() []library.Track {
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Duration: 183, Hash: "h1", Path: "/a1.flac"},
		{Artist: "Alice", Album: "Alpha", Title: "Track A2", Duration: 245, Hash: "h2", Path: "/a2.flac"},
		{Artist: "Alice", Album: "Beta", Title: "Track B1", Duration: 300, Hash: "h3", Path: "/b1.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Track G1", Duration: 120, Hash: "h4", Path: "/g1.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Track G2", Duration: 60, Hash: "h5", Path: "/g2.flac"},
		{Artist: "Charlie", Album: "Delta", Title: "Track D1", Duration: 3661, Hash: "h6", Path: "/d1.flac"},
	}
}

// newModelWithTracks creates a Model pre-loaded with sample tracks.
func newModelWithTracks() Model {
	return New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
}

// pressKey sends a key press and returns the resulting Model.
func pressKey(m Model, key string) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
	return next.(Model)
}

// pressEnter sends an Enter key press and returns the resulting Model.
func pressEnter(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(Model)
}

// pressEsc sends an Escape key press and returns the resulting Model.
func pressEsc(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	return next.(Model)
}

// pressSlash sends a "/" keypress to activate the filter bar, then executes
// the resulting FilterBarMsg command to complete activation.
func pressSlash(m Model) Model {
	next, cmd := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	return m
}

// typeText sends each rune of s as a keypress.
func typeText(m Model, s string) Model {
	for _, ch := range s {
		next, _ := m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}
	return m
}
