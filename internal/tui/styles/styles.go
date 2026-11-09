package styles

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Surface layer colors — backgrounds for status bar, zebra stripe, and accent selection.
const (
	colorSurfaceL0Dark     = "#12151c" // L0: status bar background
	colorSurfaceL0Light    = "#e2e6ee"
	colorSurfaceL2Dark     = "#202632" // L2: zebra stripe background
	colorSurfaceL2Light    = "#edf0f6"
	colorSurfaceNotifDark  = "#3b4252" // notification badge background
	colorSurfaceNotifLight = "#d0d7e3"
)

// Accent colors — the default primary: the playing track, progress,
// spectrum, active tab, key hints and active playback modes.
const (
	colorAccentPrimaryDark  = "#8ab4ff"
	colorAccentPrimaryLight = "#315fbe"
)

// Status bar foreground text.
const (
	colorFgStatusBarDark  = "#abb2bf"
	colorFgStatusBarLight = "#383a42"
)

// Feedback foreground colors — reserved for outcomes and notices (saved,
// failed, needs attention, up/down deltas), never for decoration or state.
const (
	colorFgSuccessLight = "#287348"
	colorFgSuccessDark  = "#98c379"
	colorFgWarningLight = "#986801"
	colorFgWarningDark  = "#e5c07b"
	colorFgErrorLight   = "#b52f42"
	colorFgErrorDark    = "#e06c75"
)

// Favorite foreground — one hue for every favorite, whether track, artist or
// album: the column already says which. Rose keeps it apart from the default
// accent and from the warning amber; gold stands in when the accent (which
// may follow cover art) is itself near rose. See favoriteColor.
const (
	colorFgFavoriteLight    = "#a8336a"
	colorFgFavoriteDark     = "#eb9fc2"
	colorFgFavoriteAltLight = "#986801"
	colorFgFavoriteAltDark  = "#e0af68"
)

// Text hierarchy foreground colors — primary, secondary, tertiary.
const (
	colorTextPrimaryLight   = "#263248"
	colorTextPrimaryDark    = "#dce3f0"
	colorTextSecondaryLight = "#4e5d75"
	colorTextSecondaryDark  = "#b3bed2"
	colorTextTertiaryLight  = "#566177"
	colorTextTertiaryDark   = "#9aa6bd"
)

// Explicit dim foreground — perceptible but quiet, no reliance on terminal
// Faint rendering. Use this for separators, prefixes, and other low-priority
// content where Faint(true) historically vanished on dark themes.
const (
	colorDimLight = "#566177"
	colorDimDark  = "#9aa6bd"
)

