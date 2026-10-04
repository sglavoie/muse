package overlay

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// MessagesOverlay is a scrollable snapshot; new banners do not move the text
// while someone is reading it. Reopening picks up subsequent messages.
type MessagesOverlay struct {
	messages []string
	offset   int
}

func NewMessages(messages []string) *MessagesOverlay {
	return &MessagesOverlay{messages: messages}
}

// CopyText returns the complete snapshot, newest first, without screen wrapping.
func (v MessagesOverlay) CopyText() string {
	var messages []string
	for i := len(v.messages) - 1; i >= 0; i-- {
		messages = append(messages, ansi.Strip(v.messages[i]))
	}
	return strings.Join(messages, "\n\n")
}

func (v MessagesOverlay) lines(width int) []string {
	if len(v.messages) == 0 {
		return []string{"No recent messages yet"}
	}
	var lines []string
	for i := len(v.messages) - 1; i >= 0; i-- {
		msg := v.messages[i]
		line := ansi.Strip(msg)
		lines = append(lines, strings.Split(ansi.Hardwrap(line, max(1, width), true), "\n")...)
		lines = append(lines, "")
	}
	return lines
}

func (v *MessagesOverlay) Scroll(key string, width, height int) {
	rows := max(1, height-2)
	end := max(0, len(v.lines(width))-rows)
	v.offset = max(0, min(v.offset, end))
	switch key {
	case "j", "down":
		v.offset++
	case "k", "up":
		v.offset--
	case "pgdown", "ctrl+d":
		v.offset += rows
	case "pgup", "ctrl+u":
		v.offset -= rows
	case "g", "home":
		v.offset = 0
	case "G", "end":
		v.offset = end
	}
	v.offset = max(0, min(v.offset, end))
}

func (v MessagesOverlay) Render(width, height int, s styles.Styles) string {
	lines := v.lines(width)
	rows := max(1, height-2)
	start := max(0, min(v.offset, len(lines)-rows))
	body := lines[start:min(len(lines), start+rows)]
	heading := "Recent messages — newest first"
	if len(lines) > rows {
		heading = fmt.Sprintf("Recent messages — lines %d–%d/%d", start+1, start+len(body), len(lines))
	}
	title := s.Heading.Render(text.Truncate(heading, width))
	hint := s.Dim.Render(text.Truncate("j/k scroll · y copy all · q/esc close", width))
	return title + "\n" + strings.Join(body, "\n") + "\n" + hint
}
