package tui

import (
	"context"
	"strings"
	"testing"
)

// TestWithConfigWarnings verifies that config validation warnings are stored on
// the model and that New sets a status-bar notification when warnings are present.
func TestWithConfigWarnings(t *testing.T) {
	warnings := []string{"default_volume 150 is out of range (0-100), using 100"}
	m := New(context.Background(), WithConfigWarnings(warnings))

	if len(m.configWarnings) != 1 || m.configWarnings[0] != warnings[0] {
		t.Fatalf("configWarnings = %v, want %v", m.configWarnings, warnings)
	}

	if m.statusBar.notification.msg == "" {
		t.Fatal("expected statusBar.notification to be set after New() with config warnings")
	}
	if !strings.Contains(m.statusBar.notification.msg, "default_volume") {
		t.Errorf("notification msg = %q, expected it to mention default_volume", m.statusBar.notification.msg)
	}
}

// TestWithConfigWarnings_Empty verifies that no notification is set when warnings is nil or empty.
func TestWithConfigWarnings_Empty(t *testing.T) {
	m := New(context.Background(), WithConfigWarnings(nil))
	if m.statusBar.notification.msg != "" {
		t.Fatalf("expected no notification for empty warnings, got %q", m.statusBar.notification.msg)
	}
}
