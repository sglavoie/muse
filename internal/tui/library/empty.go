package library

import "github.com/sglavoie/muse/internal/tui/styles"

func (v Panel) renderEmpty() string {
	title := "Library is empty"
	actions := []styles.EmptyAction{{Key: "ctrl+r", Label: "rescan folders"}, {Key: "?", Label: "help"}}
	switch v.StripViewLabel {
	case "Recent":
		title = "No recently added tracks"
		actions = []styles.EmptyAction{{Key: "`", Label: "switch library mode"}, {Key: "ctrl+r", Label: "rescan folders"}}
	case "History":
		title = "No listening history yet"
		actions = []styles.EmptyAction{{Key: "`", Label: "browse all tracks and play something"}, {Key: "?", Label: "help"}}
	}
	return v.Styles.RenderEmptyCard(v.Width, v.visibleRows(), title, actions)
}
