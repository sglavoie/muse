package albums

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Level identifies the active navigation level inside the albums panel.
type Level int

const (
	AlbumLevel Level = iota // showing the flat list of all albums
	TrackLevel              // showing tracks for a selected album
)

// Compile-time assertion: Styles must remain a value type on View.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the albums panel with 2-level drill-down:
// Albums -> Tracks. All fields are set by the caller before each Render call.
type View struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Albums []string
	Tracks []library.Track

	// AlbumCounts maps each album name to its track count for the right-aligned
	// count column at AlbumLevel. Optional — when nil the count column is omitted.
	AlbumCounts map[string]int

	SelectedAlbum string

	// Saved cursor/scroll for album level (restored on Back).
	AlbumCursor       int
	AlbumScrollOffset int

	Width  int
	Height int
	Styles styles.Styles

	FilterActive bool
	FilterText   string
	FilterView   string // pre-rendered textinput view for active filter display
	FilterHint   string // shown instead of cursor when filter is active and empty

	StripViewLabel     string // label rendered in the state strip; defaults to "Albums"
	StripSortLabel     string // sort field label for the strip; empty when sort inactive
	StripSortIndicator string // sort direction arrow for the strip; "▲" or "▼"

	FavoriteHashes  map[string]struct{}
	FavoriteArtists map[string]struct{}
	FavoriteAlbums  map[string]struct{}
	SelectedHashes  map[string]struct{}
	NowPlayingHash  string // hash of the currently playing track; empty when nothing is playing
	NowPlayingIcon  string // marker glyph (e.g. "▶", "⏸", "■"); empty falls back to "♪"
}

// EffectiveAlbums returns the album list filtered by FilterText.
func (v View) EffectiveAlbums() []string { return arttui.FilterStrings(v.Albums, v.FilterText) }

// EffectiveTracks returns the track list filtered by FilterText.
func (v View) EffectiveTracks() []library.Track {
	return arttui.FilterTracks(v.Tracks, v.FilterText)
}

// visibleRows returns the maximum number of rows that fit on screen.
// Reserve accounts for the state strip (which now carries the breadcrumb
// in place of a separate heading) and (at the track level) the column
// header:
//   - AlbumLevel: 1 (strip)
//   - TrackLevel: 2 (strip + column header)
func (v View) visibleRows() int {
	reserve := 1
	if v.Level == TrackLevel {
		reserve = 2
	}
	return layout.VisibleRows(v.Height, reserve)
}

// CursorUp returns a copy of v with the cursor moved up by one row.
func (v View) CursorUp() View {
	if v.Cursor > 0 {
		v.Cursor--
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	}
	return v
}

