package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

func TestFilterHistorySaveFailureDeduplicatesAndRecovers(t *testing.T) {
	dir := t.TempDir()
	m := New(context.Background(), WithDataDir(dir))
	path := filepath.Join(dir, "filter_history.json")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	commit := func(query string) {
		m = pressEsc(m)
		m = pressEnter(typeText(pressSlash(m), query))
	}
	commit("Alice")
	commit("Bob")
	countWarnings := func() int {
		n := 0
		for _, msg := range m.statusBar.RecentMessages() {
			if strings.Contains(msg.Text, "Filter history not saved") {
				n++
				if msg.Severity != statusbar.SeverityError {
					t.Fatal("warning not recorded as error")
				}
			}
		}
		return n
	}
	if countWarnings() != 1 || len(m.filterHistory) != 2 || m.library.Filter.Text != "Bob" {
		t.Fatal("failure lost query history, changed the filter, or repeated the warning")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	commit("Charlie")
	history, err := filter.LoadHistory(dir)
	if err != nil || len(history) != 3 || m.filterHistorySaveFailed {
		t.Fatalf("recovery: %v, %v", history, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	commit("Delta")
	if countWarnings() != 2 {
		t.Fatal("new failure after recovery was not reported")
	}
}
