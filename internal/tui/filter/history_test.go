package filter

import (
	"fmt"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHistoryRestoresDraftAndEdits(t *testing.T) {
	s := New().WithHistory([]string{"a: Alice", "g: Jazz"})
	s.Text = "unfinished"
	s, _ = s.Activate()
	for _, step := range []struct {
		key  rune
		want string
	}{
		{tea.KeyUp, "g: Jazz"}, {tea.KeyUp, "a: Alice"}, {tea.KeyUp, "a: Alice"},
		{tea.KeyDown, "g: Jazz"}, {tea.KeyDown, "unfinished"}, {tea.KeyDown, "unfinished"},
	} {
		s, _ = s.Update(tea.KeyPressMsg{Code: step.key})
		if s.Text != step.want {
			t.Fatalf("got %q, want %q", s.Text, step.want)
		}
	}
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	s, _ = s.Update(tea.KeyPressMsg{Code: '!', Text: "!"})
	s, _ = s.UpdateMsg(tea.KeyPressMsg{Code: tea.KeyUp})
	s, _ = s.UpdateMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Text != "g: Jazz!" {
		t.Fatalf("edited draft lost: %q", s.Text)
	}
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	s, _ = s.UpdateMsg(tea.PasteMsg{Content: " piano"})
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Text != "g: Jazz piano" {
		t.Fatalf("pasted draft lost: %q", s.Text)
	}
}

func TestHistoryCommitLimitDedupAndValueCopies(t *testing.T) {
	s := New()
	for i := range 25 {
		s.Text = fmt.Sprint(i)
		s = s.Commit()
	}
	if len(s.History()) != 20 || s.History()[0] != "5" {
		t.Fatal(s.History())
	}
	old := s
	s.Text = "10"
	s = s.Commit()
	if s.History()[19] != "10" || old.History()[19] != "24" {
		t.Fatal("history aliases or failed to reorder")
	}
	history := s.History()
	history[0] = "changed"
	if s.History()[0] != "5" {
		t.Fatal("returned history aliases state")
	}
	before := s.History()
	s.Text = "  "
	s = s.Commit()
	s.Text = "cancelled"
	s = s.Deactivate()
	if !reflect.DeepEqual(before, s.History()) {
		t.Fatal("blank/cancelled query recorded")
	}
}
