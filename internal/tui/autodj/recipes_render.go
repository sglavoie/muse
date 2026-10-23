package autodj

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// buildRecipesLines renders the Recipes page as a box sized to the terminal.
func (p TunePanel) buildRecipesLines(termWidth, termHeight int, s styles.Styles) []string {
	inner := innerWidth(termWidth)
	content := max(inner-4, 0)
	lines := p.panelHeader(tabRecipes, s, inner)

	logical, focus := p.recipeLines()
	body, focusLine := wrapRecipeLines(logical, focus, content)
	start, end := recipeWindow(len(body), focusLine, overlay.PanelContentHeight(termHeight))
	for i := start; i < end; i++ {
		lines = append(lines, boxBorderV+"  "+toneStyle(body[i].Tone, s).Render(
			text.Pad(text.Truncate(body[i].Text, content), content))+"  "+boxBorderV)
	}
	lines = append(lines, boxCornerBL+strings.Repeat(boxBorderH, inner)+boxCornerBR)
	return appendPanelHints(lines, p.recipeHint(end-start < len(body)), inner, termHeight, s)
}

// recipeLines assembles the page and reports which logical line the cursor is
// on, so the window can be drawn around it.
func (p TunePanel) recipeLines() ([]previewLine, int) {
	lines := []previewLine{{
		text: "Recipes store settings only. Saving changes nothing that is playing; " +
			"loading edits the draft, which you then " + strings.ToLower(p.CommitLabel()) + ".",
		tone: ToneInfo,
	}}
	focus := 0
	if len(p.recipes) == 0 {
		lines = append(lines, previewLine{
			text: "No saved recipes yet — press s to save these settings under a name.",
			tone: ToneInfo,
		})
	}
	known := p.knownTagIDs()
	for i, r := range p.recipes {
		if i == p.recipeCursor {
			focus = len(lines)
		}
		lines = append(lines, previewLine{text: p.recipeRow(i, r), tone: ToneInfo})
		if note, tone, ok := recipeWarning(r, unresolvedTagIDs(known, r.Settings)); ok {
			lines = append(lines, previewLine{text: note, indent: 4, tone: tone})
		}
	}
	if p.recipeNote.Text != "" {
		lines = append(lines, previewLine{text: p.recipeNote.Text, tone: p.recipeNote.Tone})
	}
	return lines, focus
}

// recipeRow summarizes one stored recipe on a single line.
func (p TunePanel) recipeRow(idx int, r Recipe) string {
	marker := text.FocusMarker(idx == p.recipeCursor)
	if r.Invalid != nil {
		return marker + r.Name + " · unreadable"
	}
	return marker + r.Name + " · " + recipeSummary(r.Settings)
}

// recipeSummary describes a stored snapshot in the same terms the Tune tab uses,
// so a user can tell two recipes apart without loading either of them.
func recipeSummary(s radio.Settings) string {
	summary := fmt.Sprintf("%s · %d%% discovery · %s",
		s.Preset().Label(), s.DiscoveryPercent, s.Anchor.Label())
	if n := len(s.Include.TagIDs) + len(s.Include.Genres) + len(s.Include.Artists); n > 0 {
		summary += fmt.Sprintf(" · %d include", n)
	}
	if n := len(s.Exclude.TagIDs) + len(s.Exclude.Genres) + len(s.Exclude.Artists); n > 0 {
		summary += fmt.Sprintf(" · %d exclude", n)
	}
	return summary
}

// recipeWarning states what stands between a recipe and being usable.
//
// An unresolved reference is reported before the recipe is loaded, not after.
// The references themselves are kept — dropping them on load would silently
// widen what the DJ may play — so the warning names the repair the Tune tab
// already offers, which happens before the settings are ever applied.
func recipeWarning(r Recipe, missing []int64) (string, Tone, bool) {
	if r.Invalid != nil {
		return "stored settings could not be read: " + r.Invalid.Error() +
			" — this recipe can only be deleted", ToneError, true
	}
	if len(missing) == 0 {
		return "", ToneInfo, false
	}
	return fmt.Sprintf("references %d %s that no longer %s — load it, then press r on the Tune tab",
		len(missing),
		text.Pluralize(len(missing), "tag", "tags"),
		text.Pluralize(len(missing), "exists", "exist")), ToneWarn, true
}

// recipeHint is the Recipes page's key legend.
func (p TunePanel) recipeHint(scrolled bool) string {
	scroll := ""
	if scrolled {
		scroll = "j/k scrolls · "
	}
	return scroll + "enter load · s save as · d delete · u update · n rename · ctrl+s " +
		strings.ToLower(p.CommitLabel()) + " · esc cancel"
}

// wrapRecipeLines folds the page onto the box width and reports where the
// focused recipe ended up, which is what the window has to contain.
func wrapRecipeLines(lines []previewLine, focus, width int) ([]StatusLine, int) {
	out := make([]StatusLine, 0, len(lines))
	focusLine := 0
	for i, l := range lines {
		if i == focus {
			focusLine = len(out)
		}
		out = append(out, wrapPreview([]previewLine{l}, width)...)
	}
	return out, focusLine
}

// recipeWindow returns the half-open range of wrapped lines to draw, always
// containing the focused recipe.
func recipeWindow(total, focusLine, termHeight int) (int, int) {
	size := total
	if termHeight > 0 {
		size = max(termHeight-previewChromeLines, 1)
	}
	if size >= total {
		return 0, total
	}
	start := 0
	if focusLine >= size {
		start = min(focusLine-size+1, total-size)
	}
	return start, start + size
}

// buildPromptLines renders the modal name box.
//
// It takes the whole overlay, like the filter picker does: naming a recipe is a
// gesture the user finishes or abandons, and leaving the list underneath visible
// would invite the arrow keys to move a cursor the box has taken over.
func (p TunePanel) buildPromptLines(termWidth int, s styles.Styles) []string {
	inner := innerWidth(termWidth)
	content := max(inner-4, 0)
	prompt := *p.prompt
	lines := boxHeader(prompt.title(), inner, s)

	value := prompt.value
	if value == "" {
		value = s.Dim.Render("type a name…")
	}
	lines = append(lines,
		boxBorderV+"  "+text.Pad(value, content)+"  "+boxBorderV,
		boxCornerBL+strings.Repeat(boxBorderH, inner)+boxCornerBR)
	return append(lines, "  "+s.Dim.Render(
		text.Truncate("enter confirm · ctrl+u clear · esc cancel", inner)))
}
