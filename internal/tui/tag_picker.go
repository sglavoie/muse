package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// checkState represents the checkbox state for a tag in the picker.
type checkState int

const (
	checkUnchecked checkState = iota // track does not have tag
	checkChecked                     // track has tag
	checkPartial                     // bulk: some tracks have tag, some do not
)

// tagPickerItem is a single row in the TagPicker list.
type tagPickerItem struct {
	tag   db.TagRow
	state checkState
}

// TagPicker is the overlay sub-model for the tag management popup (key 't').
// It supports both single-track and bulk (visual-selection) modes.
type TagPicker struct {
	items      []tagPickerItem // all tags, with current check states
	filtered   []int           // indices into items after filter
	cursor     int
	filter     filter.State
	trackHash  string   // single-track mode (empty when bulkHashes is set)
	bulkHashes []string // bulk mode (nil in single-track mode)
}

// NewTagPicker creates a TagPicker for a single track. It populates the check
// state of every item from trackTagIDs (the set of tag IDs the track already
// carries).
func NewTagPicker(tags []db.TagRow, trackHash string, trackTagIDs map[int64]struct{}) (TagPicker, tea.Cmd) {
	items := buildTagItems(tags, func(id int64) checkState {
		if _, ok := trackTagIDs[id]; ok {
			return checkChecked
		}
		return checkUnchecked
	})
	tp := TagPicker{
		items:     items,
		trackHash: trackHash,
	}
	tp.refilter()
	var cmd tea.Cmd
	tp.filter, cmd = filter.New().Activate()
	return tp, cmd
}

// NewTagPickerBulk creates a TagPicker for bulk-selected tracks. For each tag,
// the state is:
//   - checkChecked   when all hashes carry that tag
//   - checkUnchecked when no  hash  carries that tag
//   - checkPartial   when some hashes carry it and some do not
func NewTagPickerBulk(tags []db.TagRow, hashes []string, trackTags map[string]map[int64]struct{}) (TagPicker, tea.Cmd) {
	total := len(hashes)
	items := buildTagItems(tags, func(id int64) checkState {
		count := 0
		for _, h := range hashes {
			if tt, ok := trackTags[h]; ok {
				if _, has := tt[id]; has {
					count++
				}
			}
		}
		switch count {
		case 0:
			return checkUnchecked
		case total:
			return checkChecked
		default:
			return checkPartial
		}
	})
	tp := TagPicker{
		items:      items,
		bulkHashes: hashes,
	}
	tp.refilter()
	var cmd tea.Cmd
	tp.filter, cmd = filter.New().Activate()
	return tp, cmd
}

func buildTagItems(tags []db.TagRow, stateFn func(int64) checkState) []tagPickerItem {
	items := make([]tagPickerItem, len(tags))
	for i, t := range tags {
		items[i] = tagPickerItem{tag: t, state: stateFn(t.ID)}
	}
	return items
}

// IsBulk reports whether the picker is operating in bulk (multi-track) mode.
func (tp TagPicker) IsBulk() bool { return len(tp.bulkHashes) > 0 }

// Hashes returns the track hashes the picker is operating on (single or bulk).
func (tp TagPicker) Hashes() []string {
	if tp.IsBulk() {
		return tp.bulkHashes
	}
	if tp.trackHash != "" {
		return []string{tp.trackHash}
	}
	return nil
}

// filterText returns the current filter input value.
func (tp TagPicker) filterText() string { return tp.filter.Text }

// focusedOnCreate reports whether the cursor is on the "+ Create" synthetic row
// (only possible when no filtered items match the current filter text).
func (tp TagPicker) focusedOnCreate() bool {
	return len(tp.filtered) == 0 && tp.filterText() != ""
}

// focusedItem returns the tagPickerItem at the cursor, and true when valid.
func (tp TagPicker) focusedItem() (tagPickerItem, bool) {
	if tp.focusedOnCreate() || tp.cursor < 0 || tp.cursor >= len(tp.filtered) {
		return tagPickerItem{}, false
	}
	return tp.items[tp.filtered[tp.cursor]], true
}