// Styles holds the shared lipgloss styles used across TUI views.
type Styles struct {
	Background              color.Color // application canvas, independent of the terminal background
	Primary                 color.Color // selected primary color before text contrast adjustment
	Dim                     lipgloss.Style
	Accent                  lipgloss.Style // neutral focus background for the cursor row
	AccentMark              lipgloss.Style // foreground-only accent for "●" marker on non-focused rows
	Bold                    lipgloss.Style
	Heading                 lipgloss.Style // bold + primary + double-underlined for section titles
	ActiveTab               lipgloss.Style // bold + accent bg + high-contrast fg for the active tab in the title bar
	Success                 lipgloss.Style // feedback: an outcome that went well, upward deltas
	Warning                 lipgloss.Style // feedback: needs attention, confirmations, downward deltas
	Error                   lipgloss.Style // feedback: failures
	NowPlaying              lipgloss.Style // bold accent foreground for the playing track
	NowPlayingFavorite      lipgloss.Style // bold favorite foreground for playing + favorited track
	NowPlayingFavArtist     lipgloss.Style // bold favorite foreground for playing + favorited artist
	NowPlayingFavAlbum      lipgloss.Style // bold favorite foreground for playing + favorited album
	Favorite                lipgloss.Style // favorite foreground for favorited tracks
	FavoriteAccent          lipgloss.Style // focus background + favorite foreground for focused favorites
	FavArtist               lipgloss.Style // favorite foreground for favorited artists
	FavAlbum                lipgloss.Style // favorite foreground for favorited albums
	FavArtistAccent         lipgloss.Style // focus background + favorite foreground for focused favorited artists
	FavAlbumAccent          lipgloss.Style // focus background + favorite foreground for focused favorited albums
	ColHead                 lipgloss.Style // bold + tertiary foreground + full underline for column headers
	StatusBar               lipgloss.Style // full-width status bar background + foreground
	StatusBarPlayingBg      lipgloss.Style // accent-tinted bg + status bar fg when playing
	StatusBarStoppedBg      lipgloss.Style // plain bar bg + status bar fg when stopped
	StatusNotification      lipgloss.Style // notification badge: amber on contrasting bg
	StatusNotificationError lipgloss.Style // error notification: red on contrasting bg
	ModeRepeat              lipgloss.Style // bold accent fg for Repeat mode label
	ModeShuffle             lipgloss.Style // bold accent fg for Shuffle mode label
	ModeStopAfter           lipgloss.Style // bold accent fg for Stop Next mode label
	ModeRadio               lipgloss.Style // bold accent fg for Auto-DJ mode label
	Badge                   lipgloss.Style // single-char badge with lighter background (title bar view numbers)
	HelpKey                 lipgloss.Style // bold accent foreground for keybinding keys in help overlay
	ProgressFilled          lipgloss.Style // accent-colored "█" blocks for filled portion of progress bar
	ProgressEmpty           lipgloss.Style // dim empty-track color for progress bar
	TextPrimary             lipgloss.Style // primary text: artist, title columns
	TextSecondary           lipgloss.Style // secondary text: album column
	TextTertiary            lipgloss.Style // tertiary text: track numbers, durations, inactive tabs
	ArtistText              lipgloss.Style // artist column: secondary text
	AlbumText               lipgloss.Style // album column: quieter than the artist so rows scan title-first
	PaletteCategory         lipgloss.Style // dim, italic foreground for [category] prefix in command palette rows
	NowPlayingTitle         lipgloss.Style // accent fg + accent-tinted bg for title column in the now-playing row
	EmptyCard               lipgloss.Style // rounded-border padded container for empty-state cards
	EmptyKey                lipgloss.Style // bold accent foreground for key hints inside empty-state cards
	EmptyHint               lipgloss.Style // dim foreground for action labels inside empty-state cards
	Divider                 lipgloss.Style // tertiary-toned "─" for periodic row dividers in long lists
	BucketHeader            lipgloss.Style // bold secondary-toned label for sticky/inline bucket headers
	ZebraBg                 color.Color
	SelectionBg             color.Color
	NowPlayingBg            color.Color
	NowPlayingRow           lipgloss.Style // bold accent fg + accent-tinted bg for now-playing row (non-focused)
	NowPlayingRowFavorite   lipgloss.Style // bold favorite fg + accent-tinted bg for now-playing + favorited track
	NowPlayingRowFavArtist  lipgloss.Style // bold favorite fg + accent-tinted bg for now-playing + favorited artist
	NowPlayingRowFavAlbum   lipgloss.Style // bold favorite fg + accent-tinted bg for now-playing + favorited album
	PlayedRow               lipgloss.Style // tertiary foreground for played (already-listened) queue rows
	HasDark                 bool
}

// NewStyles constructs a Styles value for the given terminal background.
func NewStyles(hasDark bool) Styles {
	return NewTheme(hasDark, "")
}

