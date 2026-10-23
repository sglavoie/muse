package autodj

import (
	"errors"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestCompactTuneKeepsFeedbackAndDirtyFields(t *testing.T) {
	p, _ := press(t, newTestPanel(true), "right")
	s := styles.NewStyles(true)
	for _, size := range [][2]int{{60, 12}, {80, 21}, {100, 27}} {
		out := plain(p.Render(size[0], size[1], s, nil, Preview{}))
		for _, want := range []string{"Auto-DJ *", "*Preset", "*Discovery", "Unapplied changes"} {
			if !strings.Contains(out, want) {
				t.Fatalf("missing %q:\n%s", want, out)
			}
		}
		assertFitsBox(t, out, size[0], size[1])
	}
	p.err = errors.New("Commit failed: retry")
	out := plain(p.Render(60, 12, s, []StatusLine{{Text: "background info"}}, Preview{}))
	if !strings.Contains(out, "Commit failed: retry") || strings.Contains(out, "background info") {
		t.Fatal(out)
	}
	// A more severe root failure also precedes the editor's draft warning.
	p.err = nil
	out = plain(p.Render(60, 12, s, []StatusLine{{Text: "Playback failed", Tone: ToneError}}, Preview{}))
	if !strings.Contains(out, "Playback failed") {
		t.Fatal(out)
	}
	p = newTestPanel(true)
	p.draft.Include.TagIDs = []int64{999}
	out = plain(p.Render(60, 12, s, nil, Preview{}))
	if !strings.Contains(out, "unresolved tag") || !strings.Contains(out, "press r") {
		t.Fatal(out)
	}
	p = p.Submit(1).Ack(Applied{Token: 1, Settings: p.Draft(), Active: true})
	if p.Dirty() || p.rowDirty(rowIncludeTags) {
		t.Fatal("acknowledged fields still marked changed")
	}
}

func TestTuneExplainsControlsAndTracksScrolling(t *testing.T) {
	p := newTestPanel(true)
	if p.rowValue(rowExcludeTags) != "none" || !strings.Contains(p.rowValue(rowAnchor), "Stay near starting track") {
		t.Fatal(p.rowValue(rowAnchor))
	}
	p.draft.Anchor = radio.AnchorFollow
	if !strings.Contains(p.rowValue(rowAnchor), "Follow current track") {
		t.Fatal(p.rowValue(rowAnchor))
	}
	p = focus(t, newTestPanel(true), rowDiscovery)
	out := plain(p.Render(60, 12, styles.NewStyles(true), nil, Preview{}))
	if !strings.Contains(out, "last 90 days") || !strings.Contains(out, "1–2 of 10") {
		t.Fatal(out)
	}
	p = focus(t, p, rowCommit)
	out = plain(p.Render(60, 12, styles.NewStyles(true), nil, Preview{}))
	if !strings.Contains(out, "7–10 of 10") || !strings.Contains(out, "[ Apply ]") {
		t.Fatal(out)
	}
}

func TestChangedMarkersUseFilterIdentityAndSemanticEquality(t *testing.T) {
	p := newTestPanel(true)
	p.draft.Include.Genres = []string{"JAZZ"}
	if p.rowDirty(rowIncludeGenres) {
		t.Fatal("case-only change marked dirty")
	}
	p.applied.Include.TagIDs = []int64{1, 2}
	p.draft.Include.TagIDs = []int64{2, 1}
	if p.rowDirty(rowIncludeTags) {
		t.Fatal("reordered set marked dirty")
	}
	p.catalog.Tags[1].Name = p.catalog.Tags[0].Name
	p.applied.Include.TagIDs = []int64{1}
	p.draft.Include.TagIDs = []int64{2}
	if !p.rowDirty(rowIncludeTags) {
		t.Fatal("different tag IDs hidden by identical labels")
	}
}
