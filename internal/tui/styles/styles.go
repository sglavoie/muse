package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Surface layer colors — backgrounds for status bar, zebra stripe, and accent selection.
const (
	colorSurfaceL0Dark  = "#16161e" // L0: status bar background
	colorSurfaceL0Light = "#d8dae0"
	colorSurfaceL2Dark  = "#24283b" // L2: zebra stripe background
	colorSurfaceL2Light = "#e4e6eb"
	colorSurfaceAccentDark  = "#2e3d55" // accent / selection background
	colorSurfaceAccentLight = "#d0d7e3"
	colorSurfaceNotifDark  = "#3b4252" // notification badge background
	colorSurfaceNotifLight = "#d0d7e3"
)

// Now-playing warm background colors.
const (
	colorNowPlayingBgDark  = "#2e2a1f"
	colorNowPlayingBgLight = "#fff8e8"
)

// Accent colors — highlights, focus rings, progress bar, active tabs.
const (
	colorAccentPrimaryDark   = "#7aa2f7"
	colorAccentPrimaryLight  = "#4078f2"
	colorAccentUnderlineDark  = "#3a5fa8" // Heading underline complement for #7aa2f7
	colorAccentUnderlineLight = "#6a9edb"
)

// Now-playing amber — title column foreground for the playing row.
const (
	colorAmberDark  = "#e0af68"
	colorAmberLight = "#986801"
)

// Status bar foreground text.
const (
	colorFgStatusBarDark  = "#abb2bf"
	colorFgStatusBarLight = "#383a42"
)

// Semantic state foreground colors.
const (
	colorFgSuccessLight = "#50a14f"
	colorFgSuccessDark  = "#98c379"
	colorFgWarningLight = "#986801"
	colorFgWarningDark  = "#e5c07b"
	colorFgErrorLight   = "#e45649"
	colorFgErrorDark    = "#e06c75"
)

// Text hierarchy foreground colors — primary, secondary, tertiary.
const (
	colorTextPrimaryLight   = "#343b58"
	colorTextPrimaryDark    = "#c0caf5"
	colorTextSecondaryLight = "#6172a9"
	colorTextSecondaryDark  = "#a9b1d6"
	colorTextTertiaryLight  = "#9ca0b0"
	colorTextTertiaryDark   = "#565f89"
)

// Favorite / special-label foreground colors.
const (
	colorFgFavArtistLight = "#a626a4"
	colorFgFavArtistDark  = "#c678dd"
	colorFgFavAlbumLight  = "#0184bc"
	colorFgFavAlbumDark   = "#56b6c2"
)

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
	ColHead                 lipgloss.Style // bold + tertiary foreground + dotted underline for column headers
	StatusBar               lipgloss.Style // full-width status bar background + foreground
	StatusNotification      lipgloss.Style // notification badge: amber on contrasting bg
	StatusNotificationError lipgloss.Style // error notification: red on contrasting bg
	Badge                   lipgloss.Style // single-char badge with lighter background (title bar view numbers)
	HelpKey                 lipgloss.Style // bold accent foreground for keybinding keys in help overlay
	ProgressFilled          lipgloss.Style // accent-colored "█" blocks for filled portion of progress bar
	ProgressEmpty           lipgloss.Style // dim "░" blocks for empty portion of progress bar
	TextPrimary             lipgloss.Style // primary text: artist, title columns
	TextSecondary           lipgloss.Style // secondary text: album column
	TextTertiary            lipgloss.Style // tertiary text: track numbers, durations, inactive tabs
	NowPlayingTitle         lipgloss.Style // amber fg + warm bg for title column in the now-playing row
	ZebraBg                 color.Color
	NowPlayingBg            color.Color
	NowPlayingRow           lipgloss.Style // bold green fg + amber bg for now-playing row (non-focused)
	NowPlayingRowFavorite   lipgloss.Style // bold amber/gold fg + amber bg for now-playing + favorited track
	NowPlayingRowFavArtist  lipgloss.Style // bold purple fg + amber bg for now-playing + favorited artist
	NowPlayingRowFavAlbum   lipgloss.Style // bold cyan fg + amber bg for now-playing + favorited album
	HasDark                 bool
}

