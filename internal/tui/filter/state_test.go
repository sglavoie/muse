package filter

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestActivate(t *testing.T) {
	s, _ := New().Activate()
	if !s.Active {
		t.Error("expected Active=true")
	}
}

func TestActivatePreservesText(t *testing.T) {
	base := New()
	base.Text = "abc"
	s, _ := base.Activate()
	if s.Text != "abc" {
		t.Errorf("expected Text='abc', got %q", s.Text)
	}
}

func TestCommit(t *testing.T) {
	base := New()
	base.Active = true
	base.Text = "foo"
	base.input.SetValue("foo")
	s := base.Commit()
	if s.Active {
		t.Error("expected Active=false")
	}
	if s.Text != "foo" {
		t.Errorf("expected Text='foo', got %q", s.Text)
	}
}

func TestDeactivate(t *testing.T) {
	base := New()
	base.Active = true
	base.Text = "bar"
	s := base.Deactivate()
	if s.Active {
		t.Error("expected Active=false")
	}
	if s.Text != "" {
		t.Errorf("expected Text='', got %q", s.Text)
	}
}

func TestUpdate(t *testing.T) {
	s, _ := New().Activate()
	s, _ = s.Update(tea.KeyPressMsg{Text: "a"})
	if s.Text != "a" {
		t.Errorf("expected Text='a', got %q", s.Text)
	}
	s, _ = s.Update(tea.KeyPressMsg{Text: "b"})
	if s.Text != "ab" {
		t.Errorf("expected Text='ab', got %q", s.Text)
	}
}

func TestUpdateInactiveNoOp(t *testing.T) {
	s := New()
	s, _ = s.Update(tea.KeyPressMsg{Text: "x"})
	if s.Text != "" {
		t.Errorf("expected Text='' when inactive, got %q", s.Text)
	}
}

func TestUpdateBackspace(t *testing.T) {
	s, _ := New().Activate()
	s, _ = s.Update(tea.KeyPressMsg{Text: "a"})
	s, _ = s.Update(tea.KeyPressMsg{Text: "b"})
	s, _ = s.Update(tea.KeyPressMsg{Text: "c"})
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if s.Text != "ab" {
		t.Errorf("expected Text='ab' after backspace, got %q", s.Text)
	}
}

func TestView(t *testing.T) {
	s, _ := New().Activate()
	// View() must not panic and must return a string.
	_ = s.View()
}
