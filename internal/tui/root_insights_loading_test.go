package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	instui "github.com/sglavoie/muse/internal/tui/insights"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// blockingInsightsDB stands in for the database on both sides of an Insights
// refresh: the readers derive every snapshot from the same rows the writers
// change, and a write parks until the test releases it. That parked interval is
// the window in which a refresh could read pre-change data.
type blockingInsightsDB struct {
	mu      sync.Mutex
	tracks  []library.Track
	release chan struct{}
}

func newBlockingInsightsDB(tracks []library.Track) *blockingInsightsDB {
	return &blockingInsightsDB{tracks: slices.Clone(tracks), release: make(chan struct{})}
}

// rows hands the fixture reader a private copy, so a read on the load goroutine
// and a write on the persistence goroutine never touch the same slice.
func (d *blockingInsightsDB) rows() *uitestInsights {
	d.mu.Lock()
	defer d.mu.Unlock()
	return &uitestInsights{tracks: slices.Clone(d.tracks)}
}

func (d *blockingInsightsDB) ReadInsightPlays(since int64) ([]insights.PlayRow, error) {
	return d.rows().ReadInsightPlays(since)
}

func (d *blockingInsightsDB) ReadInsightGenres(since int64) ([]insights.GenreRow, error) {
	return d.rows().ReadInsightGenres(since)
}

func (d *blockingInsightsDB) ReadInsightSessions(since int64) ([]insights.SessionRow, error) {
	return d.rows().ReadInsightSessions(since)
}

func (d *blockingInsightsDB) ReadInsightCoverage() ([]insights.CoverageRow, error) {
	return d.rows().ReadInsightCoverage()
}

func (d *blockingInsightsDB) ReadInsightTimeline() ([]insights.TimelineRow, error) {
	return d.rows().ReadInsightTimeline()
}

func (d *blockingInsightsDB) ReadInsightQuality(minPlays, maxPlays int, minCompletion float64) (insights.QualityCounts, error) {
	return d.rows().ReadInsightQuality(minPlays, maxPlays, minCompletion)
}

func (d *blockingInsightsDB) ReadUnverifiedDurations() ([]db.UnverifiedTrack, error) {
	return d.rows().ReadUnverifiedDurations()
}

// UpdateTrackMetadata is the write the undo path launches on its own goroutine.
func (d *blockingInsightsDB) UpdateTrackMetadata(hash, title, artist, album, genre string, albumArtist ...string) error {
	<-d.release
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.tracks {
		if d.tracks[i].Hash == hash {
			d.tracks[i].Title, d.tracks[i].Artist = title, artist
			d.tracks[i].Album, d.tracks[i].Genre = album, genre
		}
	}
	return nil
}

// MarkTrackMissing is the write the rescan deletion path returns as a command.
// Every real Insights reader filters missing rows out, so the row leaves here.
func (d *blockingInsightsDB) MarkTrackMissing(hash string) error {
	<-d.release
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tracks = slices.DeleteFunc(d.tracks, func(t library.Track) bool { return t.Hash == hash })
	return nil
}

// UpsertBatch satisfies scanUpsertBatcher so the store can be the scan writer.
func (d *blockingInsightsDB) UpsertBatch([]library.Track) error { return nil }

// newBlockingInsightsModel puts a loaded Insights view in front of store, with
// whatever writers the case under test wires behind it.
func newBlockingInsightsModel(t *testing.T, store *blockingInsightsDB, opts ...Option) Model {
	t.Helper()
	base := []Option{WithLibraryTracks(insightsTestTracks()), WithInsightsReader(store)}
	m := New(context.Background(), append(base, opts...)...)
	m.termWidth, m.termHeight = 100, 30
	m.insights.SetSize(100, 30)
	m = m.switchToView(viewInsights)

	cmd := m.insightsMaybeLoadCmd()
	if cmd == nil {
		t.Fatal("expected a load cmd on first switch to Insights")
	}
	msg, ok := cmd().(insightsLoadedMsg)
	if !ok {
		t.Fatalf("expected insightsLoadedMsg, got %T", cmd())
	}
	return m.handleInsightsLoadedMsg(msg)
}

