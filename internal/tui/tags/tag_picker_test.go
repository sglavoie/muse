package tags

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func typeTagName(tp TagPicker, name string) TagPicker {
	for _, ch := range name {
		msg := tea.KeyPressMsg{Code: ch, Text: string(ch)}
		tp, _ = tp.HandleKey(msg.String(), msg)
	}
	return tp
}

func TestTagPickerCreateAlongsideMatches(t *testing.T) {
	tp, _ := NewTagPicker([]db.TagRow{{ID: 1, Name: "jazz fusion"}}, "song", nil)
	tp = typeTagName(tp, "jazz")
	if len(tp.Filtered) != 1 || tp.FocusedOnCreate() {
		t.Fatal("existing match should keep initial focus")
	}
	tp, cmd := tp.HandleKey("enter", tea.KeyPressMsg{})
	if msg, ok := cmd().(TagPickerToggleMsg); !ok || msg.TagID != 1 || !msg.Add {
		t.Fatal("Enter must toggle existing match")
	}
	tp, _ = tp.HandleKey("down", tea.KeyPressMsg{})
	if !tp.FocusedOnCreate() {
		t.Fatal("create row must be reachable alongside matches")
	}
	_, cmd = tp.HandleKey("enter", tea.KeyPressMsg{})
	msg, ok := cmd().(TagPickerCreateMsg)
	if !ok || msg.Name != "jazz" || len(msg.Hashes) != 1 || msg.Hashes[0] != "song" {
		t.Fatalf("create = %#v", msg)
	}
}

func TestTagPickerMultiwordAndExactNames(t *testing.T) {
	tp, _ := NewTagPicker(nil, "song", nil)
	tp = typeTagName(tp, "late night")
	if tp.Filter.Text != "late night" || !tp.FocusedOnCreate() {
		t.Fatalf("multiword input = %q", tp.Filter.Text)
	}
	_, cmd := tp.HandleKey("enter", tea.KeyPressMsg{})
	if msg := cmd().(TagPickerCreateMsg); msg.Name != "late night" {
		t.Fatalf("name = %q", msg.Name)
	}
	for _, query := range []string{"Jazz", " jazz ", "JAZZ"} {
		tp, _ = NewTagPicker([]db.TagRow{{ID: 1, Name: "jazz"}}, "song", nil)
		tp = typeTagName(tp, query)
		tp, _ = tp.HandleKey("down", tea.KeyPressMsg{})
		if tp.canCreate() || tp.Cursor != 0 || len(tp.Filtered) != 1 {
			t.Fatalf("duplicate allowed for %q", query)
		}
	}
	tp, _ = NewTagPicker(nil, "song", nil)
	tp = typeTagName(tp, "   ")
	_, cmd = tp.HandleKey("enter", tea.KeyPressMsg{})
	if cmd != nil || tp.canCreate() {
		t.Fatal("blank input must not create a tag")
	}
}

func TestTagPickerPagingResizeAndFilter(t *testing.T) {
	items := make([]db.TagRow, 40)
	for i := range items {
		items[i] = db.TagRow{ID: int64(i + 1), Name: fmt.Sprintf("Tag %02d", i)}
	}
	tp, _ := NewTagPicker(items, "song", nil)
	tp.SetHeight(12)
	tp, _ = tp.HandleKey("pgdown", tea.KeyPressMsg{})
	if tp.Cursor != 4 {
		t.Fatalf("page down cursor = %d", tp.Cursor)
	}
	tp, _ = tp.HandleKey("pgup", tea.KeyPressMsg{})
	if tp.Cursor != 0 {
		t.Fatalf("page up cursor = %d", tp.Cursor)
	}
	for range 20 {
		tp, _ = tp.HandleKey("pgdown", tea.KeyPressMsg{})
	}
	if tp.Cursor != 39 {
		t.Fatalf("last cursor = %d", tp.Cursor)
	}
	for _, height := range []int{25, 12, 18} {
		out := ansi.Strip(tp.Render(60, height, styles.NewStyles(true)))
		if !strings.Contains(out, "Tag 39") || !strings.Contains(out, "/40") {
			t.Fatalf("focused row/position missing:\n%s", out)
		}
		if len(strings.Split(out, "\n")) > height {
			t.Fatalf("height %d overflow:\n%s", height, out)
		}
		for _, line := range strings.Split(out, "\n") {
			if ansi.StringWidth(line) > 60 {
				t.Fatalf("wide line: %q", line)
			}
		}
	}
	tp = typeTagName(tp, "Tag 00")
	if tp.Cursor != 0 || tp.rowCount() != 1 {
		t.Fatal("filter must clamp to remaining match")
	}
	tp = tp.RemoveItem(1)
	if !tp.FocusedOnCreate() {
		t.Fatal("removing final match should expose create row")
	}
}
