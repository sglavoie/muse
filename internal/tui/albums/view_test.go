package albums

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// --- sample data helpers ---

func sampleTracks() []library.Track {
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Duration: 183},
		{Artist: "Alice", Album: "Alpha", Title: "Track A2", Duration: 245},
		{Artist: "Alice", Album: "Beta", Title: "Track B1", Duration: 300},
		{Artist: "Bob", Album: "Gamma", Title: "Track G1", Duration: 120},
		{Artist: "Bob", Album: "Gamma", Title: "Track G2", Duration: 60},
		{Artist: "Charlie", Album: "Delta", Title: "Track D1", Duration: 3661},
	}
}

// --- DeriveAllAlbums tests ---

func TestDeriveAllAlbumsUniqueSorted(t *testing.T) {
	albums := DeriveAllAlbums(sampleTracks())
	want := []string{"Alpha", "Beta", "Delta", "Gamma"}
	if len(albums) != len(want) {
		t.Fatalf("got %d albums, want %d: %v", len(albums), len(want), albums)
	}
	for i, a := range albums {
		if a != want[i] {
			t.Errorf("albums[%d] = %q, want %q", i, a, want[i])
		}
	}
}

func TestDeriveAllAlbumsEmpty(t *testing.T) {
	albums := DeriveAllAlbums(nil)
	if len(albums) != 0 {
		t.Errorf("expected empty, got %v", albums)
	}
}

func TestDeriveAllAlbumCounts(t *testing.T) {
	counts := DeriveAllAlbumCounts(sampleTracks())
	want := map[string]int{"Alpha": 2, "Beta": 1, "Gamma": 2, "Delta": 1}
	if len(counts) != len(want) {
		t.Fatalf("DeriveAllAlbumCounts: expected %d entries, got %d (%v)", len(want), len(counts), counts)
	}
	for name, wantN := range want {
		if got := counts[name]; got != wantN {
			t.Errorf("DeriveAllAlbumCounts[%q]: expected %d, got %d", name, wantN, got)
		}
	}
}

func TestDeriveAllAlbumCountsEmptyAndUnknown(t *testing.T) {
	if got := DeriveAllAlbumCounts(nil); got != nil {
		t.Errorf("DeriveAllAlbumCounts(nil): expected nil, got %v", got)
	}
	counts := DeriveAllAlbumCounts([]library.Track{
		{Album: "Alpha"},
		{Album: ""},
		{Album: ""},
	})
	if counts["(unknown album)"] != 2 {
		t.Errorf("expected unknown-album count = 2, got %d", counts["(unknown album)"])
	}
}

// --- DeriveAlbumTracks tests ---

func TestDeriveAlbumTracks(t *testing.T) {
	tracks := DeriveAlbumTracks(sampleTracks(), "Alpha")
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2", len(tracks))
	}
	if tracks[0].Title != "Track A1" || tracks[1].Title != "Track A2" {
		t.Errorf("unexpected tracks: %v", tracks)
	}
}

func TestDeriveAlbumTracksNoMatch(t *testing.T) {
	tracks := DeriveAlbumTracks(sampleTracks(), "Nonexistent")
	if len(tracks) != 0 {
		t.Errorf("expected empty, got %v", tracks)
	}
}

// --- render tests ---

func TestRenderAlbumsEmpty(t *testing.T) {
	v := View{Level: AlbumLevel}
	out := v.Render()
	if !strings.Contains(out, "No albums") {
		t.Fatalf("expected albums empty hint in output, got:\n%s", out)
	}
}

