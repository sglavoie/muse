package statusbar

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func TestErrorSeveritySurvivesBannerExpiry(t *testing.T) {
	s := StatusBar{TransientDur: -time.Second}
	s.NotifyError("Import failed: permission denied")
	s.PruneNotifications()
	if s.TopNotification().Msg != "" {
		t.Fatal("expired error still visible")
	}
	history := s.RecentMessages()
	if len(history) != 1 || history[0].Severity != SeverityError || history[0].Text != "Import failed: permission denied" {
		t.Fatalf("error was not retained with its severity: %+v", history)
	}
	// Presentation depends on severity even when the wording lacks Error:.
	normal := lipgloss.NewStyle().Bold(true)
	errorStyle := lipgloss.NewStyle().Italic(true)
	style := notificationStyle(history[0].Severity, true, normal, errorStyle)
	if !style.GetItalic() || !style.GetFaint() || style.GetBold() {
		t.Fatal("error did not receive the fading error style")
	}
	s.Notify("Error: is part of a track title")
	if s.RecentMessages()[1].Severity != severityNormal {
		t.Fatal("ordinary message inferred severity from its text")
	}
}

func TestUndoBannerIdentityAndLifetime(t *testing.T) {
	s := StatusBar{}
	deadline := time.Now().Add(time.Minute)
	s.NotifyUndo("Removed track · u to undo", deadline)
	if got := s.TopNotification().Msg; got != "Removed track · u to undo" {
		t.Fatalf("undo prompt missing: %q", got)
	}
	s.NotifyError("Export failed")
	if got := s.TopNotification().Msg; got != "Export failed" {
		t.Fatalf("unrelated error acquired a countdown: %q", got)
	}
	// After the unrelated banner expires, the same undo remains available.
	s.Notifications[1].ExpiresAt = time.Now().Add(-time.Second)
	if got := s.TopNotification().Msg; got != "Removed track · u to undo" {
		t.Fatalf("undo did not return: %q", got)
	}
	// A newer undo supersedes the old prompt, even if its banner expires first.
	s.NotifyUndo("Second action · u to undo", deadline.Add(time.Second))
	s.Notifications[len(s.Notifications)-1].ExpiresAt = time.Now().Add(-time.Second)
	if got := s.TopNotification().Msg; got != "" {
		t.Fatalf("obsolete undo resurfaced: %q", got)
	}
	s.NotifyUndo("Third action · u to undo", deadline.Add(2*time.Second))
	s.UndoExpiresAt = time.Time{}
	if got := s.TopNotification().Msg; got != "" {
		t.Fatalf("completed undo resurfaced: %q", got)
	}
}

func TestFailureOutranksConfirmationAndRemainsDiscoverable(t *testing.T) {
	s := StatusBar{Title: "Recognizable song", PlaybackState: playback.StatePlaying, Elapsed: "0:10", Total: "3:00"}
	s.NotifyError("Save failed: " + strings.Repeat("very long path/", 20))
	s.Notify("Added a track")
	if s.TopNotification().Severity != SeverityError {
		t.Fatal("success displaced failure")
	}
	out := ansi.Strip(s.Render(60, styles.NewStyles(true)))
	if !strings.Contains(out, "Recognizable") || !strings.Contains(out, "0:10 / 3:00") || !strings.Contains(out, "! errors") {
		t.Fatalf("notice hid playback or recovery: %s", out)
	}
	for i := range s.Notifications {
		s.Notifications[i].ExpiresAt = time.Now().Add(-time.Second)
	}
	s.PruneNotifications()
	if !s.hasUnreadErrors {
		t.Fatal("expiry acknowledged error")
	}
	s.MarkMessagesRead()
	if s.hasUnreadErrors || len(s.RecentMessages()) != 2 {
		t.Fatal("inspection must acknowledge without deleting history")
	}
}

func TestPlaybackIdentityAndModesSurviveLongNotification(t *testing.T) {
	s := StatusBar{Title: "Recognizable song", Artist: strings.Repeat("Artist ", 30), Album: strings.Repeat("Album ", 30), PlaylistSource: strings.Repeat("Playlist ", 30), QueuePosition: "999 of 1000", BucketLabel: strings.Repeat("Bucket ", 30), PlaybackState: playback.StatePlaying, RepeatMode: playback.RepeatAll, Shuffle: true, StopAfterCurrent: true, RadioMode: true, Elapsed: "0:10", Total: "3:00"}
	s.NotifyError(strings.Repeat("Failure ", 40))
	for _, width := range []int{60, 80, 100, 160} {
		screen := ansi.Strip(s.Render(width, styles.NewStyles(true)))
		for _, want := range []string{"Recogniz", "0:10 / 3:00", "! errors"} {
			if !strings.Contains(screen, want) {
				t.Fatalf("width %d missing %q: %s", width, want, screen)
			}
		}
		wantModes := "Rpt:all Shuf Stop DJ"
		if width == 160 {
			wantModes = "Repeat all · Shuffle · Stop after track · Auto-DJ"
		}
		if !strings.Contains(screen, wantModes) {
			t.Fatalf("width %d hid modes: %s", width, screen)
		}
		for _, line := range strings.Split(screen, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("overflow at %d: %s", width, line)
			}
		}
	}
}
