package metadata

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestSingleTrackResetOnlyRestoresFocusedField(t *testing.T) {
	for field := fieldTitle; field < fieldCount; field++ {
		ed := NewEditor("h", "Song", "Artist", "", "Jazz", 183.25)
		other := (field + 1) % fieldCount
		ed.focused = other
		ed.Fields[other].SetValue("keep this edit")
		ed.refreshFocusedEdit()
		ed.focused = field
		ed.Fields[field].SetValue("accidental edit")
		ed.refreshFocusedEdit()
		ed, action, cmd := ed.HandleKey(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
		if action != actionNone || cmd != nil || ed.focused != field {
			t.Fatal("reset should stay in the focused field")
		}
		if ed.Fields[field].Value() != ed.initialValues[field] || ed.Dirty[field] {
			t.Fatalf("field %d was not restored", field)
		}
		if ed.Fields[other].Value() != "keep this edit" || !ed.Dirty[other] {
			t.Fatal("reset changed another field")
		}
		if field == FieldDuration && ed.DurationChanged() {
			t.Fatal("reset duration would overwrite fractional seconds on save")
		}
		out := ansi.Strip(ed.Render(60, 15, newTestStyles()))
		if !strings.Contains(out, "ctrl+r reset field") || !strings.Contains(out, "1 field changed") {
			t.Fatalf("missing reset hint or incorrect change count:\n%s", out)
		}
		if len(strings.Split(strings.TrimRight(out, "\n"), "\n")) > 15 {
			t.Fatalf("reset hint overflows minimum terminal height:\n%s", out)
		}
	}
}
