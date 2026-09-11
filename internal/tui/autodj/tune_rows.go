package autodj

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// filterField names one of the three dimensions an inclusion or exclusion set
// is built from.
type filterField int

const (
	// fieldTags selects by tag id, which is stable across a rename.
	fieldTags filterField = iota
	// fieldGenres selects by genre name.
	fieldGenres
	// fieldArtists selects by artist name.
	fieldArtists
)

// label returns the human-facing name of the field.
func (f filterField) label() string {
	switch f {
	case fieldTags:
		return "tags"
	case fieldGenres:
		return "genres"
	case fieldArtists:
		return "artists"
	}
	return ""
}

// row identifies one focusable line of the panel. The order is the reading
// order: what the DJ plays, then what it may draw from, then the commit.
type row int

const (
	rowPreset row = iota
	rowDiscovery
	rowAnchor
	rowIncludeTags
	rowIncludeGenres
	rowIncludeArtists
	rowExcludeTags
	rowExcludeGenres
	rowExcludeArtists
	rowCommit
	rowCount
)

// filterRow describes the filter field a row edits.
type filterRow struct {
	exclude bool
	field   filterField
}

// filterRows maps every filter row to the field it edits. Rows absent from the
// table are not filter rows.
var filterRows = map[row]filterRow{
	rowIncludeTags:    {exclude: false, field: fieldTags},
	rowIncludeGenres:  {exclude: false, field: fieldGenres},
	rowIncludeArtists: {exclude: false, field: fieldArtists},
	rowExcludeTags:    {exclude: true, field: fieldTags},
	rowExcludeGenres:  {exclude: true, field: fieldGenres},
	rowExcludeArtists: {exclude: true, field: fieldArtists},
}

// rowLabels is the left column of the panel.
var rowLabels = [rowCount]string{
	rowPreset:         "Preset",
	rowDiscovery:      "Discovery",
	rowAnchor:         "Anchor",
	rowIncludeTags:    "Include tags",
	rowIncludeGenres:  "Include genres",
	rowIncludeArtists: "Include artists",
	rowExcludeTags:    "Exclude tags",
	rowExcludeGenres:  "Exclude genres",
	rowExcludeArtists: "Exclude artists",
	rowCommit:         "",
}

// focusedRow returns the row the cursor is on.
func (p TunePanel) focusedRow() row { return row(p.cursor) }

// rowValue renders the right column of one row.
func (p TunePanel) rowValue(r row) string {
	switch r {
	case rowPreset:
		return "‹ " + p.draft.Preset().Label() + " ›"
	case rowDiscovery:
		return fmt.Sprintf("‹ %d%% ›", p.draft.DiscoveryPercent)
	case rowAnchor:
		return "‹ " + p.draft.Anchor.Label() + " ›"
	case rowCommit:
		return p.commitButton()
	}
	if fr, ok := filterRows[r]; ok {
		return p.summarizeField(fr)
	}
	return ""
}

// commitButton renders the primary action, which reports its own progress: a
// commit in flight is the one state where pressing it again would do nothing.
func (p TunePanel) commitButton() string {
	if p.busy {
		return "[ " + p.CommitLabel() + "ing… ]"
	}
	return "[ " + p.CommitLabel() + " ]"
}

// summarizeField renders one filter field as a count and the values behind it.
func (p TunePanel) summarizeField(fr filterRow) string {
	labels := p.fieldLabels(fr)
	if len(labels) == 0 {
		return "any"
	}
	return fmt.Sprintf("%d · %s", len(labels), strings.Join(labels, ", "))
}

// fieldLabels returns the display labels of one field's current selection.
//
// A tag id the catalog cannot resolve is shown as the id itself rather than
// omitted: an unresolved reference still narrows or widens what the DJ may play,
// so hiding it would leave the user reading a filter set that is not the one in
// force.
func (p TunePanel) fieldLabels(fr filterRow) []string {
	set := p.filterSet(fr.exclude)
	if fr.field != fieldTags {
		return fieldValues(set, fr.field)
	}
	names := p.tagNames()
	labels := make([]string, 0, len(set.TagIDs))
	for _, id := range set.TagIDs {
		if name, ok := names[id]; ok {
			labels = append(labels, name)
			continue
		}
		labels = append(labels, "#"+strconv.FormatInt(id, 10)+" (missing)")
	}
	return labels
}

// tagNames indexes every tag the panel can put a name to. It reads the whole
// census rather than the picker's list, so a tag holding no playable tracks is
// still shown by name in a filter that references it.
func (p TunePanel) tagNames() map[int64]string {
	names := make(map[int64]string, len(p.catalog.KnownTags)+len(p.catalog.Tags))
	for _, t := range p.catalog.KnownTags {
		names[t.ID] = t.Name
	}
	for _, t := range p.catalog.Tags {
		names[t.ID] = t.Name
	}
	return names
}

// fieldOptions builds the picker rows for one field, in the order the picker
// shows them.
func (p TunePanel) fieldOptions(f filterField) []pickerOption {
	switch f {
	case fieldTags:
		return p.tagOptions()
	case fieldGenres:
		return stringOptions(p.catalog.Genres)
	case fieldArtists:
		return stringOptions(p.catalog.Artists)
	}
	return nil
}

// tagOptions builds the tag picker's rows: the catalog's suggestion list, then
// any existing tag the draft already references that the list leaves out.
//
// The suggestion list is filtered by track count upstream, which is right for
// choosing a new filter and wrong for one already chosen. A row the picker never
// draws is a row the user cannot toggle off, so an exclusion of a tag that went
// empty would become permanent — and unlike a deleted reference, it has no
// repair gesture behind it, because it is not broken.
func (p TunePanel) tagOptions() []pickerOption {
	opts := make([]pickerOption, 0, len(p.catalog.Tags))
	offered := make(map[int64]struct{}, len(p.catalog.Tags))
	for _, t := range p.catalog.Tags {
		opts = append(opts, tagOption(t))
		offered[t.ID] = struct{}{}
	}
	referenced := p.draftTagIDs()
	for _, t := range p.catalog.KnownTags {
		if _, drawn := offered[t.ID]; drawn {
			continue
		}
		if _, wanted := referenced[t.ID]; !wanted {
			continue
		}
		opts = append(opts, tagOption(t))
	}
	return opts
}

func tagOption(t db.TagRow) pickerOption {
	return pickerOption{key: strconv.FormatInt(t.ID, 10), label: t.Name, color: t.Color}
}

func stringOptions(values []string) []pickerOption {
	sorted := append([]string(nil), values...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i]) < strings.ToLower(sorted[j])
	})
	opts := make([]pickerOption, len(sorted))
	for i, v := range sorted {
		opts[i] = pickerOption{key: v, label: v}
	}
	return opts
}

// presetCycle returns the preset delta steps away from the draft's current one.
//
// Custom is not a member of the cycle — it is what a hand-tuned percentage
// reports, not something the user picks — so stepping off it enters the list
// from whichever end the direction implies.
func presetCycle(current radio.Preset, delta int) radio.Preset {
	presets := radio.Presets()
	idx := -1
	for i, preset := range presets {
		if preset == current {
			idx = i
			break
		}
	}
	if idx < 0 {
		if delta < 0 {
			return presets[len(presets)-1]
		}
		return presets[0]
	}
	return presets[(idx+len(presets)+delta)%len(presets)]
}
