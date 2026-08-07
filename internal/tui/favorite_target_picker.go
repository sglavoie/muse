package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
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

type favoritePickerOrigin struct {
	view          viewID
	favLevel      int
	favCategory   int
	drilldownName string
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

// favoriteTargetPicker selects whether F applies to the focused tracks'
// individual titles, artists, or albums.
type favoriteTargetPicker struct {
	tracks []library.Track
	rows   []favoriteTargetRow
	cursor int
	bulk   bool
	origin favoritePickerOrigin
}

type favoriteTargetPickedMsg struct {
	typ    string
	values []string
	add    bool
	origin favoritePickerOrigin
}

type favoriteTargetDismissedMsg struct{}

func newFavoriteTargetPicker(
	tracks []library.Track,
	favoriteHashes, favoriteArtists, favoriteAlbums map[string]struct{},
	bulk bool,
	origin favoritePickerOrigin,
) favoriteTargetPicker {
	tracks = append([]library.Track(nil), tracks...)
	p := favoriteTargetPicker{tracks: tracks, bulk: bulk, origin: origin}
	p.rows = []favoriteTargetRow{
		buildFavoriteTargetRow(favoriteTargetTitle, tracks, favoriteHashes),
		buildFavoriteTargetRow(favoriteTargetArtist, tracks, favoriteArtists),
		buildFavoriteTargetRow(favoriteTargetAlbum, tracks, favoriteAlbums),
	}
	p.cursor = p.firstEnabledRow()
	return p
}

func buildFavoriteTargetRow(target favoriteTarget, tracks []library.Track, favorites map[string]struct{}) favoriteTargetRow {
	row := favoriteTargetRow{enabled: true}
	var rawValues []string
	switch target {
	case favoriteTargetTitle:
		row.typ, row.label, row.hotkey = "track", "Title", "t"
		for _, track := range tracks {
			rawValues = append(rawValues, track.Hash)
		}
		if len(tracks) == 1 {
			row.displayValue = strings.TrimSpace(tracks[0].Title)
			if row.displayValue == "" {
				row.displayValue = "(unknown title)"
			}
		}
	case favoriteTargetArtist:
		row.typ, row.label, row.hotkey = "artist", "Artist", "r"
		for _, track := range tracks {
			if !usableFavoriteMetadata(track.Artist, "artist") {
				row.enabled, row.unavailable = false, "missing metadata"
			}
			rawValues = append(rawValues, track.Artist)
		}
		if len(tracks) == 1 {
			row.displayValue = tracks[0].Artist
		}
	case favoriteTargetAlbum:
		row.typ, row.label, row.hotkey = "album", "Album", "l"
		for _, track := range tracks {
			if !usableFavoriteMetadata(track.Album, "album") {
				row.enabled, row.unavailable = false, "missing metadata"
			}
			rawValues = append(rawValues, track.Album)
		}
		if len(tracks) == 1 {
			row.displayValue = tracks[0].Album
		}
	}

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

func (p favoriteTargetPicker) firstEnabledRow() int {
	for i, row := range p.rows {
		if row.enabled {
			return i
		}
	}
	return 0
}

func (p favoriteTargetPicker) move(delta int) favoriteTargetPicker {
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

func (p favoriteTargetPicker) pick(row favoriteTargetRow) tea.Cmd {
	if !row.enabled || len(row.changeValues) == 0 {
		return nil
	}
	msg := favoriteTargetPickedMsg{
		typ:    row.typ,
		values: append([]string(nil), row.changeValues...),
		add:    row.add,
		origin: p.origin,
	}
	return func() tea.Msg { return msg }
}

func (p favoriteTargetPicker) HandleKey(key string) (favoriteTargetPicker, tea.Cmd) {
	switch key {
	case "esc", "F":
		return p, func() tea.Msg { return favoriteTargetDismissedMsg{} }
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

func (p favoriteTargetPicker) Render(termWidth, termHeight int, s styles.Styles) string {
	rows := make([]OverlayRow, 0, len(p.rows))
	for i, row := range p.rows {
		line := p.rowText(row)
		if !row.enabled {
			line = s.Dim.Render(line)
		}
		rows = append(rows, OverlayRow{Text: line, Styled: row.enabled && i == p.cursor})
	}
	return OverlayBox{
		Title:         " Favorite target ",
		Rows:          rows,
		Hint:          "j/k navigate · enter choose · t/r/l select · F/esc cancel",
		MaxInnerWidth: max(termWidth-4, OverlayHPadding),
	}.Render(termWidth, termHeight, s)
}

func (p favoriteTargetPicker) rowText(row favoriteTargetRow) string {
	prefix := "[" + row.hotkey + "] " + row.label + " · "
	if !row.enabled {
		return prefix + "unavailable · " + row.unavailable
	}
	action := "Add"
	if !row.add {
		action = "Remove"
	}
	state := row.state.String()
	if !p.bulk && len(p.tracks) == 1 {
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