func TestRenderAlbumsShowsNames(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Width:  80,
		Albums: []string{"Alpha", "Beta", "Gamma"},
	}
	out := v.Render()
	for _, want := range []string{"Alpha", "Beta", "Gamma"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderAlbumsShowsCounts(t *testing.T) {
	v := View{
		Level:       AlbumLevel,
		Width:       80,
		Albums:      []string{"Alpha", "Beta"},
		AlbumCounts: map[string]int{"Alpha": 12, "Beta": 4},
	}
	out := v.Render()
	for _, want := range []string{"Alpha", "Beta", "12", "4"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderAlbumsCountDropsAtNarrowWidth(t *testing.T) {
	v := View{
		Level:       AlbumLevel,
		Width:       10,
		Albums:      []string{"Alpha"},
		AlbumCounts: map[string]int{"Alpha": 99},
	}
	out := v.Render()
	if strings.Contains(out, "99") {
		t.Errorf("expected count to drop at narrow width, got:\n%s", out)
	}
	if !strings.Contains(out, "Alpha") {
		t.Errorf("expected label visible at narrow width, got:\n%s", out)
	}
}

func TestRenderTracksShowsTitles(t *testing.T) {
	v := View{
		Level:         TrackLevel,
		Width:         80,
		SelectedAlbum: "Alpha",
		Tracks: []library.Track{
			{Title: "Track A1", Duration: 183},
			{Title: "Track A2", Duration: 245},
		},
	}
	out := v.Render()
	if !strings.Contains(out, "Track A1") {
		t.Errorf("expected Track A1, got:\n%s", out)
	}
	if !strings.Contains(out, "Track A2") {
		t.Errorf("expected Track A2, got:\n%s", out)
	}
	if !strings.Contains(out, "Alpha") {
		t.Errorf("expected album heading 'Alpha', got:\n%s", out)
	}
}

func TestRenderTracksEmpty(t *testing.T) {
	v := View{Level: TrackLevel, SelectedAlbum: "Alpha"}
	out := v.Render()
	if !strings.Contains(out, "No tracks") {
		t.Fatalf("expected tracks empty hint in output, got:\n%s", out)
	}
}

// --- OpenTracks / Back tests ---

func TestOpenTracksAndBack(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Albums: []string{"Alpha", "Beta"},
		Cursor: 1,
		Width:  80,
	}
	tracks := []library.Track{{Title: "T1"}, {Title: "T2"}}
	v = v.OpenTracks(tracks)

	if v.Level != TrackLevel {
		t.Fatalf("expected TrackLevel, got %d", v.Level)
	}
	if v.Cursor != 0 {
		t.Errorf("expected cursor 0, got %d", v.Cursor)
	}
	if v.AlbumCursor != 1 {
		t.Errorf("expected saved AlbumCursor=1, got %d", v.AlbumCursor)
	}

	v = v.Back()
	if v.Level != AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %d", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("expected restored cursor=1, got %d", v.Cursor)
	}
}

func TestBackAtAlbumLevelIsNoop(t *testing.T) {
	v := View{Level: AlbumLevel, Cursor: 2}
	v = v.Back()
	if v.Level != AlbumLevel || v.Cursor != 2 {
		t.Errorf("Back at AlbumLevel should be no-op, got level=%d cursor=%d", v.Level, v.Cursor)
	}
}

// --- navigation tests ---

func TestCursorUpDown(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Albums: []string{"A", "B", "C"},
		Width:  80,
		Height: 20,
	}
	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("expected cursor=1, got %d", v.Cursor)
	}
	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Errorf("expected cursor=2, got %d", v.Cursor)
	}
	v = v.CursorDown() // at last, no-op
	if v.Cursor != 2 {
		t.Errorf("expected cursor=2 (clamped), got %d", v.Cursor)
	}
	v = v.CursorUp()
	if v.Cursor != 1 {
		t.Errorf("expected cursor=1, got %d", v.Cursor)
	}
}

func TestGoToTopBottom(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Albums: []string{"A", "B", "C", "D", "E"},
		Width:  80,
		Height: 20,
	}
	v = v.GoToBottom()
	if v.Cursor != 4 {
		t.Errorf("expected cursor=4, got %d", v.Cursor)
	}
	v = v.GoToTop()
	if v.Cursor != 0 {
		t.Errorf("expected cursor=0, got %d", v.Cursor)
	}
}

// --- filter tests ---

