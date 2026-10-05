package favorites

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
)

type favoriteTarget int

const (
	favoriteTargetTitle favoriteTarget = iota
	favoriteTargetArtist
	favoriteTargetAlbum
)

type favoriteTargetState int

const (
	favoriteStateNone favoriteTargetState = iota
	favoriteStateMixed
	favoriteStateAll
)

type PickerOrigin struct {
	// View is the numeric viewID the picker was opened from; the caller owns
	// the enum, so it travels as a plain int to keep this package free of it.
	View          int
	FavLevel      int
	FavCategory   int
	DrilldownName string
}

type favoriteTargetRow struct {
	typ          string
	label        string
	hotkey       string
	values       []string
	changeValues []string
	displayValue string
	state        favoriteTargetState
	add          bool
	enabled      bool
	unavailable  string
}

// TargetPicker selects whether F applies to the focused tracks'
// individual titles, artists, or albums.
type TargetPicker struct {
	Tracks []library.Track
	rows   []favoriteTargetRow
	cursor int
	bulk   bool
	origin PickerOrigin
}

type TargetPickedMsg struct {
	Typ    string
	Values []string
	Add    bool
	Origin PickerOrigin
}

type TargetDismissedMsg struct{}

func NewTargetPicker(
	tracks []library.Track,
	favoriteHashes, favoriteArtists, favoriteAlbums map[string]struct{},
	bulk bool,
	origin PickerOrigin,
) TargetPicker {
	tracks = append([]library.Track(nil), tracks...)
	p := TargetPicker{Tracks: tracks, bulk: bulk, origin: origin}
	p.rows = []favoriteTargetRow{
		buildFavoriteTargetRow(favoriteTargetTitle, tracks, favoriteHashes),
		buildFavoriteTargetRow(favoriteTargetArtist, tracks, favoriteArtists),
		buildFavoriteTargetRow(favoriteTargetAlbum, tracks, favoriteAlbums),
	}
	p.cursor = p.firstEnabledRow()
	return p
}

func buildFavoriteTargetRow(target favoriteTarget, tracks []library.Track, favorites map[string]struct{}) favoriteTargetRow {
	row, rawValues := describeFavoriteTarget(target, tracks)
	row.values = uniqueNonEmptyStrings(rawValues)
	if len(row.values) == 0 {
		row.enabled, row.unavailable = false, "missing identity"
	}
	if favorites == nil {
		row.enabled, row.unavailable = false, "favorite state unavailable"
	}
	if !row.enabled {
		return row
	}
	return applyFavoriteState(row, favorites)
}

// describeFavoriteTarget labels the row for target and collects the raw identity
// value each track contributes to it. The row comes back disabled when a track
// carries metadata unusable as a favorite key.
func describeFavoriteTarget(target favoriteTarget, tracks []library.Track) (favoriteTargetRow, []string) {
	row := favoriteTargetRow{enabled: true}
	switch target {
	case favoriteTargetTitle:
		row.typ, row.label, row.hotkey = "track", "Title", "t"
		var rawValues []string
		for _, track := range tracks {
			rawValues = append(rawValues, track.Hash)
		}
		if len(tracks) == 1 {
			row.displayValue = strings.TrimSpace(tracks[0].Title)
			if row.displayValue == "" {
				row.displayValue = "(unknown title)"
			}
		}
		return row, rawValues
	case favoriteTargetArtist:
		row.typ, row.label, row.hotkey = "artist", "Artist", "r"
		rawValues := collectMetadataValues(&row, tracks, func(t library.Track) string { return t.Artist })
		return row, rawValues
	case favoriteTargetAlbum:
		row.typ, row.label, row.hotkey = "album", "Album", "l"
		rawValues := collectMetadataValues(&row, tracks, library.AlbumKey)
		return row, rawValues
	}
	return row, nil
}

// collectMetadataValues reads one metadata field off every track, disabling row
// as soon as a value is a placeholder that cannot identify a favorite. row.typ
// selects which placeholders count, so it must already be set.
func collectMetadataValues(row *favoriteTargetRow, tracks []library.Track, field func(library.Track) string) []string {
	var rawValues []string
	for _, track := range tracks {
		value := field(track)
		if !usableFavoriteMetadata(value, row.typ) {
			row.enabled, row.unavailable = false, "missing metadata"
		}
		rawValues = append(rawValues, value)
	}
	if len(tracks) == 1 {
		row.displayValue = field(tracks[0])
		if row.typ == "album" {
			row.displayValue = library.AlbumLabel(row.displayValue)
		}
	}
	return rawValues
}

