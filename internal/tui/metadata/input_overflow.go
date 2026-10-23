package metadata

import (
	"charm.land/bubbles/v2/textinput"
	"github.com/sglavoie/muse/internal/tui/text"
)

func inputWithOverflow(input textinput.Model, width int) string {
	return text.InputWithOverflow(input, width)
}
