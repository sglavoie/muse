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
	if got := RenderFooterHints(80, contextGlobal, HintModeDefault, reg, s); got != "" {
		t.Errorf("expected empty hint strip for contextGlobal (no primaries), got %q", got)
	}
}

func TestRenderFooterHints_ZeroWidth(t *testing.T) {
	reg := newRegistry()
	s := styles.NewStyles(true)
	if got := RenderFooterHints(0, contextLibrary, HintModeDefault, reg, s); got != "" {
		t.Errorf("expected empty hint strip at width 0, got %q", got)
	}
}

func TestRenderFooterHints_IncludesAllPrimariesAtWideWidth(t *testing.T) {
	reg := newRegistry()
	s := styles.NewStyles(true)
	got := RenderFooterHints(200, contextLibrary, HintModeDefault, reg, s)
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
	got := RenderFooterHints(20, contextLibrary, HintModeDefault, reg, s)
	if ansi.StringWidth(got) > 20 {
		t.Errorf("RenderFooterHints(20) width = %d; want ≤ 20 (plain=%q)", ansi.StringWidth(got), ansi.Strip(got))
	}
}

func TestRenderFooterHints_KeyLabelAsymmetry(t *testing.T) {
	// Pair tokens must be styled per-token: keys through HelpKey, labels
	// through TextSecondary, separators through Dim — not the prior
	// blanket-Dim rendering.
	reg := newRegistry()
	s := styles.NewStyles(true)
	got := RenderFooterHints(200, contextLibrary, HintModeDefault, reg, s)

	// Sanity: plain-text content unchanged.
	plain := ansi.Strip(got)
	for _, want := range []string{"j/k nav", "enter enqueue", "?", "help"} {
		if !strings.Contains(plain, want) {
			t.Errorf("plain hint strip missing %q: %q", want, plain)
		}
	}

	// Each key should appear wrapped in the HelpKey escape sequence; each
	// label should appear wrapped in the TextSecondary escape sequence; the
	// separator should appear wrapped in the Dim escape sequence.
	keyOpen := strings.Split(s.HelpKey.Render("X"), "X")[0]
	labelOpen := strings.Split(s.TextSecondary.Render("X"), "X")[0]
	dimOpen := strings.Split(s.Dim.Render("X"), "X")[0]

	if !strings.Contains(got, keyOpen+"j/k") {
		t.Errorf("expected HelpKey-wrapped 'j/k' in output; got %q", got)
	}
	if !strings.Contains(got, labelOpen+"nav") {
		t.Errorf("expected TextSecondary-wrapped 'nav' in output; got %q", got)
	}
	if !strings.Contains(got, dimOpen+footerHintSep) {
		t.Errorf("expected Dim-wrapped separator in output; got %q", got)
	}
}

func TestRenderFooterHints_TruncationFallbackKeyPops(t *testing.T) {
	// When the available width forces a partial render of the first pair,
	// a key that survives truncation in full should still be rendered in
	// HelpKey, with the elided remainder in Dim.
	reg := newRegistry()
	s := styles.NewStyles(true)
	// Width that fits the two-space indent, "j/k", a space, and an ellipsis
	// but not the full label "nav".
	got := RenderFooterHints(7, contextLibrary, HintModeDefault, reg, s)
	if got == "" {
		t.Fatalf("expected non-empty truncated output")
	}
	keyOpen := strings.Split(s.HelpKey.Render("X"), "X")[0]
	if !strings.Contains(got, keyOpen+"j/k") {
		t.Errorf("expected HelpKey-wrapped 'j/k' in truncated output; got %q", got)
	}
	if !strings.Contains(ansi.Strip(got), "…") {
		t.Errorf("expected ellipsis in truncated output plain=%q", ansi.Strip(got))
	}
}