// applyFavoriteState classifies how many of the row's values are already
// favorited and records the values a toggle would change.
func applyFavoriteState(row favoriteTargetRow, favorites map[string]struct{}) favoriteTargetRow {
	favoriteCount := 0
	for _, value := range row.values {
		if _, ok := favorites[value]; ok {
			favoriteCount++
		} else {
			row.changeValues = append(row.changeValues, value)
		}
	}
	switch {
	case favoriteCount == 0:
		row.state = favoriteStateNone
	case favoriteCount == len(row.values):
		row.state = favoriteStateAll
	default:
		row.state = favoriteStateMixed
	}
	row.add = row.state != favoriteStateAll
	if !row.add {
		row.changeValues = append([]string(nil), row.values...)
	}
	return row
}

func usableFavoriteMetadata(value, typ string) bool {
	if typ == "album" {
		value = library.AlbumTitle(value)
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	switch typ {
	case "artist":
		return normalized != "unknown artist" && normalized != "(unknown artist)"
	case "album":
		return normalized != "unknown album" && normalized != "(unknown album)"
	default:
		return true
	}
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (p TargetPicker) firstEnabledRow() int {
	for i, row := range p.rows {
		if row.enabled {
			return i
		}
	}
	return 0
}

func (p TargetPicker) move(delta int) TargetPicker {
	if len(p.rows) == 0 {
		return p
	}
	candidate := p.cursor
	for range len(p.rows) {
		candidate += delta
		if candidate < 0 || candidate >= len(p.rows) {
			return p
		}
		if p.rows[candidate].enabled {
			p.cursor = candidate
			return p
		}
	}
	return p
}

func (p TargetPicker) pick(row favoriteTargetRow) tea.Cmd {
	if !row.enabled || len(row.changeValues) == 0 {
		return nil
	}
	msg := TargetPickedMsg{
		Typ:    row.typ,
		Values: append([]string(nil), row.changeValues...),
		Add:    row.add,
		Origin: p.origin,
	}
	return func() tea.Msg { return msg }
}

func (p TargetPicker) HandleKey(key string) (TargetPicker, tea.Cmd) {
	switch key {
	case "esc", "F":
		return p, func() tea.Msg { return TargetDismissedMsg{} }
	case "up", "k":
		return p.move(-1), nil
	case "down", "j":
		return p.move(1), nil
	case "enter":
		if p.cursor >= 0 && p.cursor < len(p.rows) {
			return p, p.pick(p.rows[p.cursor])
		}
	case "t", "r", "l":
		for _, row := range p.rows {
			if row.hotkey == key {
				return p, p.pick(row)
			}
		}
	}
	return p, nil
}

func (p TargetPicker) Render(termWidth, termHeight int, s styles.Styles) string {
	rows := make([]overlay.Row, 0, len(p.rows))
	for i, row := range p.rows {
		line := p.rowText(row)
		if !row.enabled {
			line = s.Dim.Render(line)
		}
		rows = append(rows, overlay.Row{Text: line, Styled: row.enabled && i == p.cursor})
	}
	return overlay.Box{
		Title:         " Favorite target ",
		Rows:          rows,
		Hint:          "j/k navigate · enter choose · t/r/l select · F/esc cancel",
		MaxInnerWidth: max(termWidth-4, overlay.HPadding),
	}.Render(termWidth, termHeight, s)
}

func (p TargetPicker) rowText(row favoriteTargetRow) string {
	prefix := "[" + row.hotkey + "] " + row.label + " · "
	if !row.enabled {
		return prefix + "unavailable · " + row.unavailable
	}
	action := "Add"
	if !row.add {
		action = "Remove"
	}
	state := row.state.String()
	if !p.bulk && len(p.Tracks) == 1 {
		return fmt.Sprintf("%s%s · %s · %s", prefix, row.displayValue, state, action)
	}
	unit := row.typ
	if row.typ == "track" {
		unit = "track"
	}
	if len(row.values) != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%s%d %s · %s · %s", prefix, len(row.values), unit, state, action)
}

func (s favoriteTargetState) String() string {
	switch s {
	case favoriteStateAll:
		return "all"
	case favoriteStateMixed:
		return "mixed"
	default:
		return "none"
	}
}