// releaseWrites unparks the database and waits for the model to report no
// persistence in flight, which is the terminal state the refresh gate opens on.
func releaseWrites(t *testing.T, store *blockingInsightsDB, m Model) {
	t.Helper()
	close(store.release)
	for deadline := time.Now().Add(5 * time.Second); m.insightsPending.busy(); {
		if time.Now().After(deadline) {
			t.Fatal("the Insights persistence gate never reported itself finished")
		}
		time.Sleep(time.Millisecond)
	}
}

func hasGenreGroup(groups []insights.GenreGroup, canonical string) bool {
	for _, g := range groups {
		if g.Canonical == canonical {
			return true
		}
	}
	return false
}

// TestInsightsRefreshWaitsForPersistenceCompletion covers the two changes that
// reach the database on a goroutine nothing listens to: an undo's metadata
// restore and a rescan's deletion marks. Both update memory first, so a refresh
// that runs in between would read half the change and then look current.
func TestInsightsRefreshWaitsForPersistenceCompletion(t *testing.T) {
	t.Run("an undo holds the refresh until its metadata write lands", func(t *testing.T) {
		store := newBlockingInsightsDB(insightsTestTracks())
		m := newBlockingInsightsModel(t, store, WithMetadataWriter(store))
		if hasGenreGroup(m.insights.Snap.Rhythm.Genres, "Punk") {
			t.Fatal("precondition: the database should not know Punk yet")
		}

		// A load already in flight read the database before the undo touched it.
		preUndo := m.insightsLoadCmd()
		if preUndo == nil {
			t.Fatal("expected a load cmd while nothing was pending")
		}
		staleReply, ok := preUndo().(insightsLoadedMsg)
		if !ok {
			t.Fatalf("expected insightsLoadedMsg, got %T", preUndo())
		}

		m.setUndoSlot(undoBulkEditMetadataAction{
			originals: map[string][5]string{"h2": {"Track A2", "Alice", "Alpha", "Punk", "4:00"}},
		}, nil)
		m = updateInsightsModel(t, m, UndoMsg{})

		if !m.insights.Stale {
			t.Fatal("an undone metadata edit left the Insights memo current")
		}
		m = m.handleInsightsLoadedMsg(staleReply)
		if !m.insights.Stale {
			t.Error("a snapshot read before the undo was installed as the current one")
		}
		if cmd := m.insightsActiveRefreshCmd(); cmd != nil {
			t.Fatal("Insights refreshed while the undo's database write was still parked")
		}
		m, _ = m.handleTickMsg()
		if m.insights.Loading {
			t.Fatal("the tick started a load while the undo's database write was still parked")
		}
		if !m.insights.Stale {
			t.Fatal("the declined refresh cleared the staleness that makes the retry happen")
		}

		releaseWrites(t, store, m)

		m = reloadInsights(t, m)
		if !hasGenreGroup(m.insights.Snap.Rhythm.Genres, "Punk") {
			t.Error("the installed snapshot missed the undo the database had accepted")
		}
	})

	t.Run("a rescan deletion holds the refresh until its missing marks land", func(t *testing.T) {
		store := newBlockingInsightsDB(insightsTestTracks())
		m := newBlockingInsightsModel(t, store, WithScanWriter(store, 500))
		before := m.insights.Snap.Coverage.TotalTracks

		// What a completed full rescan leaves behind for a file that vanished: the
		// path is gone from the rebuilt index but still in the pre-rescan snapshot.
		m.library.Index.RemoveByPath("/d1.flac")
		m.scanning = true
		m.rescanPriorPaths = map[string]string{"/d1.flac": "h6"}

		next, cmd := m.Update(library.ScanDoneMsg{})
		m = next.(Model)
		if cmd == nil {
			t.Fatal("scan completion issued no command to persist the deletion")
		}
		if !m.insights.Stale {
			t.Fatal("a completed rescan left the Insights memo current")
		}
		if refresh := m.insightsActiveRefreshCmd(); refresh != nil {
			t.Fatal("Insights refreshed while the rescan deletion was still being persisted")
		}

		persisted := make(chan struct{})
		go func() {
			defer close(persisted)
			cmd()
		}()
		releaseWrites(t, store, m)
		<-persisted

		m = reloadInsights(t, m)
		if got := m.insights.Snap.Coverage.TotalTracks; got != before-1 {
			t.Errorf("coverage kept the deleted track: want %d tracks, got %d", before-1, got)
		}
	})
}

