package tui

import (
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// One recovery action drives the empty Library card, footer and idle status.
// Read model state only: rendering must never probe the filesystem.
func (m *Model) emptyLibraryGuidance() (string, styles.EmptyAction) {
	switch {
	case m.scanning:
		return "Scanning music folders…", styles.EmptyAction{Key: "esc", Label: "cancel scan"}
	case len(m.cfg.MusicDirectories) == 0 && len(m.libraryRoots) == 0:
		return "Add a music folder to get started", styles.EmptyAction{Key: "ctrl+,", Label: "add music folders"}
	case m.scanFailed || m.musicDirHint != "":
		return "Music folders need attention", styles.EmptyAction{Key: "ctrl+,", Label: "check music folders"}
	default:
		return "No music found in your folders", styles.EmptyAction{Key: "ctrl+,", Label: "check folders and exclusions"}
	}
}

func (m *Model) showsEmptyLibrary() bool {
	return m.ActiveView == viewLibrary && m.library.Mode == libtui.LibraryModeAll &&
		m.library.Index.Len() == 0 && !m.library.Filter.Active && m.library.Filter.Text == "" && m.library.TagFilterHashes == nil
}