// CursorDown returns a copy of v with the cursor moved down by one row.
func (v View) CursorDown() View {
	limit := v.itemCount() - 1
	if v.Cursor < limit {
		v.Cursor++
	}
	maxRows := v.visibleRows()
	if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// HalfPageDown moves the cursor half a visible page down.
func (v View) HalfPageDown() View { return v.moveCursor(v.visibleRows() / 2) }

// HalfPageUp moves the cursor half a visible page up.
func (v View) HalfPageUp() View { return v.moveCursor(-v.visibleRows() / 2) }

// FullPageDown moves the cursor a full visible page down.
func (v View) FullPageDown() View { return v.moveCursor(v.visibleRows()) }

// FullPageUp moves the cursor a full visible page up.
func (v View) FullPageUp() View { return v.moveCursor(-v.visibleRows()) }

// GoToTop moves the cursor to the first row.
func (v View) GoToTop() View {
	v.Cursor = 0
	v.ScrollOffset = 0
	return v
}

// GoToBottom moves the cursor to the last row.
func (v View) GoToBottom() View {
	n := v.itemCount()
	if n > 0 {
		v.Cursor = n - 1
	}
	maxRows := v.visibleRows()
	if v.Cursor >= maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// itemCount returns the number of items in the current level.
func (v View) itemCount() int {
	switch v.Level {
	case TrackLevel:
		return len(v.EffectiveTracks())
	default:
		return len(v.EffectiveAlbums())
	}
}

// moveCursor shifts the cursor by delta rows, clamping to [0, itemCount-1].
func (v View) moveCursor(delta int) View {
	n := v.itemCount()
	if n == 0 {
		return v
	}
	v.Cursor += delta
	v.Cursor = max(0, min(v.Cursor, n-1))
	maxRows := v.visibleRows()
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	} else if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// OpenTracks returns a copy of v transitioned to TrackLevel with tracks loaded.
// The current album cursor and scroll offset are saved for restoration by Back.
func (v View) OpenTracks(tracks []library.Track) View {
	v.AlbumCursor = v.Cursor
	v.AlbumScrollOffset = v.ScrollOffset
	v.Tracks = tracks
	v.Level = TrackLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	return v
}

// Back returns a copy of v navigated one level up. At TrackLevel it returns to
// AlbumLevel. At AlbumLevel it is a no-op. The restored cursor/scroll are
// clamped against the parent list size so that mutations (e.g. a deleted
// album) during the drill-down do not leave the cursor pointing past the new
// end.
func (v View) Back() View {
	if v.Level == TrackLevel {
		v.Level = AlbumLevel
		v.Cursor = v.AlbumCursor
		v.ScrollOffset = v.AlbumScrollOffset
		v.Tracks = nil
		v.FilterActive = false
		v.FilterText = ""
		v = v.clampRestoredCursor()
	}
	return v
}

// clampRestoredCursor clamps Cursor to [0, itemCount-1] and adjusts
// ScrollOffset to keep the cursor visible. Used after Back() restores a saved
// cursor that may now point past the (possibly shrunk) parent list.
func (v View) clampRestoredCursor() View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	if v.Cursor < 0 {
		v.Cursor = 0
	}
	if v.Cursor > n-1 {
		v.Cursor = n - 1
	}
	if v.ScrollOffset > v.Cursor {
		v.ScrollOffset = v.Cursor
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	return v
}

// ---------- Data derivation (pure functions) ----------

// DeriveAllAlbums extracts unique album names from a track slice, sorted
// alphabetically. This collects albums across all artists.
func DeriveAllAlbums(tracks []library.Track) []string {
	seen := make(map[string]struct{})
	var albums []string
	for _, t := range tracks {
		album := t.Album
		if album == "" {
			album = arttui.UnknownAlbum
		}
		if _, ok := seen[album]; !ok {
			seen[album] = struct{}{}
			albums = append(albums, album)
		}
	}
	sort.Slice(albums, func(i, j int) bool {
		if albums[i] == arttui.UnknownAlbum {
			return false
		}
		if albums[j] == arttui.UnknownAlbum {
			return true
		}
		return albums[i] < albums[j]
	})
	return albums
}

// DeriveAllAlbumCounts maps each album name to its track count across the
// entire catalog. Tracks with an empty Album field count under the
// arttui.UnknownAlbum display sentinel.
func DeriveAllAlbumCounts(tracks []library.Track) map[string]int {
	if len(tracks) == 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, t := range tracks {
		album := t.Album
		if album == "" {
			album = arttui.UnknownAlbum
		}
		counts[album]++
	}
	return counts
}

// DeriveAlbumTracks returns all tracks matching the given album name,
// regardless of artist.
func DeriveAlbumTracks(tracks []library.Track, album string) []library.Track {
	album = arttui.RawName(album)
	var result []library.Track
	for _, t := range tracks {
		if t.Album == album {
			result = append(result, t)
		}
	}
	return result
}

// ---------- Rendering ----------

// Render returns the string representation of the albums panel.
func (v View) Render() string {
	switch v.Level {
	case TrackLevel:
		return v.renderTracks()
	default:
		return v.renderAlbums()
	}
}

func (v View) renderAlbums() string {
	items := v.EffectiveAlbums()
	if len(v.Albums) == 0 {
		return v.Styles.Dim.Render("No albums yet — add music to your library")
	}
	stripLine := v.renderStateStrip(len(v.Albums), len(items))
	if len(items) == 0 {
		return joinStrip(stripLine, "(no matches)")
	}

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, name := range items[start:end] {
		absIdx := start + i
		_, isFav := v.FavoriteAlbums[name]
		isOdd := absIdx%2 == 1
		labelW, countStr := rowLabelWidthAndCount(v.Width, v.AlbumCounts, name)
		label := text.FavPrefix(name, labelW, isFav)
		switch {
		case absIdx == v.Cursor && isFav:
			a := v.Styles.FavAlbumAccent
			lines[i] = a.Render(label) + a.Render(countStr)
		case absIdx == v.Cursor:
			a := v.Styles.Accent
			lines[i] = a.Render(label) + a.Render(countStr)
		case isFav:
			labelStyled := styles.WithZebraBg(v.Styles.FavAlbum, v.Styles.ZebraBg, isOdd).Render(label)
			lines[i] = labelStyled + renderCountZone(v.Styles, countStr, isOdd)
		case isOdd:
			labelStyled := lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(label)
			lines[i] = labelStyled + renderCountZone(v.Styles, countStr, true)
		default:
			lines[i] = label + renderCountZone(v.Styles, countStr, false)
		}
	}
	return joinStrip(stripLine, strings.Join(lines, "\n"))
}

// rowLabelWidthAndCount computes the label-zone width and pre-formatted count
// suffix for a Browse list row. See artists.rowLabelWithCount for shared
// semantics — duplicated here to keep the albums package free of cross-pkg
// rendering dependencies.
func rowLabelWidthAndCount(width int, counts map[string]int, name string) (int, string) {
	if counts == nil || width < text.MinLabelWithCount+1+text.CountColWidth {
		return width, ""
	}
	labelW := width - 1 - text.CountColWidth
	return labelW, text.FormatCountSuffix(counts[name])
}

// renderCountZone wraps the count suffix in TextTertiary with optional zebra
// background. Returns "" when countStr is empty.
func renderCountZone(s styles.Styles, countStr string, isOdd bool) string {
	if countStr == "" {
		return ""
	}
	return styles.WithZebraBg(s.TextTertiary, s.ZebraBg, isOdd).Render(countStr)
}

func (v View) renderTracks() string {
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return v.Styles.Dim.Render("No tracks found for this album")
	}
	stripLine := v.renderStateStrip(len(v.Tracks), len(items))
	if len(items) == 0 {
		return joinStrip(stripLine, "(no matches)")
	}

	cols := ComputeTrack(v.Width)

	header := strings.Repeat(" ", cols.SelectionMarker) + v.Styles.ColHead.Render(
		fmt.Sprintf("%-*s %-*s %-*s %*s",
			cols.Position, "#", cols.Title, "Title", cols.Artist, "Artist", cols.Duration, "Time"))

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	showDisc := false
	for _, t := range items[start:end] {
		if t.DiscNumber > 1 {
			showDisc = true
			break
		}
	}

	lines := make([]string, end-start)
	for i, t := range items[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.Cursor
		isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash
		_, isSelected := v.SelectedHashes[t.Hash]

		markerStr := text.RowMarker(isSelected, isFocused, isNowPlaying, v.NowPlayingIcon)

		var rawPos string
		if showDisc {
			rawPos = text.FormatTrackPos(t.TrackNumber, t.DiscNumber, absIdx+1)
		} else {
			rawPos = text.FormatTrackPosSimple(t.TrackNumber, absIdx+1)
		}
		pos := fmt.Sprintf("%-*s", cols.Position, rawPos)
		_, isTrackFav := v.FavoriteHashes[t.Hash]
		_, isArtistFav := v.FavoriteArtists[t.Artist]
		titleStr := text.FavPrefix(t.Title, cols.Title, isTrackFav)
		artistStr := text.FavPrefix(t.Artist, cols.Artist, isArtistFav)
		dur := text.FormatDurationSecs(t.Duration)
		dur = fmt.Sprintf("%*s", cols.Duration, dur)
		switch {
		case isFocused:
			a := v.Styles.Accent
			sep := a.Render(" ")
			titleStr = styles.AccentFavRender(a, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			artistStr = styles.AccentFavRender(a, v.Styles.FavArtistAccent, isArtistFav, artistStr)
			lines[i] = a.Render(markerStr+pos) + sep + titleStr + sep + artistStr + sep + a.Render(dur)
		case isNowPlaying:
			np := v.Styles.NowPlayingRow
			sep := np.Render(" ")
			titleStr = v.Styles.NowPlayingTitle.Render(titleStr)
			if isArtistFav {
				artistStr = v.Styles.NowPlayingRowFavArtist.Render(artistStr)
			} else {
				artistStr = v.Styles.TextPrimary.Background(v.Styles.NowPlayingBg).Render(artistStr)
			}
			lines[i] = np.Render(markerStr+pos) + sep + titleStr + sep + artistStr + sep + np.Render(dur)
		default:
			isOdd := absIdx%2 == 1
			rowBg := v.Styles.ZebraBg
			useBg := isOdd
			if isSelected {
				rowBg = v.Styles.SelectionBg
				useBg = true
			}
			if isTrackFav {
				titleStr = styles.WithZebraBg(v.Styles.Favorite, rowBg, useBg).Render(titleStr)
			}
			if isArtistFav {
				artistStr = styles.WithZebraBg(v.Styles.FavArtist, rowBg, useBg).Render(artistStr)
			}
			if useBg {
				zb := lipgloss.NewStyle().Background(rowBg)
				if !isTrackFav {
					titleStr = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(titleStr)
				}
				if !isArtistFav {
					artistStr = styles.WithZebraBg(v.Styles.TextPrimary, rowBg, true).Render(artistStr)
				}
				var markerPart string
				if isSelected {
					markerPart = v.Styles.AccentMark.Background(rowBg).Render("●") + zb.Render(" ")
				} else {
					markerPart = zb.Render(markerStr)
				}
				posPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(pos)
				durPart := styles.WithZebraBg(v.Styles.TextTertiary, rowBg, true).Render(dur)
				sep := zb.Render(" ")
				lines[i] = markerPart + posPart + sep + titleStr + sep + artistStr + sep + durPart
			} else {
				if !isTrackFav {
					titleStr = v.Styles.TextPrimary.Render(titleStr)
				}
				if !isArtistFav {
					artistStr = v.Styles.TextPrimary.Render(artistStr)
				}
				lines[i] = markerStr + v.Styles.TextTertiary.Render(pos) + " " + titleStr + " " + artistStr + " " + v.Styles.TextTertiary.Render(dur)
			}
		}
	}
	rest := header + "\n" + strings.Join(lines, "\n")
	return joinStrip(stripLine, rest)
}

// renderStateStrip builds the unified single-line state strip carrying the
// breadcrumb, sort, filter, counts, and an "esc ←" hint at the track
// level. total is the full item count before filtering; filtered is the
// count after filtering (equal to total when no filter is active).
func (v View) renderStateStrip(total, filtered int) string {
	root := v.StripViewLabel
	if root == "" {
		root = "Albums"
	}
	breadcrumb := []string{root}
	if v.Level == TrackLevel && v.SelectedAlbum != "" {
		breadcrumb = append(breadcrumb, v.SelectedAlbum)
	}
	hint := ""
	if len(breadcrumb) > 1 {
		hint = "esc ←"
	}
	sortChunk := strip.SortChunk{Label: v.StripSortLabel, Indicator: v.StripSortIndicator}
	f := strip.FilterChunk{Active: v.FilterActive, Text: v.FilterText}
	if v.FilterActive {
		if v.FilterText == "" && v.FilterHint != "" {
			f.Hint = v.Styles.TextSecondary.Render(v.FilterHint)
		} else {
			f.InputView = v.FilterView
		}
	}
	counts := strip.CountsChunk{
		Filtered:  filtered,
		Total:     total,
		Filtering: v.FilterText != "",
	}
	var sel *strip.SelectionChunk
	if n := len(v.SelectedHashes); n > 0 {
		sel = &strip.SelectionChunk{Count: n}
	}
	return strip.Render(v.Width, breadcrumb, sortChunk, f, counts, sel, hint, v.Styles)
}

// joinStrip joins a top-level strip line with the rendered body, skipping the
// strip entirely when it is empty (zero-width fallback).
func joinStrip(stripLine, body string) string {
	if stripLine == "" {
		return body
	}
	return stripLine + "\n" + body
}
