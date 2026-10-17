package tags

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// CheckState represents the checkbox state for a tag in the picker.
type CheckState int

const (
	CheckUnchecked CheckState = iota // track does not have tag
	CheckChecked                     // track has tag
	CheckPartial                     // bulk: some tracks have tag, some do not
)

// TagPickerItem is a single row in the TagPicker list.
type TagPickerItem struct {
	tag   db.TagRow
	state CheckState
}

// TagPicker is the overlay sub-model for the tag management popup (key 't').
// It supports both single-track and bulk (visual-selection) modes.
type TagPicker struct {
	Items      []TagPickerItem // all tags, with current check states
	Filtered   []int           // indices into Items after filter
	Cursor     int
	height     int
	Filter     filter.State
	trackHash  string   // single-track mode (empty when bulkHashes is set)
	bulkHashes []string // bulk mode (nil in single-track mode)
}

// NewTagPicker creates a TagPicker for a single track. It populates the check
// state of every item from trackTagIDs (the set of tag IDs the track already
// carries).
func NewTagPicker(tags []db.TagRow, trackHash string, trackTagIDs map[int64]struct{}) (TagPicker, tea.Cmd) {
	items := buildTagItems(tags, func(id int64) CheckState {
		if _, ok := trackTagIDs[id]; ok {
			return CheckChecked
		}
		return CheckUnchecked
	})
	tp := TagPicker{
		Items:     items,
		trackHash: trackHash,
	}
	tp.Refilter()
	var cmd tea.Cmd
	tp.Filter, cmd = filter.New().Activate()
	return tp, cmd
}

// NewTagPickerBulk creates a TagPicker for bulk-selected tracks. For each tag,
// the state is:
//   - checkChecked   when all hashes carry that tag
//   - checkUnchecked when no  hash  carries that tag
//   - checkPartial   when some hashes carry it and some do not
func NewTagPickerBulk(tags []db.TagRow, hashes []string, trackTags map[string]map[int64]struct{}) (TagPicker, tea.Cmd) {
	total := len(hashes)
	items := buildTagItems(tags, func(id int64) CheckState {
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
			return CheckUnchecked
		case total:
			return CheckChecked
		default:
			return CheckPartial
		}
	})
	tp := TagPicker{
		Items:      items,
		bulkHashes: hashes,
	}
	tp.Refilter()
	var cmd tea.Cmd
	tp.Filter, cmd = filter.New().Activate()
	return tp, cmd
}

