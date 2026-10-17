package statusbar

import (
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