// brokenInsightsReader answers exactly like the fixture reader until fail is
// set, at which point two of its seven reads return an error. Two rather than
// one because the load message is supposed to carry every failure rather than
// the first one it hits.
type brokenInsightsReader struct {
	*uitestInsights
	fail error
}

func (r *brokenInsightsReader) ReadInsightPlays(sinceUnix int64) ([]insights.PlayRow, error) {
	if r.fail != nil {
		return nil, r.fail
	}
	return r.uitestInsights.ReadInsightPlays(sinceUnix)
}

func (r *brokenInsightsReader) ReadInsightCoverage() ([]insights.CoverageRow, error) {
	if r.fail != nil {
		return nil, r.fail
	}
	return r.uitestInsights.ReadInsightCoverage()
}

// newBrokenInsightsModel returns an Insights view that has not loaded yet,
// alongside the reader whose reads the test can break and mend.
func newBrokenInsightsModel(t *testing.T) (Model, *brokenInsightsReader) {
	t.Helper()
	tracks := insightsTestTracks()
	reader := &brokenInsightsReader{uitestInsights: &uitestInsights{tracks: tracks}}
	m := New(context.Background(), WithLibraryTracks(tracks), WithInsightsReader(reader))
	m.termWidth, m.termHeight = 100, 30
	m.insights.SetSize(100, 30)
	return m, reader
}

// enterInsights runs the real view-entry action, which is also the retry after
// a failed read, and returns the load it issued.
func enterInsights(t *testing.T, m Model) (Model, insightsLoadedMsg) {
	t.Helper()
	next, cmd := m.handleKeyAction(keys.ActionSwitchInsights)
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKeyAction returned %T", next)
	}
	if cmd == nil {
		t.Fatal("entering Insights issued no load")
	}
	msg, ok := cmd().(insightsLoadedMsg)
	if !ok {
		t.Fatalf("expected insightsLoadedMsg, got %T", cmd())
	}
	return updated, msg
}