func TestEffectiveAlbumsFiltered(t *testing.T) {
	v := View{
		Level:      AlbumLevel,
		Albums:     []string{"Alpha", "Beta", "Gamma"},
		FilterText: "alp",
	}
	items := v.EffectiveAlbums()
	if len(items) != 1 || items[0] != "Alpha" {
		t.Errorf("expected [Alpha], got %v", items)
	}
}

func TestEffectiveTracksFiltered(t *testing.T) {
	v := View{
		Level: TrackLevel,
		Tracks: []library.Track{
			{Title: "Hello World"},
			{Title: "Goodbye"},
		},
		FilterText: "hello",
	}
	items := v.EffectiveTracks()
	if len(items) != 1 || items[0].Title != "Hello World" {
		t.Errorf("expected [Hello World], got %v", items)
	}
}

// --- render with styles ---

func TestRenderWithStyles(t *testing.T) {
	s := styles.NewStyles(true)
	v := View{
		Level:  AlbumLevel,
		Width:  80,
		Height: 20,
		Albums: []string{"Alpha"},
		Styles: s,
	}
	out := v.Render()
	if out == "" {
		t.Error("expected non-empty render output")
	}
}

func TestStripFilterActive(t *testing.T) {
	s := styles.NewStyles(true)
	v := View{
		Level:        AlbumLevel,
		Albums:       []string{"a"},
		FilterActive: true,
		FilterText:   "test",
		FilterView:   "test",
		Width:        80,
		Styles:       s,
	}
	out := ansi.Strip(v.renderStateStrip(1, 1))
	if !strings.Contains(out, `filter "test"`) {
		t.Errorf("expected filter chunk, got: %q", out)
	}
}

func TestStripFilterCommitted(t *testing.T) {
	s := styles.NewStyles(true)
	v := View{
		Level:      AlbumLevel,
		Albums:     []string{"a"},
		FilterText: "test",
		Width:      80,
		Styles:     s,
	}
	out := ansi.Strip(v.renderStateStrip(1, 1))
	if !strings.Contains(out, `filter "test"`) {
		t.Errorf("expected committed filter chunk, got: %q", out)
	}
}

func TestFilterNoMatch(t *testing.T) {
	v := View{
		Level:      AlbumLevel,
		Width:      80,
		Albums:     []string{"Alpha"},
		FilterText: "zzz",
	}
	out := v.Render()
	if !strings.Contains(out, "no matches") {
		t.Errorf("expected '(no matches)', got:\n%s", out)
	}
}

// --- selection marker tests ---

// TestRenderTracksShowsSelectionMarker verifies that a selected track renders
// the bullet marker (●) and an unselected track does not.
func TestRenderTracksShowsSelectionMarker(t *testing.T) {
	tracks := []library.Track{
		{Title: "Selected", Hash: "h1", Duration: 60},
		{Title: "Not Selected", Hash: "h2", Duration: 60},
	}
	v := View{
		Level:          TrackLevel,
		Width:          80,
		SelectedAlbum:  "Alpha",
		Tracks:         tracks,
		SelectedHashes: map[string]struct{}{"h1": {}},
	}
	out := v.Render()
	if !strings.Contains(out, "●") {
		t.Errorf("expected selection marker ● in output for selected track, got:\n%s", out)
	}
}

// TestRenderTracksNoMarkersWhenNoneSelected verifies that when no tracks are
// selected, the bullet marker does not appear in the rendered output.
func TestRenderTracksNoMarkersWhenNoneSelected(t *testing.T) {
	tracks := []library.Track{
		{Title: "Track A", Hash: "h1", Duration: 60},
		{Title: "Track B", Hash: "h2", Duration: 60},
	}
	v := View{
		Level:         TrackLevel,
		Width:         80,
		SelectedAlbum: "Alpha",
		Tracks:        tracks,
	}
	out := v.Render()
	if strings.Contains(out, "●") {
		t.Errorf("did not expect selection marker ● when nothing is selected, got:\n%s", out)
	}
}