func TestRenderFooterHints_ContextSpecificContent(t *testing.T) {
	reg := newRegistry()
	s := styles.NewStyles(true)
	queue := ansi.Strip(RenderFooterHints(200, contextQueue, HintModeDefault, reg, s))
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

func TestRenderFooterHints_SelectionMode(t *testing.T) {
	reg := newRegistry()
	s := styles.NewStyles(true)
	// Library selection set surfaces bulk-action labels distinct from default.
	got := ansi.Strip(RenderFooterHints(200, contextLibrary, HintModeSelection, reg, s))
	for _, want := range []string{"enter", "enqueue", "a", "bulk action", "t", "tag", "X", "clear"} {
		if !strings.Contains(got, want) {
			t.Errorf("Library selection hint strip missing %q: %q", want, got)
		}
	}
	// "play-next" is a default-mode label and should not bleed into selection mode.
	if strings.Contains(got, "play-next") {
		t.Errorf("Library selection hint strip should not surface default-only 'play-next': %q", got)
	}
	// Queue selection set keeps its view-local actions (remove, save).
	q := ansi.Strip(RenderFooterHints(200, contextQueue, HintModeSelection, reg, s))
	for _, want := range []string{"play", "remove", "save", "clear"} {
		if !strings.Contains(q, want) {
			t.Errorf("Queue selection hint strip missing %q: %q", want, q)
		}
	}
}

func TestRenderFooterHints_VisualMode(t *testing.T) {
	reg := newRegistry()
	s := styles.NewStyles(true)
	for _, ctx := range []bindingContext{contextLibrary, contextQueue, contextArtists, contextFavorites} {
		got := ansi.Strip(RenderFooterHints(200, ctx, HintModeVisual, reg, s))
		for _, want := range []string{"j/k", "extend", "v", "exit", "esc", "cancel", "X", "clear"} {
			if !strings.Contains(got, want) {
				t.Errorf("ctx %d visual hint strip missing %q: %q", ctx, want, got)
			}
		}
	}
}

func TestRenderFooterHints_VisualFallsBackToDefaultWhenUnregistered(t *testing.T) {
	reg := newRegistry()
	s := styles.NewStyles(true)
	// contextMixes has no visual-mode hints registered — should fall back to default.
	got := ansi.Strip(RenderFooterHints(200, contextMixes, HintModeVisual, reg, s))
	def := ansi.Strip(RenderFooterHints(200, contextMixes, HintModeDefault, reg, s))
	if got != def {
		t.Errorf("expected visual mode to fall back to default for contextMixes; got=%q want=%q", got, def)
	}
}

func TestRenderFooterHints_DefaultModeMatchesCurrent(t *testing.T) {
	// Regression guard: the default-mode hint set must remain identical to
	// what existed before HintMode was introduced. If a new default-mode
	// hint is added we want the change to be deliberate.
	reg := newRegistry()
	s := styles.NewStyles(true)
	got := ansi.Strip(RenderFooterHints(200, contextLibrary, HintModeDefault, reg, s))
	want := []string{"j/k nav", "enter enqueue", "N play-next", "/ filter", "v select", "? help"}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("default Library hint strip missing %q: %q", w, got)
		}
	}
	// Make sure selection/visual labels did not leak into default.
	for _, bad := range []string{"bulk action", "extend", "cancel"} {
		if strings.Contains(got, bad) {
			t.Errorf("default Library hint strip should not contain %q: %q", bad, got)
		}
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

// Regression: a primaryHint with an empty Label is rendered as the key alone,
// so its width budget must also exclude the trailing space. Otherwise a
// label-less hint would be dropped from the strip one column too early.
func TestRenderFooterHints_EmptyLabelDoesNotOverReserveWidth(t *testing.T) {
	reg := Registry{
		bindings:      map[registryKey]action{},
		primaries:     map[primaryHintKey][]primaryHint{},
		domainReasons: map[registryKey]string{},
	}
	reg.primaries[primaryHintKey{ctx: contextLibrary, mode: HintModeDefault}] = []primaryHint{
		{Key: "ab", Label: ""}, // 2 columns rendered, no trailing space
	}
	s := styles.NewStyles(true)
	// Width budget: 2 (leading) + 2 (key) == 4. With the previous accounting
	// (treating the pair as "ab " = 3 cols) this would fall into the
	// truncation fallback even though the key fits exactly.
	got := RenderFooterHints(4, contextLibrary, HintModeDefault, reg, s)
	if got == "" {
		t.Fatalf("expected non-empty output at width 4 for a 2-col key")
	}
	plain := ansi.Strip(got)
	if !strings.Contains(plain, "ab") {
		t.Errorf("expected plain output to contain 'ab', got %q", plain)
	}
	if strings.Contains(plain, "…") {
		t.Errorf("expected no truncation for a key that fits in full, got %q", plain)
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
