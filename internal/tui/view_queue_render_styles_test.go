package tui

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"strings"
	"testing"
)

// TestQueueRenderNowPlayingFavoriteStyle verifies that a track which is both
// currently playing and a favorite gets the NowPlayingFavorite style.
func TestQueueRenderNowPlayingFavoriteStyle(t *testing.T) {
	v := makeQueue(120, []QueueEntry{
		{Title: "Normal", Artist: "A", Hash: "aaa", DurationMs: 60_000},
		{Title: "FavPlaying", Artist: "B", Hash: "bbb", DurationMs: 120_000},
	})
	v.cursor = 0
	favs := map[string]struct{}{"bbb": {}}
	s := styles.NewStyles(true)
	out := v.render(s, 1, PlaybackPlaying, favs, nil, nil)

	npfRef := s.NowPlayingFavorite.Background(s.NowPlayingBg).Render("REF")
	artistRef := s.TextPrimary.Background(s.NowPlayingBg).Render("REF")

	var rawLine string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(l, "FavPlaying") {
			rawLine = l
			break
		}
	}
	if rawLine == "" {
		t.Fatal("FavPlaying entry must appear in the output")
	}
	npfPrefix, _, _ := strings.Cut(npfRef, "REF")
	artistPrefix, _, _ := strings.Cut(artistRef, "REF")

	if !strings.Contains(rawLine, npfPrefix) {
		t.Errorf("now-playing+favorite title cell must use NowPlayingFavorite style; got: %q", rawLine)
	}
	if !strings.Contains(rawLine, artistPrefix) {
		t.Errorf("non-favorited artist cell of now-playing entry must use TextPrimary+NowPlayingBg style; got: %q", rawLine)
	}
}

// TestQueueRenderNarrowWidthUsesMax verifies that the queue render function
// uses max(flex, 4) to prevent negative column widths when the terminal is
// very narrow.
func TestQueueRenderNarrowWidthUsesMax(t *testing.T) {
	s := styles.NewStyles(true)
	v := QueueView{
		entries: []QueueEntry{
			{Hash: "aaa", Title: "T", Artist: "A", Album: "Al", DurationMs: 60000},
		},
		width:  10,
		height: 10,
	}
	got := v.render(s, -1, PlaybackIdle, nil, nil, nil)
	if got == "" {
		t.Error("render must produce non-empty output even with narrow width")
	}
}

// TestQueueRenderSelectedAndFocusedMarker verifies that a selected+focused
// entry uses a plain "●" glyph.
func TestQueueRenderSelectedAndFocusedMarker(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{{Title: "Focused", Artist: "A", DurationMs: 60_000, Selected: true}},
		cursor:  0,
		width:   120,
	}
	out := v.render(styles.NewStyles(true), -1, PlaybackIdle, nil, nil, nil)
	lines := strings.Split(out, "\n")
	var focusedLine string
	for _, l := range lines {
		if strings.Contains(l, "Focused") {
			focusedLine = l
			break
		}
	}
	if focusedLine == "" {
		t.Fatal("focused entry must appear in the output")
	}
	if !strings.Contains(focusedLine, "●") {
		t.Error("selected+focused entry must contain ● marker")
	}
}

// TestQueueRenderArtistOnlyFavoriteStyle verifies that when only the artist is
// favorited, only the artist cell receives FavArtist styling.
func TestQueueRenderArtistOnlyFavoriteStyle(t *testing.T) {
	v := makeQueue(120, []QueueEntry{{Title: "Song", Artist: "FavArtist", Album: "NormalAlbum", Hash: "aaa", DurationMs: 60_000}})
	v.cursor = 1
	favArtists := map[string]struct{}{"FavArtist": {}}
	s := styles.NewStyles(true)
	out := v.render(s, -1, PlaybackIdle, nil, favArtists, nil)

	artistRef := s.FavArtist.Render("REF")
	if ansi.Strip(artistRef) == artistRef {
		return
	}
	artistPrefix, _, _ := strings.Cut(artistRef, "REF")
	titleRef := s.Favorite.Render("REF")
	titlePrefix, _, _ := strings.Cut(titleRef, "REF")

	var rawLine string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(l, "Song") {
			rawLine = l
			break
		}
	}
	if rawLine == "" {
		t.Fatal("entry must appear in output")
	}
	if !strings.Contains(rawLine, artistPrefix) {
		t.Errorf("artist-only fav: artist cell must use FavArtist style; got: %q", rawLine)
	}
	if strings.Contains(rawLine, titlePrefix) {
		t.Errorf("artist-only fav: title cell must NOT use Favorite style; got: %q", rawLine)
	}
}

// TestQueueRenderAllThreeFavoritesStyle verifies that when track, artist, and
// album are all favorited, each cell independently receives its own color style.
func TestQueueRenderAllThreeFavoritesStyle(t *testing.T) {
	v := makeQueue(120, []QueueEntry{{Title: "Song", Artist: "FavArtist", Album: "FavAlbum", Hash: "favhash", DurationMs: 60_000}})
	v.cursor = 1
	favHashes := map[string]struct{}{"favhash": {}}
	favArtists := map[string]struct{}{"FavArtist": {}}
	favAlbums := map[string]struct{}{"FavAlbum": {}}
	s := styles.NewStyles(true)
	out := v.render(s, -1, PlaybackIdle, favHashes, favArtists, favAlbums)

	titleRef := s.Favorite.Render("REF")
	if ansi.Strip(titleRef) == titleRef {
		return
	}
	titlePrefix, _, _ := strings.Cut(titleRef, "REF")
	artistPrefix, _, _ := strings.Cut(s.FavArtist.Render("REF"), "REF")
	albumPrefix, _, _ := strings.Cut(s.FavAlbum.Render("REF"), "REF")

	var rawLine string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(l, "Song") {
			rawLine = l
			break
		}
	}
	if rawLine == "" {
		t.Fatal("entry must appear in output")
	}
	if !strings.Contains(rawLine, titlePrefix) {
		t.Errorf("all-3 fav: title cell must use Favorite style; got: %q", rawLine)
	}
	if !strings.Contains(rawLine, artistPrefix) {
		t.Errorf("all-3 fav: artist cell must use FavArtist style; got: %q", rawLine)
	}
	if !strings.Contains(rawLine, albumPrefix) {
		t.Errorf("all-3 fav: album cell must use FavAlbum style; got: %q", rawLine)
	}
}

// TestQueueRenderFocusedWithFavoriteUsesAccentBackground verifies that a
// focused+favorited row gets Accent background wrapping while preserving the
// favorite title color.
func TestQueueRenderFocusedWithFavoriteUsesAccentBackground(t *testing.T) {
	v := makeQueue(120, []QueueEntry{{Title: "FocFav", Artist: "A", Album: "B", Hash: "favhash", DurationMs: 60_000}})
	v.cursor = 0
	favHashes := map[string]struct{}{"favhash": {}}
	s := styles.NewStyles(true)
	out := v.render(s, -1, PlaybackIdle, favHashes, nil, nil)

	favAccentRef := s.FavoriteAccent.Render("REF")
	if ansi.Strip(favAccentRef) == favAccentRef {
		return
	}
	favAccentPrefix, _, _ := strings.Cut(favAccentRef, "REF")

	var rawLine string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(l, "FocFav") {
			rawLine = l
			break
		}
	}
	if rawLine == "" {
		t.Fatal("focused entry must appear in output")
	}
	if !strings.Contains(rawLine, favAccentPrefix) {
		t.Errorf("focused+fav row: title cell must use FavoriteAccent style; got: %q", rawLine)
	}
}
