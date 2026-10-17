package keys

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestRenderFooterHints_EmptyWhenNoPrimaries(t *testing.T) {
	reg := NewRegistry()
	s := styles.NewStyles(true)
	if got := RenderFooterHints(80, ContextGlobal, HintModeDefault, reg, s); got != "" {
		t.Errorf("expected empty hint strip for contextGlobal (no primaries), got %q", got)
	}
}

func TestFilterFooterPrioritizesApplyAndClear(t *testing.T) {
	reg := NewRegistry()
	s := styles.NewStyles(true)
	for _, width := range []int{26, 60, 100} {
		got := ansi.Strip(RenderFooterHints(width, ContextLibrary, HintModeFilter, reg, s))
		if !strings.Contains(got, "enter apply · esc clear") || ansi.StringWidth(got) > width {
			t.Fatalf("width %d: %q", width, got)
		}
		if width == 26 && strings.Contains(got, "history") {
			t.Fatalf("history should yield to apply and clear: %q", got)
		}
	}
}

func TestRenderFooterHints_ZeroWidth(t *testing.T) {
	reg := NewRegistry()
	s := styles.NewStyles(true)
	if got := RenderFooterHints(0, ContextLibrary, HintModeDefault, reg, s); got != "" {
		t.Errorf("expected empty hint strip at width 0, got %q", got)
	}
}

func TestRenderFooterHints_IncludesAllPrimariesAtWideWidth(t *testing.T) {
	reg := NewRegistry()
	s := styles.NewStyles(true)
	got := RenderFooterHints(200, ContextLibrary, HintModeDefault, reg, s)
	plain := ansi.Strip(got)
	for _, want := range []string{"j/k", "nav", "enter", "enqueue", ":", "commands", "/", "filter", "v", "select", "?", "help"} {
		if !strings.Contains(plain, want) {
			t.Errorf("RenderFooterHints(wide) = %q; missing %q", plain, want)
		}
	}
}

func TestRenderFooterHints_TruncatesToWidth(t *testing.T) {
	reg := NewRegistry()
	s := styles.NewStyles(true)
	got := RenderFooterHints(20, ContextLibrary, HintModeDefault, reg, s)
	if ansi.StringWidth(got) > 20 {
		t.Errorf("RenderFooterHints(20) width = %d; want ≤ 20 (plain=%q)", ansi.StringWidth(got), ansi.Strip(got))
	}
}

// TestRenderFooterHints_KeepsHelpWhenNarrow guards that dropping pairs to fit
// a narrow width drops the ones before the trailing "? help" pair rather than
// help itself, which is where every dropped hint can still be found.
func TestRenderFooterHints_KeepsHelpWhenNarrow(t *testing.T) {
	reg := NewRegistry()
	s := styles.NewStyles(true)
	for _, width := range []int{10, 20, 40, 60} {
		got := RenderFooterHints(width, ContextLibrary, HintModeDefault, reg, s)
		plain := ansi.Strip(got)
		if ansi.StringWidth(got) > width {
			t.Errorf("width %d: strip is %d columns wide: %q", width, ansi.StringWidth(got), plain)
		}
		if !strings.HasSuffix(plain, "? help") {
			t.Errorf("width %d: strip should end with \"? help\", got %q", width, plain)
		}
	}
	if got := ansi.Strip(RenderFooterHints(40, ContextLibrary, HintModeDefault, reg, s)); !strings.HasPrefix(got, "  enter enqueue") {
		t.Errorf("primary action should survive compact layout, got %q", got)
	}
}

func TestRenderFooterHints_KeyLabelAsymmetry(t *testing.T) {
	// Pair tokens must be styled per-token: keys through HelpKey, labels
	// through TextSecondary, separators through Dim — not the prior
	// blanket-Dim rendering.
	reg := NewRegistry()
	s := styles.NewStyles(true)
	got := RenderFooterHints(200, ContextLibrary, HintModeDefault, reg, s)

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
	reg := NewRegistry()
	reg.primaries[primaryHintKey{ctx: ContextLibrary, mode: HintModeDefault}] = []primaryHint{{Key: "enter", Label: "enqueue"}}
	s := styles.NewStyles(true)
	// Width that fits the two-space indent, "j/k", a space, and an ellipsis
	// but not the full label "nav".
	got := RenderFooterHints(10, ContextLibrary, HintModeDefault, reg, s)
	if got == "" {
		t.Fatalf("expected non-empty truncated output")
	}
	keyOpen := strings.Split(s.HelpKey.Render("X"), "X")[0]
	if !strings.Contains(got, keyOpen+"enter") {
		t.Errorf("expected HelpKey-wrapped 'enter' in truncated output; got %q", got)
	}
	if !strings.Contains(ansi.Strip(got), "…") {
		t.Errorf("expected ellipsis in truncated output plain=%q", ansi.Strip(got))
	}
}

