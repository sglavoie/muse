package styles_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestNewStyles(t *testing.T) {
	renderers := []struct {
		name   string
		render func(styles.Styles) string
	}{
		{"Dim", func(s styles.Styles) string { return s.Dim.Render("x") }},
		{"Accent", func(s styles.Styles) string { return s.Accent.Render("x") }},
		{"Bold", func(s styles.Styles) string { return s.Bold.Render("x") }},
		{"Heading", func(s styles.Styles) string { return s.Heading.Render("x") }},
		{"Success", func(s styles.Styles) string { return s.Success.Render("x") }},
		{"Warning", func(s styles.Styles) string { return s.Warning.Render("x") }},
		{"Error", func(s styles.Styles) string { return s.Error.Render("x") }},
		{"ColHead", func(s styles.Styles) string { return s.ColHead.Render("x") }},
		{"AccentMark", func(s styles.Styles) string { return s.AccentMark.Render("x") }},
		{"StatusBar", func(s styles.Styles) string { return s.StatusBar.Render("x") }},
		{"FavArtist", func(s styles.Styles) string { return s.FavArtist.Render("x") }},
		{"FavAlbum", func(s styles.Styles) string { return s.FavAlbum.Render("x") }},
		{"FavArtistAccent", func(s styles.Styles) string { return s.FavArtistAccent.Render("x") }},
		{"FavAlbumAccent", func(s styles.Styles) string { return s.FavAlbumAccent.Render("x") }},
		{"NowPlayingFavArtist", func(s styles.Styles) string { return s.NowPlayingFavArtist.Render("x") }},
		{"NowPlayingFavAlbum", func(s styles.Styles) string { return s.NowPlayingFavAlbum.Render("x") }},
		{"TextPrimary", func(s styles.Styles) string { return s.TextPrimary.Render("x") }},
		{"TextSecondary", func(s styles.Styles) string { return s.TextSecondary.Render("x") }},
		{"TextTertiary", func(s styles.Styles) string { return s.TextTertiary.Render("x") }},
		{"NowPlayingTitle", func(s styles.Styles) string { return s.NowPlayingTitle.Render("x") }},
	}

	for _, hasDark := range []bool{true, false} {
		s := styles.NewStyles(hasDark)
		if s.HasDark != hasDark {
			t.Errorf("NewStyles(%v).HasDark = %v, want %v", hasDark, s.HasDark, hasDark)
		}

		for _, renderer := range renderers {
			if got := renderer.render(s); got == "" {
				t.Errorf("NewStyles(%v).%s.Render() returned empty string", hasDark, renderer.name)
			}
		}
	}
}
