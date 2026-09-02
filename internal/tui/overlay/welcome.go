package overlay

import (
	"strings"

	"github.com/sglavoie/muse/internal/tui/text"
)

const welcomeKeyColWidth = 20

func WelcomeRow(key, action string) Row {
	return Row{Text: text.Pad(key, welcomeKeyColWidth) + action}
}

// MusicDirHeadline picks the overlay's first line. A partially valid
// configuration must not be told that no directories were found: the valid
// roots are being scanned, and the user's job is to fix the rest.
func MusicDirHeadline(partial bool) string {
	if partial {
		return "Some music directories could not be read."
	}
	return "No music directories found."
}

// HintRows turns config.ConfigHint's multi-line instruction block into overlay
// rows. The text is reused verbatim (leading blank line aside) so the overlay
// and the validation error can never disagree about which key to edit.
func HintRows(hint string) []Row {
	var rows []Row
	for _, line := range strings.Split(hint, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rows = append(rows, Row{Text: line})
	}
	return rows
}
