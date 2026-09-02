package uitest

// TestFixtureDriverRunsExactNavigationByName was carved out of
// internal/tui/root_command_palette_navigation_test.go when the driver moved
// here: it reaches UITestDriver.runCommand and UITestDriver.model, which are
// package-internal to uitest, so package tui can no longer host it.

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui"
)

func TestFixtureDriverRunsExactNavigationByName(t *testing.T) {
	driver := newTestDriver(t, tui.FixtureRich, 100, 30)
	for _, tc := range []struct {
		name string
		mode string
	}{
		{name: "Go to Library: Recently Added", mode: "Recent"},
		{name: "Go to Library: History", mode: "History"},
	} {
		if err := driver.runCommand(tc.name); err != nil {
			t.Fatalf("runCommand(%q): %v", tc.name, err)
		}
		got := driver.model.Snapshot()
		if got.PrimaryView != "Library" || got.Mode != tc.mode {
			t.Fatalf("runCommand(%q) snapshot = view %q mode %q", tc.name, got.PrimaryView, got.Mode)
		}
	}
}