// TagPickerToggleMsg is dispatched when the user toggles a tag in the picker.
// The caller (root) applies the change to the DB and the in-memory cache.
type TagPickerToggleMsg struct {
	TagID  int64
	Hashes []string
	Add    bool // true → assign tag; false → remove tag
}

// TagPickerCreateMsg is dispatched when the user requests a new tag be created.
type TagPickerCreateMsg struct {
	Name string
}

// HandleKey processes a key press inside the tag picker.
func (tp TagPicker) HandleKey(key string, msg tea.KeyPressMsg) (TagPicker, tea.Cmd) {
	switch key {
	case "esc":
		return tp, func() tea.Msg { return tagPickerDismissedMsg{} }

	case "up", "ctrl+k", "ctrl+p":
		if tp.cursor > 0 {
			tp.cursor--
		}
		return tp, nil

	case "down", "ctrl+j", "ctrl+n":
		maxCursor := len(tp.filtered) - 1
		if tp.focusedOnCreate() {
			// When a "+Create" row is visible, the create option is at the end.
			maxCursor = 0 // no filtered items; create is the only option
		}
		if !tp.focusedOnCreate() && tp.cursor < maxCursor {
			tp.cursor++
		}
		return tp, nil

	case "enter", "space":
		return tp.handleToggleOrCreate()

	default:
		var cmd tea.Cmd
		tp.filter, cmd = tp.filter.Update(msg)
		tp.refilter()
		return tp, cmd
	}
}

// handleToggleOrCreate executes the action for the currently focused row.
func (tp TagPicker) handleToggleOrCreate() (TagPicker, tea.Cmd) {
	if tp.focusedOnCreate() {
		name := strings.TrimSpace(tp.filterText())
		if name == "" {
			return tp, nil
		}
		return tp, func() tea.Msg { return TagPickerCreateMsg{Name: name} }
	}

	item, ok := tp.focusedItem()
	if !ok {
		return tp, nil
	}

	hashes := tp.Hashes()
	add := item.state != checkChecked // partial and unchecked → add

	return tp, func() tea.Msg {
		return TagPickerToggleMsg{TagID: item.tag.ID, Hashes: hashes, Add: add}
	}
}

// UpdateItemState updates the check state for the given tag ID in-place.
// Called after a successful DB write to keep the picker display in sync.
func (tp TagPicker) UpdateItemState(tagID int64, state checkState) TagPicker {
	for i := range tp.items {
		if tp.items[i].tag.ID == tagID {
			tp.items[i].state = state
			break
		}
	}
	return tp
}

// refilter rebuilds tp.filtered and clamps the cursor.
func (tp *TagPicker) refilter() {
	query := strings.ToLower(tp.filter.Text)
	if query == "" {
		tp.filtered = make([]int, len(tp.items))
		for i := range tp.items {
			tp.filtered[i] = i
		}
	} else {
		tp.filtered = nil
		for i, item := range tp.items {
			if strings.Contains(strings.ToLower(item.tag.Name), query) {
				tp.filtered = append(tp.filtered, i)
			}
		}
	}
	if tp.cursor >= len(tp.filtered) {
		tp.cursor = max(len(tp.filtered)-1, 0)
	}
}

// --- Rendering ---

const (
	tpBorderH  = "─"
	tpBorderV  = "│"
	tpCornerTL = "╭"
	tpCornerTR = "╮"
	tpCornerBL = "╰"
	tpCornerBR = "╯"
	tpCrossL   = "├"
	tpCrossR   = "┤"
)

