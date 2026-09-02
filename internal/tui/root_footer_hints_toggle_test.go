package tui

import "testing"

func TestToggleFooterHintsFlipsState(t *testing.T) {
	m := Model{showFooterHints: true, termWidth: 80, termHeight: 24}
	m2, _ := m.handleToggleFooterHintsMsg()
	if m2.showFooterHints {
		t.Errorf("after toggle, showFooterHints should be false")
	}
	if m2.cfg.ShowFooterHints {
		t.Errorf("after toggle, cfg.ShowFooterHints should be false")
	}
	m3, _ := m2.handleToggleFooterHintsMsg()
	if !m3.showFooterHints {
		t.Errorf("after second toggle, showFooterHints should be true again")
	}
}
