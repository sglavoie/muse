package tui

import (
	"errors"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newHealthModel returns a Model sitting on the loaded Data Health panel of the
// dirty health fixture, with the merge writer wired.
func newHealthModel(t *testing.T) Model {
	t.Helper()
	m, err := BuildFixture(FixtureHealth)
	if err != nil {
		t.Fatalf("BuildFixture(health): %v", err)
	}
	m.termWidth, m.termHeight = 100, 30
	m.insights.setSize(100, 30)
	m = m.switchToView(viewInsights)
	m = m.loadInsights(t)
	m.insights.panel = panelDataHealth
	return m
}

// loadInsights runs the pending load command to completion and installs it.
func (m Model) loadInsights(t *testing.T) Model {
	t.Helper()
	cmd := m.insightsLoadCmd()
	if cmd == nil {
		t.Fatal("expected an insights load cmd")
	}
	msg, ok := cmd().(insightsLoadedMsg)
	if !ok {
		t.Fatalf("expected insightsLoadedMsg, got %T", cmd())
	}
	return m.handleInsightsLoadedMsg(msg)
}

// runMergeWrite confirms an armed merge and runs the write command, returning
// the reply it produced.
func runMergeWrite(t *testing.T, m Model) (Model, insightsMergedMsg) {
	t.Helper()
	m, cmd, handled := m.handleInsightsMergeKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !handled {
		t.Fatal("y should be consumed while a merge is pending")
	}
	if cmd == nil {
		t.Fatal("confirming should return the write cmd")
	}
	msg, ok := cmd().(insightsMergedMsg)
	if !ok {
		t.Fatalf("merge cmd returned %T, want insightsMergedMsg", cmd())
	}
	return m, msg
}

// applyMergeReply routes the write reply the way Update does and runs the
// reload it asks for. This is where a merge reaches memory, so a test that
// skips it sees only what the database accepted.
func (m Model) applyMergeReply(t *testing.T, msg insightsMergedMsg) Model {
	t.Helper()
	m, cmd := m.handleInsightsMergedMsg(msg)
	if cmd == nil {
		t.Fatal("a merge reply should reload the panels")
	}
	loaded, ok := cmd().(insightsLoadedMsg)
	if !ok {
		t.Fatalf("merge reload returned %T, want insightsLoadedMsg", cmd())
	}
	return m.handleInsightsLoadedMsg(loaded)
}

// flakyMetadataWriter refuses every write past the first succeed, so a merge
// can be interrupted after at least one track really landed in the database.
type flakyMetadataWriter struct {
	inner   metadataUpdater
	succeed int
	calls   int
}

func (w *flakyMetadataWriter) UpdateTrackMetadata(hash, title, artist, album, genre string) error {
	w.calls++
	if w.calls > w.succeed {
		return errors.New("database is locked")
	}
	return w.inner.UpdateTrackMetadata(hash, title, artist, album, genre)
}

// mergeRaceDB is the test's database. It guards its rows so -race can see the
// merge write and the edit write for what they are, and it parks the first
// write until released, which is what holds the merge genuinely in flight while
// the edit is handled.
type mergeRaceDB struct {
	inner metadataUpdater

	parkOnce sync.Once
	parked   chan string   // carries the hash of the write that parked
	release  chan struct{} // closed to let the parked write through

	mu   sync.Mutex
	rows map[string][4]string // hash → title, artist, album, genre as persisted
}

func newMergeRaceDB(inner metadataUpdater) *mergeRaceDB {
	return &mergeRaceDB{
		inner:   inner,
		parked:  make(chan string, 1),
		release: make(chan struct{}),
		rows:    map[string][4]string{},
	}
}

func (d *mergeRaceDB) UpdateTrackMetadata(hash, title, artist, album, genre string) error {
	d.parkOnce.Do(func() {
		d.parked <- hash
		<-d.release
	})
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rows[hash] = [4]string{title, artist, album, genre}
	return d.inner.UpdateTrackMetadata(hash, title, artist, album, genre)
}

func (d *mergeRaceDB) row(hash string) [4]string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.rows[hash]
}

