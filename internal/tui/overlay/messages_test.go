package overlay

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMessagesScrollWrapAndResize(t *testing.T) {
	messages := make([]string, 30)
	for i := range messages {
		messages[i] = fmt.Sprintf("message %02d %s", i, strings.Repeat("音楽 ", 30))
	}
	v := NewMessages(messages)
	top := ansi.Strip(v.Render(60, 12, newTestStyles()))
	if !strings.Contains(top, "message 29") {
		t.Fatal("newest message not first")
	}
	v.Scroll("G", 60, 12)
	bottom := ansi.Strip(v.Render(60, 12, newTestStyles()))
	if !strings.Contains(bottom, "message 00") || strings.Contains(bottom, "message 29") {
		t.Fatal("cannot read oldest message")
	}
	for _, size := range [][2]int{{100, 27}, {60, 12}} {
		v.Scroll("k", size[0], size[1])
		screen := ansi.Strip(v.Render(size[0], size[1], newTestStyles()))
		lines := strings.Split(screen, "\n")
		if len(lines) > size[1] {
			t.Fatalf("height %d exceeds %d", len(lines), size[1])
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("line too wide: %q", line)
			}
		}
	}
	v.Scroll("g", 60, 12)
	if !strings.Contains(ansi.Strip(v.Render(60, 12, newTestStyles())), "message 29") {
		t.Fatal("home failed")
	}
}

func TestMessagesPositionAndCopySnapshot(t *testing.T) {
	long := strings.Repeat("音楽", 80)
	v := NewMessages([]string{"old", "\x1b[31mnew\x1b[0m " + long})
	if got := v.CopyText(); got != "new "+long+"\n\nold" {
		t.Fatalf("copy wrapped or changed order: %q", got)
	}
	// Force enough wrapped lines for a position indicator.
	v = NewMessages([]string{strings.Repeat(long, 6)})
	top := ansi.Strip(v.Render(60, 12, newTestStyles()))
	if !strings.Contains(top, "lines 1–10/") {
		t.Fatal(top)
	}
	v.Scroll("G", 60, 12)
	bottom := ansi.Strip(v.Render(60, 12, newTestStyles()))
	if strings.Contains(bottom, "lines 1–10/") || !strings.Contains(bottom, "y copy all") {
		t.Fatal(bottom)
	}
	if NewMessages(nil).CopyText() != "" {
		t.Fatal("empty history copies placeholder")
	}
}
