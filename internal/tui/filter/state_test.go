package filter

import "testing"

func TestActivate(t *testing.T) {
	s := State{}.Activate()
	if !s.Active {
		t.Error("expected Active=true")
	}
}

func TestActivatePreservesText(t *testing.T) {
	s := State{Text: "abc"}.Activate()
	if s.Text != "abc" {
		t.Errorf("expected Text='abc', got %q", s.Text)
	}
}

func TestCommit(t *testing.T) {
	s := State{Active: true, Text: "foo"}.Commit()
	if s.Active {
		t.Error("expected Active=false")
	}
	if s.Text != "foo" {
		t.Errorf("expected Text='foo', got %q", s.Text)
	}
}

func TestDeactivate(t *testing.T) {
	s := State{Active: true, Text: "bar"}.Deactivate()
	if s.Active {
		t.Error("expected Active=false")
	}
	if s.Text != "" {
		t.Errorf("expected Text='', got %q", s.Text)
	}
}

func TestAppendChar(t *testing.T) {
	s := State{Active: true}
	s, changed := s.AppendChar('a')
	if !changed {
		t.Error("expected changed=true")
	}
	if s.Text != "a" {
		t.Errorf("expected Text='a', got %q", s.Text)
	}
	s, changed = s.AppendChar('b')
	if !changed {
		t.Error("expected changed=true")
	}
	if s.Text != "ab" {
		t.Errorf("expected Text='ab', got %q", s.Text)
	}
}

func TestAppendCharInactive(t *testing.T) {
	s := State{}
	s, changed := s.AppendChar('x')
	if changed {
		t.Error("expected changed=false when inactive")
	}
	if s.Text != "" {
		t.Errorf("expected Text='', got %q", s.Text)
	}
}

func TestBackspace(t *testing.T) {
	s := State{Active: true, Text: "abc"}
	s, changed := s.Backspace()
	if !changed {
		t.Error("expected changed=true")
	}
	if s.Text != "ab" {
		t.Errorf("expected Text='ab', got %q", s.Text)
	}
}

func TestBackspaceEmpty(t *testing.T) {
	s := State{Active: true}
	s, changed := s.Backspace() //nolint:staticcheck // SA4006: s is intentionally reassigned to test changed flag
	if changed {
		t.Error("expected changed=false on empty text")
	}
}

func TestBackspaceInactive(t *testing.T) {
	s := State{Text: "abc"}
	s, changed := s.Backspace()
	if changed {
		t.Error("expected changed=false when inactive")
	}
	if s.Text != "abc" {
		t.Errorf("expected Text='abc', got %q", s.Text)
	}
}

func TestBackspaceUnicode(t *testing.T) {
	s := State{Active: true, Text: "héllo"}
	s, _ = s.Backspace()
	if s.Text != "héll" {
		t.Errorf("expected Text='héll', got %q", s.Text)
	}
}