// NewStyles constructs a Styles value for the given terminal background.
func NewStyles(hasDark bool) Styles {
	ld := lipgloss.LightDark(hasDark)
	return Styles{
		Dim:    lipgloss.NewStyle().Faint(true),
		Accent: lipgloss.NewStyle().Background(ld(lipgloss.Color(colorSurfaceAccentLight), lipgloss.Color(colorSurfaceAccentDark))),
		AccentMark: lipgloss.NewStyle().Foreground(ld(lipgloss.Color(colorAccentPrimaryLight), lipgloss.Color(colorAccentPrimaryDark))),
		Bold:   lipgloss.NewStyle().Bold(true),
		Heading: lipgloss.NewStyle().Bold(true).
			UnderlineStyle(lipgloss.UnderlineDouble).
			UnderlineColor(ld(lipgloss.Color(colorAccentUnderlineLight), lipgloss.Color(colorAccentUnderlineDark))).
			Foreground(ld(lipgloss.Color(colorAccentPrimaryLight), lipgloss.Color(colorAccentPrimaryDark))),
		Success:                 lipgloss.NewStyle().Foreground(ld(lipgloss.Color(colorFgSuccessLight), lipgloss.Color(colorFgSuccessDark))),
		Warning:                 lipgloss.NewStyle().Foreground(ld(lipgloss.Color(colorFgWarningLight), lipgloss.Color(colorFgWarningDark))),
		Error:                   lipgloss.NewStyle().Foreground(ld(lipgloss.Color(colorFgErrorLight), lipgloss.Color(colorFgErrorDark))),
		NowPlaying:              lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color(colorFgSuccessLight), lipgloss.Color(colorFgSuccessDark))),
		NowPlayingFavorite:      lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color(colorFgWarningLight), lipgloss.Color(colorFgWarningDark))),
		NowPlayingFavArtist:     lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color(colorFgFavArtistLight), lipgloss.Color(colorFgFavArtistDark))),
		NowPlayingFavAlbum:      lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color(colorFgFavAlbumLight), lipgloss.Color(colorFgFavAlbumDark))),
		Favorite:                lipgloss.NewStyle().Foreground(ld(lipgloss.Color(colorFgWarningLight), lipgloss.Color(colorFgWarningDark))),
		FavoriteAccent:          lipgloss.NewStyle().Background(ld(lipgloss.Color(colorSurfaceAccentLight), lipgloss.Color(colorSurfaceAccentDark))).Foreground(ld(lipgloss.Color(colorFgWarningLight), lipgloss.Color(colorFgWarningDark))),
		FavArtist:               lipgloss.NewStyle().Foreground(ld(lipgloss.Color(colorFgFavArtistLight), lipgloss.Color(colorFgFavArtistDark))),
		FavAlbum:                lipgloss.NewStyle().Foreground(ld(lipgloss.Color(colorFgFavAlbumLight), lipgloss.Color(colorFgFavAlbumDark))),
		FavArtistAccent:         lipgloss.NewStyle().Background(ld(lipgloss.Color(colorSurfaceAccentLight), lipgloss.Color(colorSurfaceAccentDark))).Foreground(ld(lipgloss.Color(colorFgFavArtistLight), lipgloss.Color(colorFgFavArtistDark))),
		FavAlbumAccent:          lipgloss.NewStyle().Background(ld(lipgloss.Color(colorSurfaceAccentLight), lipgloss.Color(colorSurfaceAccentDark))).Foreground(ld(lipgloss.Color(colorFgFavAlbumLight), lipgloss.Color(colorFgFavAlbumDark))),
		ColHead: lipgloss.NewStyle().Bold(true).
			Foreground(ld(lipgloss.Color(colorTextTertiaryLight), lipgloss.Color(colorTextTertiaryDark))).
			UnderlineStyle(lipgloss.UnderlineDotted),
		StatusBar:               lipgloss.NewStyle().Background(ld(lipgloss.Color(colorSurfaceL0Light), lipgloss.Color(colorSurfaceL0Dark))).Foreground(ld(lipgloss.Color(colorFgStatusBarLight), lipgloss.Color(colorFgStatusBarDark))),
		StatusNotification:      lipgloss.NewStyle().Background(ld(lipgloss.Color(colorSurfaceNotifLight), lipgloss.Color(colorSurfaceNotifDark))).Foreground(ld(lipgloss.Color(colorFgWarningLight), lipgloss.Color(colorFgWarningDark))),
		StatusNotificationError: lipgloss.NewStyle().Background(ld(lipgloss.Color(colorSurfaceNotifLight), lipgloss.Color(colorSurfaceNotifDark))).Foreground(ld(lipgloss.Color(colorFgErrorLight), lipgloss.Color(colorFgErrorDark))),
		Badge:                   lipgloss.NewStyle().Background(ld(lipgloss.Color(colorSurfaceL0Light), lipgloss.Color(colorSurfaceL0Dark))).Foreground(ld(lipgloss.Color(colorTextTertiaryLight), lipgloss.Color(colorTextTertiaryDark))),
		HelpKey:                 lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color(colorAccentPrimaryLight), lipgloss.Color(colorAccentPrimaryDark))),
		ProgressFilled:          lipgloss.NewStyle().Foreground(ld(lipgloss.Color(colorAccentPrimaryLight), lipgloss.Color(colorAccentPrimaryDark))),
		ProgressEmpty:           lipgloss.NewStyle().Faint(true),
		TextPrimary:             lipgloss.NewStyle().Foreground(ld(lipgloss.Color(colorTextPrimaryLight), lipgloss.Color(colorTextPrimaryDark))),
		TextSecondary:           lipgloss.NewStyle().Foreground(ld(lipgloss.Color(colorTextSecondaryLight), lipgloss.Color(colorTextSecondaryDark))),
		TextTertiary:            lipgloss.NewStyle().Foreground(ld(lipgloss.Color(colorTextTertiaryLight), lipgloss.Color(colorTextTertiaryDark))),
		NowPlayingTitle:         lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color(colorAmberLight), lipgloss.Color(colorAmberDark))).Background(ld(lipgloss.Color(colorNowPlayingBgLight), lipgloss.Color(colorNowPlayingBgDark))),
		ZebraBg:                 ld(lipgloss.Color(colorSurfaceL2Light), lipgloss.Color(colorSurfaceL2Dark)),
		NowPlayingBg:            ld(lipgloss.Color(colorNowPlayingBgLight), lipgloss.Color(colorNowPlayingBgDark)),
		NowPlayingRow:           lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color(colorFgSuccessLight), lipgloss.Color(colorFgSuccessDark))).Background(ld(lipgloss.Color(colorNowPlayingBgLight), lipgloss.Color(colorNowPlayingBgDark))),
		NowPlayingRowFavorite:   lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color(colorFgWarningLight), lipgloss.Color(colorFgWarningDark))).Background(ld(lipgloss.Color(colorNowPlayingBgLight), lipgloss.Color(colorNowPlayingBgDark))),
		NowPlayingRowFavArtist:  lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color(colorFgFavArtistLight), lipgloss.Color(colorFgFavArtistDark))).Background(ld(lipgloss.Color(colorNowPlayingBgLight), lipgloss.Color(colorNowPlayingBgDark))),
		NowPlayingRowFavAlbum:   lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color(colorFgFavAlbumLight), lipgloss.Color(colorFgFavAlbumDark))).Background(ld(lipgloss.Color(colorNowPlayingBgLight), lipgloss.Color(colorNowPlayingBgDark))),
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

// TagColorStyle returns a foreground-only lipgloss.Style for the given hex
// color string (e.g. "#ff6600"). When hex is empty the returned style is the
// zero value (unstyled — callers get plain text).
func TagColorStyle(hex string) lipgloss.Style {
	if hex == "" {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
}

// ColoredDot renders "●" in the given tag color, or plain "●" when color is empty.
func ColoredDot(tagColor string) string {
	return TagColorStyle(tagColor).Render("●")
}

// WithZebraBg returns style with ZebraBg background when isOdd, else style unchanged.
func WithZebraBg(style lipgloss.Style, zebraBg color.Color, isOdd bool) lipgloss.Style {
	if isOdd {
		return style.Background(zebraBg)
	}
	return style
}