func buildTagItems(tags []db.TagRow, stateFn func(int64) CheckState) []TagPickerItem {
	items := make([]TagPickerItem, len(tags))
	for i, t := range tags {
		items[i] = TagPickerItem{tag: t, state: stateFn(t.ID)}
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
func (tp TagPicker) filterText() string { return tp.Filter.Text }

// canCreate allows a new name alongside substring matches, but not duplicates.
func (tp TagPicker) canCreate() bool {
	name := strings.TrimSpace(tp.filterText())
	if name == "" {
		return false
	}
	for _, item := range tp.Items {
		if strings.EqualFold(item.tag.Name, name) {
			return false
		}
	}
	return true
}

func (tp TagPicker) rowCount() int {
	count := len(tp.Filtered)
	if tp.canCreate() {
		count++
	}
	return count
}

// FocusedOnCreate reports whether the cursor is on the final synthetic row.
func (tp TagPicker) FocusedOnCreate() bool {
	return tp.canCreate() && tp.Cursor == len(tp.Filtered)
}

// SetHeight supplies the overlay height for paging, including its borders and hint.
func (tp *TagPicker) SetHeight(height int) { tp.height = height }

func tagPickerPageSize(height int) int {
	// Six fixed box rows, the footer hint, and its trailing newline.
	return max(1, height-8)
}

func (tp TagPicker) moveCursor(delta int) TagPicker {
	tp.Cursor = max(0, min(tp.Cursor+delta, tp.rowCount()-1))
	return tp
}

// focusedItem returns the TagPickerItem at the cursor, and true when valid.
func (tp TagPicker) focusedItem() (TagPickerItem, bool) {
	if tp.FocusedOnCreate() || tp.Cursor < 0 || tp.Cursor >= len(tp.Filtered) {
		return TagPickerItem{}, false
	}
	return tp.Items[tp.Filtered[tp.Cursor]], true
}

// TagPickerToggleMsg is dispatched when the user toggles a tag in the picker.
// The caller (root) applies the change to the DB and the in-memory cache.
type TagPickerToggleMsg struct {
	TagID  int64
	Hashes []string
	Add    bool // true → assign tag; false → remove tag
}

// TagPickerCreateMsg is dispatched when the user requests a new tag be created.
// Hashes carries the picker's current target tracks so that the create handler
// can assign the new tag to them atomically.
type TagPickerCreateMsg struct {
	Name   string
	Hashes []string
}

// TagPickerDismissedMsg is dispatched when the tag picker overlay is closed
// without making any changes.
type TagPickerDismissedMsg struct{}

// HandleKey processes a key press inside the tag picker.
func (tp TagPicker) HandleKey(key string, msg tea.KeyPressMsg) (TagPicker, tea.Cmd) {
	switch key {
	case "esc":
		return tp, func() tea.Msg { return TagPickerDismissedMsg{} }

	case "up", "ctrl+k", "ctrl+p":
		return tp.moveCursor(-1), nil
	case "down", "ctrl+j", "ctrl+n":
		return tp.moveCursor(1), nil
	case "pgup":
		return tp.moveCursor(-tagPickerPageSize(tp.height)), nil
	case "pgdown":
		return tp.moveCursor(tagPickerPageSize(tp.height)), nil
	case "enter":
		return tp.handleToggleOrCreate()

	default:
		var cmd tea.Cmd
		tp.Filter, cmd = tp.Filter.Update(msg)
		tp.Refilter()
		return tp, cmd
	}
}

// handleToggleOrCreate executes the action for the currently focused row.
func (tp TagPicker) handleToggleOrCreate() (TagPicker, tea.Cmd) {
	if tp.FocusedOnCreate() {
		name := strings.TrimSpace(tp.filterText())
		if name == "" {
			return tp, nil
		}
		hashes := tp.Hashes()
		return tp, func() tea.Msg { return TagPickerCreateMsg{Name: name, Hashes: hashes} }
	}

	item, ok := tp.focusedItem()
	if !ok {
		return tp, nil
	}

	hashes := tp.Hashes()
	add := item.state != CheckChecked // partial and unchecked → add

	return tp, func() tea.Msg {
		return TagPickerToggleMsg{TagID: item.tag.ID, Hashes: hashes, Add: add}
	}
}

// UpdateItemState updates the check state for the given tag ID in-place.
// Called after a successful DB write to keep the picker display in sync.
func (tp TagPicker) UpdateItemState(tagID int64, state CheckState) TagPicker {
	for i := range tp.Items {
		if tp.Items[i].tag.ID == tagID {
			tp.Items[i].state = state
			break
		}
	}
	return tp
}

// RemoveItem removes the tag with the given ID from the picker's item list,
// rebuilds the filtered indices, and clamps the cursor. Used when a tag is
// auto-deleted while the picker is open (its row would otherwise still be
// shown and toggling it on would attempt to re-add a deleted tag).
func (tp TagPicker) RemoveItem(tagID int64) TagPicker {
	out := make([]TagPickerItem, 0, len(tp.Items))
	for _, it := range tp.Items {
		if it.tag.ID != tagID {
			out = append(out, it)
		}
	}
	tp.Items = out
	tp.Refilter()
	return tp
}

// Refilter rebuilds tp.Filtered and clamps the cursor.
func (tp *TagPicker) Refilter() {
	query := strings.ToLower(strings.TrimSpace(tp.Filter.Text))
	if query == "" {
		tp.Filtered = make([]int, len(tp.Items))
		for i := range tp.Items {
			tp.Filtered[i] = i
		}
	} else {
		tp.Filtered = nil
		for i, item := range tp.Items {
			if strings.Contains(strings.ToLower(item.tag.Name), query) {
				tp.Filtered = append(tp.Filtered, i)
			}
		}
	}
	tp.Cursor = max(0, min(tp.Cursor, tp.rowCount()-1))
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
	boxLines := tp.buildBoxLines(termWidth, termHeight, s)
	hint := "enter toggle/create · pgup/pgdown scroll · esc close"
	if tp.IsBulk() {
		hint = "enter toggle/create · pgup/dn · esc close · ~ assign all"
	}
	return overlay.RenderCenteredLines(boxLines, s.Dim.Render(hint), termWidth, termHeight)
}

// tagPickerInnerWidth sizes the modal to its widest tag row, then clamps it to
// what the terminal can show.
func (tp TagPicker) tagPickerInnerWidth(termWidth, minInner int) int {
	innerWidth := minInner
	if tp.canCreate() {
		innerWidth = max(innerWidth, ansi.StringWidth(strings.TrimSpace(tp.filterText()))+13)
	}
	for _, i := range tp.Filtered {
		// Use plain name for width calculation (dot is always 1 visible cell).
		label := tp.Items[i].checkboxLabel() + " ● " + tp.Items[i].tag.Name
		innerWidth = max(innerWidth, ansi.StringWidth(label)+6)
	}
	if maxInner := termWidth - 4; termWidth > 0 && innerWidth > maxInner {
		innerWidth = maxInner
	}
	return max(innerWidth, 1)
}

func (tp TagPicker) buildBoxLines(termWidth, termHeight int, s styles.Styles) []string {
	start, end := tp.visibleWindow(termHeight)
	modalTitle := " Manage Tags "
	if tp.rowCount() > end-start {
		modalTitle = fmt.Sprintf(" Manage Tags · %d–%d/%d ", start+1, end, tp.rowCount())
	}
	const minInner = 32

	innerWidth := tp.tagPickerInnerWidth(termWidth, max(minInner, ansi.StringWidth(modalTitle)+2))
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
	filterStr := tp.Filter.View()
	if !tp.Filter.Active {
		filterStr = tp.Filter.Text
	}
	filterDisplay := " / " + filterStr
	filterPadded := text.Pad(filterDisplay, contentWidth)
	lines = append(lines, tpBorderV+" "+filterPadded+" "+tpBorderV)

	// Separator before items.
	lines = append(lines, tpCrossL+strings.Repeat(tpBorderH, innerWidth)+tpCrossR)

	// Tag rows.
	lines = append(lines, tp.tagRowLines(contentWidth, start, end, s)...)

	// Bottom border.
	lines = append(lines, tpCornerBL+strings.Repeat(tpBorderH, innerWidth)+tpCornerBR)

	return lines
}

// visibleWindow keeps the focused row visible, including after a resize.
func (tp TagPicker) visibleWindow(height int) (int, int) {
	count := tp.rowCount()
	size := tagPickerPageSize(height)
	start := max(0, min(tp.Cursor-size+1, count-size))
	return start, min(count, start+size)
}

// tagRowLines renders only the visible tags and optional create row.
func (tp TagPicker) tagRowLines(contentWidth, start, end int, s styles.Styles) []string {
	if tp.rowCount() == 0 {
		return []string{tagPickerRow(text.Pad("(no tags — type to create)", contentWidth), false, s)}
	}
	lines := make([]string, 0, end-start)
	for row := start; row < end; row++ {
		label := `+ Create "` + strings.TrimSpace(tp.filterText()) + `"`
		if row < len(tp.Filtered) {
			item := tp.Items[tp.Filtered[row]]
			label = item.checkboxLabel() + " " + styles.ColoredDot(item.tag.Color) + " " + item.tag.Name
		}
		lines = append(lines, tagPickerRow(text.Pad(label, contentWidth), row == tp.Cursor, s))
	}
	return lines
}

// tagPickerRow frames one padded row between the modal's vertical borders,
// accenting it when focused.
func tagPickerRow(padded string, accented bool, s styles.Styles) string {
	rowContent := " " + text.FocusMarker(accented) + text.Pad(strings.TrimRight(padded, " "), max(ansi.StringWidth(padded)-2, 0)) + " "
	if accented {
		return tpBorderV + s.Accent.Render(rowContent) + tpBorderV
	}
	return tpBorderV + rowContent + tpBorderV
}

// checkboxLabel returns the [x], [ ], or [~] prefix for the item.
func (item TagPickerItem) checkboxLabel() string {
	switch item.state {
	case CheckChecked:
		return "[x]"
	case CheckPartial:
		return "[~]"
	default:
		return "[ ]"
	}
}