// NewTheme builds a preset with an optional primary color override.
//
// Colors follow four roles, and each role keeps to its own job:
//   - accent (the primary color): what is active — the playing track, its
//     progress and spectrum, the active view, key hints, active modes;
//   - neutral: structure and navigation — text tiers, focus and selection
//     surfaces, the status bar;
//   - favorite: one hue for everything favorited;
//   - feedback (success, warning, error): outcomes and notices only.
func NewTheme(hasDark bool, primary string) Styles {
	ld := lipgloss.LightDark(hasDark)
	c := func(light, dark string) color.Color { return ld(lipgloss.Color(light), lipgloss.Color(dark)) }
	p := newThemeColors(hasDark, primary)

	textPrimary := c(colorTextPrimaryLight, colorTextPrimaryDark)
	textSecondary := c(colorTextSecondaryLight, colorTextSecondaryDark)
	textTertiary := c(colorTextTertiaryLight, colorTextTertiaryDark)
	dim := c(colorDimLight, colorDimDark)
	success := c(colorFgSuccessLight, colorFgSuccessDark)
	warning := c(colorFgWarningLight, colorFgWarningDark)
	errorFg := c(colorFgErrorLight, colorFgErrorDark)
	favorite := favoriteColor(p.primary, c(colorFgFavoriteLight, colorFgFavoriteDark), c(colorFgFavoriteAltLight, colorFgFavoriteAltDark))
	barBg := c(colorSurfaceL0Light, colorSurfaceL0Dark)
	barFg := c(colorFgStatusBarLight, colorFgStatusBarDark)
	notifBg := c(colorSurfaceNotifLight, colorSurfaceNotifDark)

	fg := func(col color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(col) }
	bold := func(col color.Color) lipgloss.Style { return fg(col).Bold(true) }
	onFocus := func(col color.Color) lipgloss.Style { return fg(col).Background(p.focus) }
	onPlaying := func(col color.Color) lipgloss.Style { return bold(col).Background(p.nowPlaying) }
	bar := func(bg color.Color) lipgloss.Style { return lipgloss.NewStyle().Background(bg).Foreground(barFg) }

	return Styles{
		Background: p.background,
		Primary:    p.primary,
		Dim:        fg(dim),
		Accent:     lipgloss.NewStyle().Background(p.focus),
		AccentMark: fg(p.accentText),
		Bold:       lipgloss.NewStyle().Bold(true),
		Heading: bold(p.accentText).
			UnderlineStyle(lipgloss.UnderlineDouble).
			UnderlineColor(p.accentText),
		ActiveTab: lipgloss.NewStyle().Bold(true).
			Background(p.primary).
			Foreground(p.onPrimary),
		Success:             fg(success),
		Warning:             fg(warning),
		Error:               fg(errorFg),
		NowPlaying:          bold(p.accentText),
		NowPlayingFavorite:  bold(favorite),
		NowPlayingFavArtist: bold(favorite),
		NowPlayingFavAlbum:  bold(favorite),
		Favorite:            fg(favorite),
		FavoriteAccent:      onFocus(favorite),
		FavArtist:           fg(favorite),
		FavAlbum:            fg(favorite),
		FavArtistAccent:     onFocus(favorite),
		FavAlbumAccent:      onFocus(favorite),
		ColHead:             bold(textTertiary).UnderlineStyle(lipgloss.UnderlineSingle),
		StatusBar:           bar(barBg),
		// Playing lifts the bar with a trace of the accent; stopped rests on
		// the plain surface.
		StatusBarPlayingBg:      bar(mixColors(barBg, p.primary, 0.06)),
		StatusBarStoppedBg:      bar(barBg),
		StatusNotification:      fg(warning).Background(notifBg),
		StatusNotificationError: fg(errorFg).Background(notifBg),
		ModeRepeat:              bold(p.accentText),
		ModeShuffle:             bold(p.accentText),
		ModeStopAfter:           bold(p.accentText),
		ModeRadio:               bold(p.accentText),
		Badge:                   fg(textTertiary).Background(barBg),
		HelpKey:                 bold(p.accentText),
		ProgressFilled:          fg(p.accentText),
		ProgressEmpty:           fg(dim).Background(notifBg),
		TextPrimary:             fg(textPrimary),
		TextSecondary:           fg(textSecondary),
		TextTertiary:            fg(textTertiary),
		ArtistText:              fg(textSecondary),
		AlbumText:               fg(textTertiary),
		PaletteCategory:         fg(textTertiary).Italic(true),
		NowPlayingTitle:         onPlaying(p.accentText),
		EmptyCard: lipgloss.NewStyle().Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(textTertiary),
		EmptyKey:               bold(p.accentText),
		EmptyHint:              fg(dim),
		Divider:                fg(textTertiary),
		BucketHeader:           bold(textSecondary),
		ZebraBg:                c(colorSurfaceL2Light, colorSurfaceL2Dark),
		SelectionBg:            p.selection,
		NowPlayingBg:           p.nowPlaying,
		NowPlayingRow:          onPlaying(p.accentText),
		NowPlayingRowFavorite:  onPlaying(favorite),
		NowPlayingRowFavArtist: onPlaying(favorite),
		NowPlayingRowFavAlbum:  onPlaying(favorite),
		PlayedRow:              fg(textTertiary),
		HasDark:                hasDark,
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
	hex = strings.TrimSpace(hex)
	if !isHexColor(hex) {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
}

// ColoredDot renders "●" in the given tag color, or plain "●" when color is empty.
func ColoredDot(tagColor string) string {
	return TagColorStyle(tagColor).Render("●")
}

func isHexColor(s string) bool {
	if len(s) != 4 && len(s) != 7 {
		return false
	}
	if s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !isHexDigit(r) {
			return false
		}
	}
	return true
}

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

// WithZebraBg returns style with ZebraBg background when isOdd, else style unchanged.
func WithZebraBg(style lipgloss.Style, zebraBg color.Color, isOdd bool) lipgloss.Style {
	if isOdd {
		return style.Background(zebraBg)
	}
	return style
}

// RenderCountZone renders a right-aligned count string styled as tertiary text
// with optional zebra-stripe background. Returns "" when countStr is empty so
// callers can skip the zone entirely without extra checks.
func RenderCountZone(s Styles, countStr string, isOdd bool) string {
	if countStr == "" {
		return ""
	}
	return WithZebraBg(s.TextTertiary, s.ZebraBg, isOdd).Render(countStr)
}
