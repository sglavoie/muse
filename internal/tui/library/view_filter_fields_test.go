package library

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	musiclib "github.com/sglavoie/muse/internal/library"
)

// TestFieldSpecificAlbumHighlight verifies that a field-specific album query
// (l:debut) produces bold ANSI codes in the album column.
func TestFieldSpecificAlbumHighlight(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	}
	idx := buildIndex(tracks)
	fake := highlightFake{
		results: []musiclib.Match{
			{TrackIndex: 0, AlbumPositions: []int{0, 1, 2, 3, 4}},
		},
	}
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  fake,
	}
	v = v.ActivateFilter()
	v = v.AppendFilterChar('l')
	v = v.AppendFilterChar(':')
	v = v.AppendFilterChar('d')
	got := v.Render()
	boldDebut := styledText(lipgloss.NewStyle().Bold(true), "Debut")
	if !strings.Contains(got, boldDebut) {
		t.Errorf("expected bold 'Debut' in album column for field-specific query, got:\n%q", got)
	}
}

// TestFieldSpecificArtistNarrowsResults verifies that a:alice only shows
// tracks with "alice" in the artist field.
func TestFieldSpecificArtistNarrowsResults(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v = v.ActivateFilter()
	for _, ch := range "a:Alice" {
		v = v.AppendFilterChar(ch)
	}
	got := v.Render()
	if !strings.Contains(got, "Alice") {
		t.Errorf("a:Alice should show Alice tracks, got:\n%s", got)
	}
	if strings.Contains(ansi.Strip(got), "Bob") {
		t.Errorf("a:Alice should not show Bob tracks, got:\n%s", got)
	}
}

// TestGeneralSearchMatchesAlbum verifies that a plain (unprefixed) query
// matches album text. This is new behaviour — previously album was excluded.
func TestGeneralSearchMatchesAlbum(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{
		Index:  idx,
		Width:  80,
		Height: 24,
		Fuzzy:  musiclib.FakeFuzzySearcher{},
	}
	v = v.ActivateFilter()
	for _, ch := range "Debut" {
		v = v.AppendFilterChar(ch)
	}
	got := v.Render()
	if !strings.Contains(got, "Alice") {
		t.Errorf("searching 'Debut' should match Alice's album, got:\n%s", got)
	}
}

// TestFieldSpecificMatchedIndexesNil verifies that for field-specific queries,
// the general MatchedIndexes path is not used (album is highlighted via per-field
// positions, not via the concatenated string offsets).
func TestFieldSpecificMatchedIndexesNil(t *testing.T) {
	tracks := []musiclib.Track{
		{Artist: "Radiohead", Title: "Creep", Album: "Pablo Honey", Duration: 238},
	}
	idx := buildIndex(tracks)
	fake := highlightFake{
		results: []musiclib.Match{
			{
				TrackIndex:      0,
				ArtistPositions: []int{0, 1, 2, 3, 4, 5, 6, 7, 8},
				// MatchedIndexes intentionally nil for field-specific
			},
		},
	}
	v := View{
		Index:  idx,
		Width:  120,
		Height: 24,
		Fuzzy:  fake,
	}
	v = v.ActivateFilter()
	for _, ch := range "a:radiohead" {
		v = v.AppendFilterChar(ch)
	}
	got := v.Render()
	// Artist should be highlighted (all 9 chars grouped into one bold run)
	boldRadiohead := styledText(lipgloss.NewStyle().Bold(true), "Radiohead")
	if !strings.Contains(got, boldRadiohead) {
		t.Errorf("expected bold 'Radiohead' in artist column for field-specific query, got:\n%q", got)
	}
}