// A merge plans a full metadata row at confirmation time and persists it later,
// so an ordinary edit of one of its tracks can be handled while it is in
// flight. The edit is the newer intent: it must be what the database and the
// library index both end up holding, whether it arrives before the merge has
// reached that row or after the merge has already written it.
func TestGenreMergeConcurrentMetadataEditPreservesLatestValues(t *testing.T) {
	edited := [4]string{"Edited Title", "Edited Artist", "Edited Album", "Edited Genre"}

	for _, tc := range []struct {
		name string
		// editParked edits the very row the merge is blocked inside, so the
		// merge's write really does land first and is then superseded.
		editParked bool
	}{
		{"edit arrives before the merge reaches the row", false},
		{"edit arrives after the merge wrote the row", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newHealthModel(t)
			m.insights.cursor = healthRowIndex(t, m, healthGenreCollision)
			row := m.insights.health[m.insights.cursor]

			var targets []string
			for _, hash := range row.hashes {
				tr, ok := m.library.index.FindByHash(hash)
				if !ok {
					t.Fatalf("collision track %q is not in the library index", hash)
				}
				if tr.Genre != row.canonical {
					targets = append(targets, hash)
				}
			}
			if len(targets) != 2 {
				t.Fatalf("the merge should have 2 rows to write, got %d", len(targets))
			}

			store := newMergeRaceDB(m.metadataWriter)
			m.metadataWriter = store

			m = m.handleInsightsMergeGenre()
			m, mergeCmd, handled := m.handleInsightsMergeKeyPress(tea.KeyPressMsg{Code: 'y', Text: "y"})
			if !handled || mergeCmd == nil {
				t.Fatalf("confirming should return the write cmd, got handled=%v cmd=%v", handled, mergeCmd)
			}
			replies := make(chan tea.Msg, 1)
			go func() { replies <- mergeCmd() }()

			parked := <-store.parked
			target := parked
			if !tc.editParked {
				for _, hash := range targets {
					if hash != parked {
						target = hash
					}
				}
			}
			untouched := targets[0]
			if untouched == target {
				untouched = targets[1]
			}

			was, _ := m.library.index.FindByHash(target)
			ed := NewMetadataEditor(target, edited[0], edited[1], edited[2], edited[3], was.Duration)
			m.overlay.metadataEditor = &ed
			saved, editCmd := m.handleMetadataSave()
			m = saved.(Model)
			if editCmd == nil {
				t.Fatal("saving metadata should return the write cmd")
			}
			edits := make(chan tea.Msg, 1)
			go func() { edits <- editCmd() }()

			close(store.release)
			reply, ok := (<-replies).(insightsMergedMsg)
			if !ok {
				t.Fatalf("merge cmd returned %T, want insightsMergedMsg", reply)
			}
			<-edits

			m = m.applyMergeReply(t, reply)

			tr, ok := m.library.index.FindByHash(target)
			if !ok {
				t.Fatalf("edited track %q left the library index", target)
			}
			if got := [4]string{tr.Title, tr.Artist, tr.Album, tr.Genre}; got != edited {
				t.Errorf("library index holds %v for the edited track, want %v", got, edited)
			}
			if got := store.row(target); got != edited {
				t.Errorf("database holds %v for the edited track, want %v", got, edited)
			}

			// The merge still has to do its job on the row nobody touched.
			other, ok := m.library.index.FindByHash(untouched)
			if !ok {
				t.Fatalf("collision track %q left the library index", untouched)
			}
			if other.Genre != row.canonical {
				t.Errorf("untouched row reads %q in memory, want the merged %q", other.Genre, row.canonical)
			}
			if got := store.row(untouched)[3]; got != row.canonical {
				t.Errorf("untouched row reads %q in the database, want the merged %q", got, row.canonical)
			}

			// Undo must not carry the pre-edit row either: restoring it would
			// revert the edit one keypress later.
			action, ok := m.undo.slot.action.(undoBulkEditMetadataAction)
			if !ok {
				t.Fatalf("undo slot holds %T, want a bulk metadata edit", m.undo.slot.action)
			}
			if _, covered := action.originals[target]; covered {
				t.Errorf("undo covers %q, which the merge never applied", target)
			}
		})
	}
}

func healthRowIndex(t *testing.T, m Model, kind healthRowKind) int {
	t.Helper()
	for i, row := range m.insights.health {
		if row.kind == kind {
			return i
		}
	}
	t.Fatalf("no health row of kind %d in %d rows", kind, len(m.insights.health))
	return -1
}
