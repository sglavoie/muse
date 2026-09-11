package autodj

import (
	"sort"
	"strconv"
	"strings"
)

// pickerOption is one selectable row of a filter picker.
type pickerOption struct {
	// key is the value stored in the filter set: a tag id as text, or the
	// genre/artist name itself.
	key string
	// label is what the row reads as.
	label string
	// color is the tag's swatch, empty for genres and artists.
	color string
}

// pickerAction is what a key press asked the picker to do.
type pickerAction int

const (
	pickerNone pickerAction = iota
	// pickerConfirm commits the picker's selection into the draft.
	pickerConfirm
	// pickerCancel discards it.
	pickerCancel
)

// filterPicker is the searchable multi-select list that edits one filter field.
//
// It is deliberately its own thing rather than a reuse of the single-select tag
// filter picker: that one answers "which tag am I filtering by", picks on enter,
// and gives j/k to navigation. Here enter has to mean "I am done choosing", the
// answer is a set rather than a value, and the search box has to accept every
// letter — a genre list where "jazz" cannot be typed is not a search box. So
// navigation lives on the arrows and the ctrl- pairs the command palette
// already uses, and tab toggles the focused row.
type filterPicker struct {
	field   filterField
	exclude bool

	options  []pickerOption
	filtered []int
	cursor   int
	query    string

	// chosen is keyed by option key and deliberately retains keys the catalog
	// no longer offers. A stale tag reference that vanished from the library is
	// still part of the filter in force, and confirming an unrelated edit must
	// not be the thing that quietly drops it.
	chosen map[string]struct{}
}

// newFilterPicker opens a picker on the given options with selected pre-chosen.
func newFilterPicker(field filterField, exclude bool, options []pickerOption, selected []string) filterPicker {
	chosen := make(map[string]struct{}, len(selected))
	for _, key := range selected {
		chosen[key] = struct{}{}
	}
	fp := filterPicker{field: field, exclude: exclude, options: options, chosen: chosen}
	return fp.refilter()
}

// refilter rebuilds the visible rows from the query and clamps the cursor.
func (fp filterPicker) refilter() filterPicker {
	q := strings.ToLower(strings.TrimSpace(fp.query))
	fp.filtered = nil
	for i, opt := range fp.options {
		if q == "" || strings.Contains(strings.ToLower(opt.label), q) {
			fp.filtered = append(fp.filtered, i)
		}
	}
	if fp.cursor >= len(fp.filtered) {
		fp.cursor = max(len(fp.filtered)-1, 0)
	}
	return fp
}

// pickerNavKeys maps every key that only moves the cursor.
var pickerNavKeys = map[string]int{
	"up": -1, "ctrl+k": -1, "ctrl+p": -1,
	"down": 1, "ctrl+j": 1, "ctrl+n": 1,
}

// handleKey applies one key press.
func (fp filterPicker) handleKey(key string) (filterPicker, pickerAction) {
	if delta, ok := pickerNavKeys[key]; ok {
		return fp.moveCursor(delta), pickerNone
	}
	switch key {
	case "esc":
		return fp, pickerCancel
	case "enter":
		return fp, pickerConfirm
	case "tab":
		return fp.toggle(), pickerNone
	case "ctrl+u":
		fp.query = ""
		return fp.refilter(), pickerNone
	case "backspace":
		return fp.deleteRune(), pickerNone
	}
	if runes := []rune(key); len(runes) == 1 {
		fp.query += key
		return fp.refilter(), pickerNone
	}
	return fp, pickerNone
}

func (fp filterPicker) moveCursor(delta int) filterPicker {
	if len(fp.filtered) == 0 {
		fp.cursor = 0
		return fp
	}
	fp.cursor = (fp.cursor + len(fp.filtered) + delta) % len(fp.filtered)
	return fp
}

func (fp filterPicker) deleteRune() filterPicker {
	if fp.query == "" {
		return fp
	}
	runes := []rune(fp.query)
	fp.query = string(runes[:len(runes)-1])
	return fp.refilter()
}

// toggle flips the focused row's membership.
func (fp filterPicker) toggle() filterPicker {
	if fp.cursor >= len(fp.filtered) {
		return fp
	}
	key := fp.options[fp.filtered[fp.cursor]].key
	chosen := make(map[string]struct{}, len(fp.chosen)+1)
	for k := range fp.chosen {
		chosen[k] = struct{}{}
	}
	if _, on := chosen[key]; on {
		delete(chosen, key)
	} else {
		chosen[key] = struct{}{}
	}
	fp.chosen = chosen
	return fp
}

// selection returns the chosen keys: catalog members first, in catalog order,
// then any key the catalog no longer offers, sorted so the result is stable.
func (fp filterPicker) selection() []string {
	keys := make([]string, 0, len(fp.chosen))
	seen := make(map[string]struct{}, len(fp.chosen))
	for _, opt := range fp.options {
		if _, on := fp.chosen[opt.key]; !on {
			continue
		}
		keys = append(keys, opt.key)
		seen[opt.key] = struct{}{}
	}
	var orphans []string
	for key := range fp.chosen {
		if _, known := seen[key]; !known {
			orphans = append(orphans, key)
		}
	}
	sort.Strings(orphans)
	return append(keys, orphans...)
}

// remapTag re-points the picker at a merged tag: the rows it offers become
// options, and a chosen source key becomes the destination.
//
// The query and the cursor are re-evaluated against the new rows rather than
// preserved verbatim, because the list is one row shorter and a cursor left
// where it was would land on a neighbour.
func (fp filterPicker) remapTag(src, dest int64, options []pickerOption) filterPicker {
	srcKey, destKey := strconv.FormatInt(src, 10), strconv.FormatInt(dest, 10)
	fp.options = options
	if _, on := fp.chosen[srcKey]; on {
		chosen := make(map[string]struct{}, len(fp.chosen))
		for k := range fp.chosen {
			if k == srcKey {
				k = destKey
			}
			chosen[k] = struct{}{}
		}
		fp.chosen = chosen
	}
	return fp.refilter()
}

// title is the picker's heading, naming both the field and its polarity.
func (fp filterPicker) title() string {
	verb := "Include"
	if fp.exclude {
		verb = "Exclude"
	}
	return " " + verb + " " + fp.field.label() + " "
}
