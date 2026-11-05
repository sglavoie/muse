// Package lyrics reads song lyrics: LRC files beside the audio file and
// lyrics embedded in its tags. Synced lyrics carry a start time per line so a
// player can follow along; unsynced lyrics are plain text.
package lyrics

import (
	"sort"
	"time"
)

// Line is one lyric line. At is when it starts, already corrected by the
// file's [offset:] tag; Text is empty for an instrumental gap.
type Line struct {
	At   time.Duration
	Text string
}

// Lyrics are a song's lyrics. Synced lyrics have their lines in time order;
// unsynced lyrics keep the text's own order and leave At zero.
type Lyrics struct {
	Lines  []Line
	Synced bool
	// Source names where the lyrics came from: "sidecar" or "embedded".
	Source string
}

// Empty reports whether there is nothing to show.
func (l *Lyrics) Empty() bool {
	if l == nil {
		return true
	}
	for _, line := range l.Lines {
		if line.Text != "" {
			return false
		}
	}
	return true
}

// Index returns the line playing at pos: the last line starting at or
// before it, or -1 before the first line and for unsynced lyrics. Lines are
// sorted, so it is a binary search.
func (l *Lyrics) Index(pos time.Duration) int {
	if l == nil || !l.Synced {
		return -1
	}
	// The first line starting after pos; the one before it is playing.
	return sort.Search(len(l.Lines), func(i int) bool { return l.Lines[i].At > pos }) - 1
}
