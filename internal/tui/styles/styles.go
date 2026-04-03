package styles

import "charm.land/lipgloss/v2"

// Styles holds the shared lipgloss styles used across TUI views.
type Styles struct {
	Dim                     lipgloss.Style
	Accent                  lipgloss.Style
	AccentMark              lipgloss.Style // foreground-only accent for "●" marker on non-focused rows
	Bold                    lipgloss.Style
	Heading                 lipgloss.Style // bold + blue + double-underlined for section titles
	Success                 lipgloss.Style // green foreground for playing state
	Warning                 lipgloss.Style // gold foreground for paused state
	Error                   lipgloss.Style // red foreground for errors / stopped
	NowPlaying              lipgloss.Style // bold green foreground for currently playing track
	NowPlayingFavorite      lipgloss.Style // bold amber/gold foreground for playing + favorited track
	NowPlayingFavArtist     lipgloss.Style // bold purple foreground for playing + favorited artist
	NowPlayingFavAlbum      lipgloss.Style // bold cyan foreground for playing + favorited album
	Favorite                lipgloss.Style // amber/gold foreground for favorited tracks
	FavoriteAccent          lipgloss.Style // accent background + favorite foreground for focused favorites
	FavArtist               lipgloss.Style // purple foreground for favorited artists
	FavAlbum                lipgloss.Style // cyan foreground for favorited albums
	FavArtistAccent         lipgloss.Style // accent background + purple foreground for focused favorited artists
	FavAlbumAccent          lipgloss.Style // accent background + cyan foreground for focused favorited albums
	ColHead                 lipgloss.Style // bold + faint for column headers
	StatusBar               lipgloss.Style // full-width status bar background + foreground
	StatusNotification      lipgloss.Style // notification badge: amber on contrasting bg
	StatusNotificationError lipgloss.Style // error notification: red on contrasting bg
	Badge                   lipgloss.Style // single-char badge with lighter background (title bar view numbers)
	HasDark                 bool
}

// NewStyles constructs a Styles value for the given terminal background.
func NewStyles(hasDark bool) Styles {
	ld := lipgloss.LightDark(hasDark)
	return Styles{
		Dim:        lipgloss.NewStyle().Faint(true),
		Accent:     lipgloss.NewStyle().Background(ld(lipgloss.Color("#d0d7e3"), lipgloss.Color("#2c3e50"))),
		AccentMark: lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#4078f2"), lipgloss.Color("#61afef"))),
		Bold:       lipgloss.NewStyle().Bold(true),
		Heading: lipgloss.NewStyle().Bold(true).
			UnderlineStyle(lipgloss.UnderlineDouble).
			UnderlineColor(ld(lipgloss.Color("#6a9edb"), lipgloss.Color("#3a6fbf"))).
			Foreground(ld(lipgloss.Color("#4078f2"), lipgloss.Color("#61afef"))),
		Success:                 lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#50a14f"), lipgloss.Color("#98c379"))),
		Warning:                 lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#986801"), lipgloss.Color("#e5c07b"))),
		Error:                   lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#e45649"), lipgloss.Color("#e06c75"))),
		NowPlaying:              lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#50a14f"), lipgloss.Color("#98c379"))),
		NowPlayingFavorite:      lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#986801"), lipgloss.Color("#e5c07b"))),
		NowPlayingFavArtist:     lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#a626a4"), lipgloss.Color("#c678dd"))),
		NowPlayingFavAlbum:      lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#0184bc"), lipgloss.Color("#56b6c2"))),
		Favorite:                lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#986801"), lipgloss.Color("#e5c07b"))),
		FavoriteAccent:          lipgloss.NewStyle().Background(ld(lipgloss.Color("#d0d7e3"), lipgloss.Color("#2c3e50"))).Foreground(ld(lipgloss.Color("#986801"), lipgloss.Color("#e5c07b"))),
		FavArtist:               lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#a626a4"), lipgloss.Color("#c678dd"))),
		FavAlbum:                lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#0184bc"), lipgloss.Color("#56b6c2"))),
		FavArtistAccent:         lipgloss.NewStyle().Background(ld(lipgloss.Color("#d0d7e3"), lipgloss.Color("#2c3e50"))).Foreground(ld(lipgloss.Color("#a626a4"), lipgloss.Color("#c678dd"))),
		FavAlbumAccent:          lipgloss.NewStyle().Background(ld(lipgloss.Color("#d0d7e3"), lipgloss.Color("#2c3e50"))).Foreground(ld(lipgloss.Color("#0184bc"), lipgloss.Color("#56b6c2"))),
		ColHead:                 lipgloss.NewStyle().Bold(true).Faint(true).UnderlineStyle(lipgloss.UnderlineDotted),
		StatusBar:               lipgloss.NewStyle().Background(ld(lipgloss.Color("#e8eaed"), lipgloss.Color("#282c34"))).Foreground(ld(lipgloss.Color("#383a42"), lipgloss.Color("#abb2bf"))),
		StatusNotification:      lipgloss.NewStyle().Background(ld(lipgloss.Color("#d0d7e3"), lipgloss.Color("#3b4252"))).Foreground(ld(lipgloss.Color("#986801"), lipgloss.Color("#e5c07b"))),
		StatusNotificationError: lipgloss.NewStyle().Background(ld(lipgloss.Color("#d0d7e3"), lipgloss.Color("#3b4252"))).Foreground(ld(lipgloss.Color("#e45649"), lipgloss.Color("#e06c75"))),
		Badge:                   lipgloss.NewStyle().Background(ld(lipgloss.Color("#e8eaed"), lipgloss.Color("#282c34"))).Foreground(ld(lipgloss.Color("#383a42"), lipgloss.Color("#abb2bf"))),
		HasDark:                 hasDark,
	}
}

// AccentFavRender renders text with the accent background. When isFav is
// true the combined foreground+background favorite accent style is used;
// otherwise the plain accent background style is applied. This avoids
// nesting Render calls whose ANSI resets would kill an outer background.
func AccentFavRender(accent, favAccent lipgloss.Style, isFav bool, text string) string {
	if isFav {
		return favAccent.Render(text)
	}
	return accent.Render(text)
}