// Render produces a centered overlay string for the tag picker.
func (tp TagPicker) Render(termWidth, termHeight int, s styles.Styles) string {
	boxLines := tp.buildBoxLines(termWidth, s)

	boxW := ansi.StringWidth(boxLines[0])
	boxH := len(boxLines)

	leftPad := 0
	if termWidth > boxW {
		leftPad = (termWidth - boxW) / 2
	}
	topPad := 0
	if termHeight > boxH {
		topPad = (termHeight - boxH) / 2
	}

	prefix := strings.Repeat(" ", leftPad)
	var b strings.Builder
	for range topPad {
		b.WriteByte('\n')
	}
	for _, line := range boxLines {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

func (tp TagPicker) buildBoxLines(termWidth int, s styles.Styles) []string {
	const modalTitle = " Manage Tags "
	const minInner = 32

	innerWidth := max(minInner, ansi.StringWidth(modalTitle)+2)
	for _, i := range tp.filtered {
		// Use plain name for width calculation (dot is always 1 visible cell).
		label := tp.items[i].checkboxLabel() + " ● " + tp.items[i].tag.Name
		innerWidth = max(innerWidth, ansi.StringWidth(label)+6)
	}
	if maxInner := termWidth - 4; termWidth > 0 && innerWidth > maxInner {
		innerWidth = maxInner
	}
	if innerWidth < 1 {
		innerWidth = 1
	}
	contentWidth := max(innerWidth-2, 0)

	var lines []string

	// Top border.
	lines = append(lines, tpCornerTL+strings.Repeat(tpBorderH, innerWidth)+tpCornerTR)

	// Title row.
	titlePad := max(0, innerWidth-ansi.StringWidth(modalTitle))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, tpBorderV+strings.Repeat(" ", leftTP)+s.Heading.Render(modalTitle)+strings.Repeat(" ", rightTP)+tpBorderV)

	// Separator.
	lines = append(lines, tpCrossL+strings.Repeat(tpBorderH, innerWidth)+tpCrossR)

	// Filter row.
	filterStr := tp.filter.View()
	if !tp.filter.Active {
		filterStr = tp.filter.Text
	}
	filterDisplay := " / " + filterStr
	filterPadded := text.Pad(filterDisplay, contentWidth)
	lines = append(lines, tpBorderV+" "+filterPadded+" "+tpBorderV)

	// Separator before items.
	lines = append(lines, tpCrossL+strings.Repeat(tpBorderH, innerWidth)+tpCrossR)

	// Tag rows.
	if len(tp.filtered) == 0 && tp.filterText() == "" {
		noTags := text.Pad("(no tags — type to create)", contentWidth)
		lines = append(lines, tpBorderV+" "+noTags+" "+tpBorderV)
	} else {
		for rowIdx, itemIdx := range tp.filtered {
			item := tp.items[itemIdx]
			dot := styles.ColoredDot(item.tag.Color)
			label := item.checkboxLabel() + " " + dot + " " + item.tag.Name
			padded := text.Pad(label, contentWidth)
			rowContent := " " + padded + " "
			if rowIdx == tp.cursor {
				lines = append(lines, tpBorderV+s.Accent.Render(rowContent)+tpBorderV)
			} else {
				lines = append(lines, tpBorderV+rowContent+tpBorderV)
			}
		}

		// "+ Create" synthetic row when filter text matches nothing.
		if len(tp.filtered) == 0 && tp.filterText() != "" {
			createLabel := `+ Create "` + tp.filterText() + `"`
			padded := text.Pad(createLabel, contentWidth)
			rowContent := " " + padded + " "
			lines = append(lines, tpBorderV+s.Accent.Render(rowContent)+tpBorderV)
		}
	}

	// Bottom border.
	lines = append(lines, tpCornerBL+strings.Repeat(tpBorderH, innerWidth)+tpCornerBR)

	// Hint line.
	hint := "space toggle · enter select/create · esc close"
	if tp.IsBulk() {
		hint = "~ partial → assign all · " + hint
	}
	lines = append(lines, "  "+s.Dim.Render(hint))

	return lines
}

// checkboxLabel returns the [x], [ ], or [~] prefix for the item.
func (item tagPickerItem) checkboxLabel() string {
	switch item.state {
	case checkChecked:
		return "[x]"
	case checkPartial:
		return "[~]"
	default:
		return "[ ]"
	}
}
