package queue

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
	"strings"
	"testing"
)

func TestRangeUsesTagFilteredQueueCount(t *testing.T) {
	v := View{Width: 60, Height: 8, TagFilterHashes: map[string]struct{}{}}
	for i := range 30 {
		hash := fmt.Sprint(i)
		v.Entries = append(v.Entries, Entry{ID: int64(i + 1), Hash: hash, Title: "Song " + hash})
		if i < 15 {
			v.TagFilterHashes[hash] = struct{}{}
		}
	}
	out := ansi.Strip(v.Render(styles.NewStyles(true), -1, playback.StateStopped, nil, nil, nil))
	if !strings.Contains(out, "of 15 · 30 total") {
		t.Fatal(out)
	}
	v.ScrollOffset = 10 // A resize must reveal the whole filtered queue.
	v.Cursor = 14
	v.Height = 30
	out = ansi.Strip(v.Render(styles.NewStyles(true), -1, playback.StateStopped, nil, nil, nil))
	if strings.Contains(out, "of 15") || !strings.Contains(out, "15/30 tracks") {
		t.Fatal(out)
	}
}
