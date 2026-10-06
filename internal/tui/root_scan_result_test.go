package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

type failingRescanWriter struct {
	upsertErr, genreErr, missingErr error
	upserts, genres, missing        int
}

func (w *failingRescanWriter) UpsertBatch([]library.Track) error {
	w.upserts++
	return w.upsertErr
}

func (w *failingRescanWriter) PopulateTrackGenres([]library.Track) error {
	w.genres++
	return w.genreErr
}

func (w *failingRescanWriter) MarkTrackMissing(string) error {
	w.missing++
	return w.missingErr
}

func TestRescanSummaryTracksIdentities(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before []library.Track
		after  []library.Track
		want   string
	}{
		{"empty", nil, nil, "0 tracks · 0 added · 0 missing"},
		{"first scan", nil, []library.Track{{Hash: "new", Path: "/new"}}, "1 track · 1 added · 0 missing"},
		{"unchanged", []library.Track{{Hash: "a", Path: "/a"}}, []library.Track{{Hash: "a", Path: "/a"}}, "1 track · 0 added · 0 missing"},
		{"moved", []library.Track{{Hash: "a", Path: "/old"}}, []library.Track{{Hash: "a", Path: "/new"}}, "1 track · 0 added · 0 missing"},
		{"added and missing", []library.Track{{Hash: "old", Path: "/old"}}, []library.Track{{Hash: "new", Path: "/new"}}, "1 track · 1 added · 1 missing"},
		{"replacement", []library.Track{{Hash: "old", Path: "/same"}}, []library.Track{{Hash: "new", Path: "/same"}}, "1 track · 1 added · 1 missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background())
			m.scanning = true
			m.rescanSnapshot = make(map[string]rescanEntry)
			m.rescanPriorPaths = make(map[string]string)
			for _, track := range tc.before {
				m.rescanSnapshot[track.Hash] = rescanEntry{}
				m.rescanPriorPaths[track.Path] = track.Hash
			}
			m, _ = m.handleScanBatchMsg(library.ScanBatchMsg{Tracks: tc.after})
			m, _ = m.handleScanDoneMsg()
			if got := m.statusBar.TopNotification().Msg; got != "Scan complete — "+tc.want {
				t.Fatalf("summary = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRescanWriteFailuresReachRecentMessages(t *testing.T) {
	for _, operation := range []string{"tracks", "genres", "missing", "success"} {
		t.Run(operation, func(t *testing.T) {
			writer := &failingRescanWriter{}
			failure := errors.New("disk full")
			switch operation {
			case "tracks":
				writer.upsertErr = failure
			case "genres":
				writer.genreErr = failure
			case "missing":
				writer.missingErr = failure
			}
			m := New(context.Background(), WithScanWriter(writer, 1))
			m.scanning = true
			m.rescanSnapshot = map[string]rescanEntry{"gone": {}}
			m.rescanPriorPaths = map[string]string{"/gone": "gone"}
			raw := make(chan library.Track, 2)
			raw <- library.Track{Hash: "a", Path: "/a"}
			raw <- library.Track{Hash: "b", Path: "/b"}
			close(raw)
			ch, outcome := m.bridgeScanChannel(raw)
			m.scanWriteOutcome = outcome
			for track := range ch {
				m, _ = m.handleScanBatchMsg(library.ScanBatchMsg{Tracks: []library.Track{track}})
			}
			m, cmd := m.handleScanDoneMsg()
			if cmd == nil {
				t.Fatal("expected missing-track persistence command")
			}
			if len(m.statusBar.RecentMessages()) != 0 {
				t.Fatal("scan reported completion before missing-track writes finished")
			}
			next, _ := m.Update(cmd())
			m = next.(Model)
			history := m.statusBar.RecentMessages()
			if len(history) != 1 {
				t.Fatalf("expected one bounded result, got %+v", history)
			}
			if writer.upserts != 2 || writer.genres != 2 || writer.missing != 1 {
				t.Fatal("write failure stopped remaining scan writes")
			}
			if !strings.Contains(history[0].Text, "2 added · 1 missing") {
				t.Fatalf("missing change counts: %+v", history)
			}
			if operation == "success" {
				if !strings.HasPrefix(history[0].Text, "Scan complete") {
					t.Fatalf("missing success result: %+v", history)
				}
			} else {
				retry := "ctrl+r"
				if operation == "missing" {
					retry = "restart muse"
				}
				if history[0].Severity != statusbar.SeverityError || !strings.Contains(history[0].Text, "disk full") || !strings.Contains(history[0].Text, retry) || strings.Contains(history[0].Text, "Scan complete") {
					t.Fatalf("missing actionable save error: %+v", history)
				}
			}
		})
	}
}

func TestCanceledRescanDrainsBeforeAnotherStarts(t *testing.T) {
	ch := make(chan library.Track)
	m := New(context.Background(), WithScanChannel(ch), WithLibraryRoots([]string{t.TempDir()}), WithExtractor(stubExtractor{}))
	m, _, _ = m.handleScanCancelKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = next.(Model)
	if cmd != nil || m.scanning || m.scanCh != ch {
		t.Fatal("new scan replaced a canceled scan before its bridge drained")
	}
	close(ch)
	m, _ = m.handleScanDoneMsg()
	if m.statusBar.TopNotification().Msg != "Scan canceled" {
		t.Fatal("draining canceled scan replaced cancellation with success")
	}
}

func TestRescanFinalBatchFailureAndRecovery(t *testing.T) {
	writer := &failingRescanWriter{upsertErr: errors.New("read-only database")}
	m := New(context.Background(), WithScanWriter(writer, 500))
	for _, fail := range []bool{true, false} {
		if !fail {
			writer.upsertErr = nil
		}
		m.scanning = true
		m.scanned = 0
		raw := make(chan library.Track, 1)
		raw <- library.Track{Hash: "a", Path: "/a"}
		close(raw)
		ch, outcome := m.bridgeScanChannel(raw)
		m.scanWriteOutcome = outcome
		for track := range ch {
			m, _ = m.handleScanBatchMsg(library.ScanBatchMsg{Tracks: []library.Track{track}})
		}
		var cmd tea.Cmd
		m, cmd = m.handleScanDoneMsg()
		if cmd != nil || m.scanWriteOutcome != nil {
			t.Fatal("scan without missing marks should finish and release its outcome")
		}
		history := m.statusBar.RecentMessages()
		got := history[len(history)-1].Text
		if fail && !strings.Contains(got, "read-only database") {
			t.Fatalf("final partial batch failure lost: %s", got)
		}
		if !fail && got != "Scan complete — 1 track" {
			t.Fatalf("previous failure leaked into successful retry: %s", got)
		}
	}
}
