package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestRenderFooterHints_EmptyWhenNoPrimaries(t *testing.T) {
	reg := newRegistry()
	s := styles.NewStyles(true)
	if got := RenderFooterHints(80, contextGlobal, reg, s); got != "" {
		t.Errorf("expected empty hint strip for contextGlobal (no primaries), got %q", got)
	}
}

func TestRenderFooterHints_ZeroWidth(t *testing.T) {
	reg := newRegistry()
	s := styles.NewStyles(true)
	if got := RenderFooterHints(0, contextLibrary, reg, s); got != "" {
		t.Errorf("expected empty hint strip at width 0, got %q", got)
	}
}

func TestRenderFooterHints_IncludesAllPrimariesAtWideWidth(t *testing.T) {
	reg := newRegistry()
	s := styles.NewStyles(true)
	got := RenderFooterHints(200, contextLibrary, reg, s)
	plain := ansi.Strip(got)
	for _, want := range []string{"j/k", "nav", "enter", "enqueue", "N", "play-next", "/", "filter", "v", "select", "?", "help"} {
		if !strings.Contains(plain, want) {
			t.Errorf("RenderFooterHints(wide) = %q; missing %q", plain, want)
		}
	}
}

func TestRenderFooterHints_TruncatesToWidth(t *testing.T) {
	reg := newRegistry()
	s := styles.NewStyles(true)
	got := RenderFooterHints(20, contextLibrary, reg, s)
	if ansi.StringWidth(got) > 20 {
		t.Errorf("RenderFooterHints(20) width = %d; want ≤ 20 (plain=%q)", ansi.StringWidth(got), ansi.Strip(got))
	}
}

func TestRenderFooterHints_ContextSpecificContent(t *testing.T) {
	reg := newRegistry()
	s := styles.NewStyles(true)
	queue := ansi.Strip(RenderFooterHints(200, contextQueue, reg, s))
	if !strings.Contains(queue, "play") {
		t.Errorf("Queue hint strip missing 'play': %q", queue)
	}
	if !strings.Contains(queue, "remove") {
		t.Errorf("Queue hint strip missing 'remove': %q", queue)
	}
	if strings.Contains(queue, "enqueue") {
		t.Errorf("Queue hint strip should not contain Library-only label 'enqueue': %q", queue)
	}
}

func TestPrimariesRegistered_AllPrimaryViews(t *testing.T) {
	reg := newRegistry()
	for _, ctx := range []bindingContext{
		contextLibrary, contextQueue, contextPlaylists,
		contextBrowse, contextArtists, contextAlbums, contextGenres,
		contextFavorites, contextTags, contextMixes,
	} {
		hints := reg.Primaries(ctx)
		if len(hints) < 4 || len(hints) > 6 {
			t.Errorf("context %d: expected 4-6 primary hints, got %d", ctx, len(hints))
		}
	}
}

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