// TestInsightsLoadFailureDoesNotInstallPartialSnapshotAndCanRetry covers both
// halves of a failed read: the first one, which has no numbers to fall back on,
// and a refresh, which must not replace good numbers with a partial fold.
func TestInsightsLoadFailureDoesNotInstallPartialSnapshotAndCanRetry(t *testing.T) {
	dbDown := errors.New("database is locked")

	t.Run("an initial failure installs nothing and stays retryable", func(t *testing.T) {
		m, reader := newBrokenInsightsModel(t)
		reader.fail = dbDown

		m, msg := enterInsights(t, m)

		if msg.err == nil {
			t.Fatal("a failed read produced a load message with no error")
		}
		for _, want := range []string{"plays", "coverage"} {
			if !strings.Contains(msg.err.Error(), want) {
				t.Errorf("the error should name every failed reader: %q missing from %v", want, msg.err)
			}
		}
		if msg.snap.TotalPlays != 0 || msg.snap.Coverage.TotalTracks != 0 {
			t.Errorf("a failed read computed a snapshot anyway: %+v", msg.snap)
		}

		m = m.handleInsightsLoadedMsg(msg)

		if m.insights.Loaded {
			t.Error("a failed read marked the view loaded")
		}
		if m.insights.Loading {
			t.Error("a failed read left the view loading forever")
		}
		if m.insights.LoadErr == nil {
			t.Fatal("the failure was swallowed")
		}
		rendered := m.insights.Render(styles.NewStyles(false), nil, nil, nil, "", "")
		if !strings.Contains(rendered, "Could not read your listening history") {
			t.Errorf("expected a visible error state, got: %q", rendered)
		}
		// The active refresh rides the 1 Hz tick, so retrying there would turn a
		// broken database into a failed read every second.
		if m.insightsActiveRefreshCmd() != nil {
			t.Error("a failed read is being retried by the automatic refresh")
		}

		reader.fail = nil
		m, msg = enterInsights(t, m)
		m = m.handleInsightsLoadedMsg(msg)

		if !m.insights.Loaded || m.insights.LoadErr != nil {
			t.Fatalf("re-entering the view did not recover: loaded=%v err=%v", m.insights.Loaded, m.insights.LoadErr)
		}
		if m.insights.Snap.TotalPlays == 0 {
			t.Error("the retry installed no plays")
		}
	})

	t.Run("a failed refresh keeps the last complete snapshot", func(t *testing.T) {
		m, reader := newBrokenInsightsModel(t)
		m, msg := enterInsights(t, m)
		m = m.handleInsightsLoadedMsg(msg)
		before := m.insights.Snap.TotalPlays
		if before == 0 {
			t.Fatal("precondition: the first load should carry plays")
		}

		// The real play path: a row in play_history and one more play on the
		// in-memory track, then the message that invalidates the memo.
		now := time.Now().Unix()
		reader.tracks[0].PlayCount++
		reader.tracks[0].LastPlayedAt = now
		m.library.Index.IncrementPlayCount("h1", now)
		m = updateInsightsModel(t, m, playback.PlayRecordedMsg{Hash: "h1", PlayedAt: now})

		reader.fail = dbDown
		m = reloadInsights(t, m)

		if got := m.insights.Snap.TotalPlays; got != before {
			t.Errorf("a failed refresh replaced the good snapshot: want %d plays, got %d", before, got)
		}
		if !m.insights.Loaded {
			t.Error("a failed refresh threw away a snapshot the view already had")
		}
		rendered := m.insights.Render(styles.NewStyles(false), nil, nil, nil, "", "")
		if !strings.Contains(rendered, "retry · read failed") {
			t.Errorf("the strip should say the figures are not current, got: %q", rendered)
		}
		if m.insightsActiveRefreshCmd() != nil {
			t.Error("a failed refresh is being retried by the automatic refresh")
		}

		reader.fail = nil
		m, msg = enterInsights(t, m)
		m = m.handleInsightsLoadedMsg(msg)

		if got := m.insights.Snap.TotalPlays; got != before+1 {
			t.Errorf("the retry missed the play recorded during the outage: want %d, got %d", before+1, got)
		}
		if m.insights.LoadErr != nil || m.insights.Stale {
			t.Errorf("a recovered load left the view marked bad: err=%v stale=%v", m.insights.LoadErr, m.insights.Stale)
		}
	})
}

func TestInsightsWindowCycleFixedPanelDoesNotReload(t *testing.T) {
	m := newInsightsModel(t)
	if m.insights.Spec().Class != instui.ClassFixed {
		t.Fatal("precondition: Coverage should be a fixed panel")
	}
	before := m.insights.WindowIdx

	m, cmd := m.handleInsightsWindowCycle(1)

	if cmd != nil {
		t.Error("fixed panel should not trigger a reload")
	}
	if m.insights.WindowIdx != before {
		t.Errorf("fixed panel should not move the window: got %d, want %d", m.insights.WindowIdx, before)
	}
}

func TestInsightsWindowCycleWindowedPanelReloads(t *testing.T) {
	m := newInsightsModel(t)
	m = m.handleInsightsPanelCycle(1)
	if m.insights.Spec().Class != instui.ClassWindowed {
		t.Fatalf("precondition: Rhythm should be windowed, got %v", m.insights.Spec().Class)
	}

	m, cmd := m.handleInsightsWindowCycle(1)

	if cmd == nil {
		t.Fatal("windowed panel should trigger a reload")
	}
	if m.insights.WindowIdx != 1 {
		t.Errorf("window did not advance: got %d, want 1", m.insights.WindowIdx)
	}
}
