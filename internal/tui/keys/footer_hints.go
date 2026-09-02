package keys

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// footerHintSep is the visual divider between adjacent "key label" pairs in
// the footer hint strip. The middle-dot character matches the existing
// palette hint footer (command_palette.go).
const footerHintSep = " · "

// RenderFooterHints builds the single-line footer hint strip shown above the
// status bar. It returns an empty string when no primaries are registered for
// (ctx, mode) (so the caller can avoid reserving a row).
//
// Each "key label" pair is rendered with explicit per-token styling — keys in
// HelpKey (bold + accent foreground), labels in TextSecondary, separators in
// Dim — so keys can be picked up peripherally while labels stay legible and
// separators recede.
//
// mode selects the hint set: HintModeDefault for the resting state,
// HintModeSelection when 1+ rows are bulk-selected, and HintModeVisual
// while visual line-selection is active. When a mode-specific set is not
// registered for the context, the default-mode set is shown.
//
// The returned string is at most width display columns wide. Pairs that
// don't fit are dropped from the right; any partially-rendered pair is
// elided with "…" so the strip never wraps.
func RenderFooterHints(width int, ctx Context, mode HintMode, r Registry, s styles.Styles) string {
	if width <= 0 {
		return ""
	}
	hints := r.primaryHintsForMode(ctx, mode)
	if len(hints) == 0 {
		return ""
	}

	const leading = "  "
	avail := width - ansi.StringWidth(leading)
	if avail <= 0 {
		return ""
	}

	styled, rendered := renderHintsWithinBudget(hints, avail, s)
	if rendered == 0 {
		// Even the first pair didn't fit — truncate it so something shows.
		truncated := ansi.Truncate(hintPairText(hints[0]), avail, "…")
		styled = renderTruncatedHint(truncated, hints[0].Key, s)
	}
	return leading + styled
}

// renderHintsWithinBudget styles as many separated hint pairs as fit in avail
// columns, dropping the rest from the right, and reports how many it kept.
func renderHintsWithinBudget(hints []primaryHint, avail int, s styles.Styles) (string, int) {
	var styled strings.Builder
	used := 0
	rendered := 0
	for i, h := range hints {
		pieceW := ansi.StringWidth(hintPairText(h))
		if i > 0 {
			pieceW += ansi.StringWidth(footerHintSep)
		}
		if used+pieceW > avail {
			break
		}
		if i > 0 {
			styled.WriteString(s.Dim.Render(footerHintSep))
		}
		styled.WriteString(renderHintKeyLabel(h.Key, h.Label, s))
		used += pieceW
		rendered++
	}
	return styled.String(), rendered
}

// hintPairText is the unstyled "key label" text a hint occupies. It is what
// the width budget is measured against, so it must track renderHintKeyLabel
// exactly: a label-less hint is the key alone, with no trailing space, or
// label-less hints would be dropped a column too early.
func hintPairText(h primaryHint) string {
	if h.Label == "" {
		return h.Key
	}
	return h.Key + " " + h.Label
}

// renderHintKeyLabel styles a single "key label" pair: HelpKey for the key,
// a plain space, then TextSecondary for the label. A label-less hint is
// rendered as the key alone in HelpKey. Width arithmetic upstream uses the
// unstyled string, so this function never changes display width.
func renderHintKeyLabel(key, label string, s styles.Styles) string {
	if label == "" {
		return s.HelpKey.Render(key)
	}
	return s.HelpKey.Render(key) + " " + s.TextSecondary.Render(label)
}

// renderTruncatedHint styles the degraded-state fallback used when even the
// first hint pair must be elided to fit the available width. If the full key
// survives truncation, the key is rendered in HelpKey and the remainder in
// Dim so the most useful token still pops. Otherwise the whole truncated
// chunk falls back to Dim (degraded state).
func renderTruncatedHint(truncated, key string, s styles.Styles) string {
	// truncated starts with key only if the key fits in full followed by a
	// space (the separator between key and label in the unstyled pair).
	if len(truncated) > len(key) && truncated[:len(key)] == key && truncated[len(key)] == ' ' {
		rest := truncated[len(key)+1:]
		if rest == "" {
			return s.HelpKey.Render(key)
		}
		return s.HelpKey.Render(key) + " " + s.Dim.Render(rest)
	}
	return s.Dim.Render(truncated)
}
