package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

func TestFailureFeedbackInBannerAndMessages(t *testing.T) {
	for _, msg := range []tea.Msg{
		metadataImportParsedMsg{err: errors.New("permission denied")},
		metadataExportedMsg{err: errors.New("disk full")},
		statusbar.TransientErrorMsg("Smart playlist error: invalid rule"),
		metadataImportAppliedMsg{applied: 1, failed: 1},
	} {
		m := newRichFixture()
		m.setUndoSlot(undoClearAction{}, nil)
		m, _ = updateModel(m, msg)
		notice := m.statusBar.TopNotification()
		if notice.Severity != statusbar.SeverityError {
			t.Fatalf("%T did not produce an error: %+v", msg, notice)
		}
		m = m.showRecentMessages()
		if copied := m.overlay.messages.CopyText(); !strings.Contains(copied, "[error] "+notice.Msg) {
			t.Fatalf("%T lost severity in recent messages: %q", msg, copied)
		}
		for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 15}, {Width: 100, Height: 30}} {
			m, _ = updateModel(m, size)
			for _, line := range strings.Split(viewContent(m), "\n") {
				if ansi.StringWidth(line) > size.Width {
					t.Fatalf("%T messages overflow at %dx%d: %q", msg, size.Width, size.Height, line)
				}
			}
			m.overlay.messages = nil
			bar := ansi.Strip(m.RenderStatusBar())
			if strings.Contains(bar, "s)") {
				t.Fatalf("%T error borrowed undo countdown: %s", msg, bar)
			}
			m = m.showRecentMessages()
		}
	}
}

func TestCompletedUndoDoesNotResurface(t *testing.T) {
	m := newRichFixture()
	m.setUndoSlot(undoClearAction{}, nil)
	m = m.handleUndo()
	// Expire the success banner while the old undo prompt would still live.
	for i := range m.statusBar.Notifications {
		if strings.HasPrefix(m.statusBar.Notifications[i].Msg, "Undone:") {
			m.statusBar.Notifications[i].ExpiresAt = time.Now().Add(-time.Second)
		}
	}
	if got := ansi.Strip(m.RenderStatusBar()); strings.Contains(got, "undo") {
		t.Fatalf("completed undo prompt returned: %s", got)
	}
}
