package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestNumberedNavigationStableWithFilters(t *testing.T) {
	m := newRichFixture()
	for _, width := range []int{60, 80, 100, 160} {
		m.termWidth = width
		for _, id := range viewOrder {
			m.ActiveView = id
			before := ansi.Strip(m.renderTitleBar())
			if ansi.StringWidth(before) > width {
				t.Fatalf("width %d: %s", width, before)
			}
			for i := 1; i <= 8; i++ {
				if !strings.Contains(before, fmt.Sprintf("%d ", i)) {
					t.Fatalf("missing shortcut %d at %d: %s", i, width, before)
				}
			}
			tag := int64(99)
			m.activeTagFilter = &tag
			m.tagNames[tag] = strings.Repeat("長", 100)
			if after := ansi.Strip(m.renderTitleBar()); after != before {
				t.Fatal("filter moved tabs")
			}
			if ansi.StringWidth(m.renderTitleDivider()) > width {
				t.Fatal("filter divider overflow")
			}
			m.activeTagFilter = nil
		}
	}
}

func TestInspectingMessagesAcknowledgesOnlyExistingErrors(t *testing.T) {
	m := newRichFixture()
	m.termWidth = 80
	m.statusBar.TransientDur = -time.Second
	m.statusBar.NotifyError("Save failed")
	if !strings.Contains(ansi.Strip(m.RenderStatusBar()), "! errors") {
		t.Fatal("expired failure has no unread indicator")
	}
	m = m.showRecentMessages()
	if strings.Contains(ansi.Strip(m.RenderStatusBar()), "! errors") {
		t.Fatal("inspected errors remain unread")
	}
	if !strings.Contains(m.overlay.messages.CopyText(), "Save failed") {
		t.Fatal("acknowledgement lost history")
	}
	m.statusBar.NotifyError("Another failure")
	if !strings.Contains(ansi.Strip(m.RenderStatusBar()), "! errors") {
		t.Fatal("new error was acknowledged without inspection")
	}
}

func TestTrackTablesUseConsistentIdentityOrder(t *testing.T) {
	for _, width := range []int{60, 80, 100} {
		m := newRichFixture()
		m.termWidth = width
		m.library.SetSize(width, 20)
		m.queue.Width = width
		m.queue.Height = 20
		var headers []string
		for _, view := range []viewID{viewLibrary, viewQueue} {
			m.ActiveView = view
			lines := strings.Split(ansi.Strip(m.renderPrimaryBody()), "\n")
			header := ""
			for _, line := range lines {
				if strings.Contains(line, "Title") && strings.Contains(line, "Artist") {
					header = line
					break
				}
			}
			if header == "" || strings.Index(header, "Title") >= strings.Index(header, "Artist") {
				t.Fatalf("view %v width %d: %s", view, width, header)
			}
			headers = append(headers, header)
		}
		if headers[0] != headers[1] {
			t.Fatalf("track headers disagree at %d:\n%s\n%s", width, headers[0], headers[1])
		}
	}
}