func TestRenderFooterHints_ContextSpecificContent(t *testing.T) {
	reg := NewRegistry()
	s := styles.NewStyles(true)
	queue := ansi.Strip(RenderFooterHints(200, ContextQueue, HintModeDefault, reg, s))
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
	reg := NewRegistry()
	s := styles.NewStyles(true)
	// Library selection set surfaces bulk-action labels distinct from default.
	got := ansi.Strip(RenderFooterHints(200, ContextLibrary, HintModeSelection, reg, s))
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
	q := ansi.Strip(RenderFooterHints(200, ContextQueue, HintModeSelection, reg, s))
	for _, want := range []string{"play", "remove", "save", "clear"} {
		if !strings.Contains(q, want) {
			t.Errorf("Queue selection hint strip missing %q: %q", want, q)
		}
	}
}

func TestRenderFooterHints_VisualMode(t *testing.T) {
	reg := NewRegistry()
	s := styles.NewStyles(true)
	for _, ctx := range []Context{ContextLibrary, ContextQueue, ContextArtists, ContextFavorites} {
		got := ansi.Strip(RenderFooterHints(200, ctx, HintModeVisual, reg, s))
		for _, want := range []string{"j/k", "extend", "v", "exit", "esc", "cancel", "X", "clear"} {
			if !strings.Contains(got, want) {
				t.Errorf("ctx %d visual hint strip missing %q: %q", ctx, want, got)
			}
		}
	}
}

func TestRenderFooterHints_VisualFallsBackToDefaultWhenUnregistered(t *testing.T) {
	reg := NewRegistry()
	s := styles.NewStyles(true)
	// contextMixes has no visual-mode hints registered — should fall back to default.
	got := ansi.Strip(RenderFooterHints(200, ContextMixes, HintModeVisual, reg, s))
	def := ansi.Strip(RenderFooterHints(200, ContextMixes, HintModeDefault, reg, s))
	if got != def {
		t.Errorf("expected visual mode to fall back to default for contextMixes; got=%q want=%q", got, def)
	}
}

func TestRenderFooterHints_DefaultModeMatchesCurrent(t *testing.T) {
	// Regression guard: the default-mode hint set must remain identical to
	// what existed before HintMode was introduced. If a new default-mode
	// hint is added we want the change to be deliberate.
	reg := NewRegistry()
	s := styles.NewStyles(true)
	got := ansi.Strip(RenderFooterHints(200, ContextLibrary, HintModeDefault, reg, s))
	want := []string{"j/k nav", "enter enqueue", ": commands", "/ filter", "v select", "? help"}
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
	reg := NewRegistry()
	for _, ctx := range []Context{
		ContextLibrary, ContextQueue, ContextPlaylists,
		ContextBrowse, ContextArtists, ContextAlbums, ContextGenres,
		ContextFavorites, ContextTags, ContextMixes,
	} {
		hints := reg.primaryHints(ctx)
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
		Bindings:      map[RegistryKey]Action{},
		primaries:     map[primaryHintKey][]primaryHint{},
		domainReasons: map[RegistryKey]string{},
	}
	reg.primaries[primaryHintKey{ctx: ContextLibrary, mode: HintModeDefault}] = []primaryHint{
		{Key: "ab", Label: ""}, // 2 columns rendered, no trailing space
	}
	s := styles.NewStyles(true)
	// Width budget: 2 (leading) + 2 (key) == 4. With the previous accounting
	// (treating the pair as "ab " = 3 cols) this would fall into the
	// truncation fallback even though the key fits exactly.
	got := RenderFooterHints(4, ContextLibrary, HintModeDefault, reg, s)
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

// TestRenderFooterHints_CreateRenameMatchAcrossPlaylistsAndTags guards that
// Playlists and Tags word and order their create/rename hints the same way.
func TestRenderFooterHints_CreateRenameMatchAcrossPlaylistsAndTags(t *testing.T) {
	reg := NewRegistry()
	s := styles.NewStyles(true)
	for _, ctx := range []Context{ContextPlaylists, ContextTags} {
		got := ansi.Strip(RenderFooterHints(200, ctx, HintModeDefault, reg, s))
		if !strings.Contains(got, "c create · r rename") {
			t.Errorf("context %d: want \"c create · r rename\", got %q", ctx, got)
		}
	}
}

// TestRenderFooterHints_SelectionLabelsRemoveWhereARemoves guards that the
// selection footer names what a does: in Playlists and Favorites it removes
// the selection, so "bulk action" undersold a destructive key.
func TestRenderFooterHints_SelectionLabelsRemoveWhereARemoves(t *testing.T) {
	reg := NewRegistry()
	s := styles.NewStyles(true)
	for _, ctx := range []Context{ContextPlaylists, ContextFavorites} {
		got := ansi.Strip(RenderFooterHints(200, ctx, HintModeSelection, reg, s))
		if !strings.Contains(got, "a remove") || strings.Contains(got, "bulk action") {
			t.Errorf("context %d: want \"a remove\" and no \"bulk action\", got %q", ctx, got)
		}
	}
}
