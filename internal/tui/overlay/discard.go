package overlay

import "github.com/sglavoie/muse/internal/tui/styles"

// DiscardPrompt protects a draft without changing the underlying form or focus.
// Its zero value selects Keep editing when opened.
type DiscardPrompt struct {
	Active  bool
	discard bool
}

// HandleKey returns true only after an explicit choice to discard the draft.
func (p DiscardPrompt) HandleKey(key string) (DiscardPrompt, bool) {
	switch key {
	case "esc":
		return DiscardPrompt{}, false
	case "tab", "shift+tab", "left", "right", "up", "down":
		p.discard = !p.discard
	case "enter":
		return DiscardPrompt{}, p.discard
	}
	return p, false
}

func (p DiscardPrompt) Render(width, height int, s styles.Styles) string {
	return (Box{
		Title: " Discard unsaved changes? ",
		Rows: []Row{
			{Text: "Keep editing", Styled: !p.discard},
			{Text: "Discard changes", Styled: p.discard},
		},
		Hint:          "tab choose · enter confirm · esc keep editing",
		MaxInnerWidth: max(1, width-4),
	}).Render(width, height, s)
}
