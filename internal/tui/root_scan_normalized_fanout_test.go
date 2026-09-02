package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestScanBatchReconcilesCachesWithNormalizedMetadata pins FR-004: the scan
// handler must fan the *normalized* track slice (user-edited overlay + preserved
// non-zero duration) out to every cache reconciliation target, not the raw
// scanned tracks. Otherwise the index and the queue/collection caches diverge:
// the index would carry the edited title and preserved duration while the caches
// kept the raw scanner values.
func TestScanBatchReconcilesCachesWithNormalizedMetadata(t *testing.T) {
	sp := &stubPlayerQueueSyncer{}
	m := New(context.Background(), WithPlayer(sp))

	// Existing index entry carries the play stats and a real duration that the
	// scanner (which reports Duration == 0 here) must not clobber.
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.mp3", Title: "Old", Artist: "Old",
		CreatedAt: 1000, Duration: 200.0,
	})

	// User manually edited this track's tags; the overlay must win over the raw
	// scanned values in every cache, exactly as it does in the index.
	m.userEditedOverlay = map[string]db.UserEditedRow{
		"h1": {Hash: "h1", Title: "Edited Title", Artist: "Edited Artist", Album: "Edited Album", Genre: "Edited Genre"},
	}

	// Queue and a collection (mixes) cache hold the pre-scan raw values.
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/a.mp3", Title: "Old", Artist: "Old", DurationMs: 0},
	}
	m.mixes.cachedMixes = map[db.MixID][]library.Track{
		db.MixRediscover: {{Hash: "h1", Path: "/a.mp3", Title: "Old", Artist: "Old", Duration: 0}},
	}

	// Scanner reports raw title/artist and a zero duration (no probe).
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Hash: "h1", Path: "/a.mp3", Title: "Scanned Title", Artist: "Scanned Artist", Duration: 0},
		},
	})
	nm := next.(Model)

	// Index carries the normalized values (sanity check on the reference).
	idx, ok := nm.library.Index.FindByHash("h1")
	if !ok {
		t.Fatal("h1 not found in index")
	}
	if idx.Title != "Edited Title" || idx.Duration != 200.0 {
		t.Fatalf("index normalization = %q/%v; want Edited Title/200", idx.Title, idx.Duration)
	}

	// Queue cache must match the index, not the raw scan.
	qe := nm.queue.Entries[0]
	if qe.Title != "Edited Title" {
		t.Errorf("queue Title = %q; want Edited Title (user-edited overlay)", qe.Title)
	}
	if qe.Artist != "Edited Artist" {
		t.Errorf("queue Artist = %q; want Edited Artist (user-edited overlay)", qe.Artist)
	}
	if qe.DurationMs != 200000 {
		t.Errorf("queue DurationMs = %d; want 200000 (preserved non-zero duration)", qe.DurationMs)
	}

	// Collection (mixes) cache must match too.
	cached := nm.mixes.cachedMixes[db.MixRediscover][0]
	if cached.Title != "Edited Title" {
		t.Errorf("mix cached Title = %q; want Edited Title", cached.Title)
	}
	if cached.Duration != 200.0 {
		t.Errorf("mix cached Duration = %v; want 200 (preserved non-zero duration)", cached.Duration)
	}

	// The player queue must receive the same normalized display metadata.
	if len(sp.setQueueCalls) == 0 {
		t.Fatal("expected SetQueue to be called when the queue metadata changes")
	}
	last := sp.setQueueCalls[len(sp.setQueueCalls)-1]
	if len(last) != 1 {
		t.Fatalf("player queue length = %d; want 1", len(last))
	}
	if last[0].Path != "/a.mp3" {
		t.Errorf("player queue Path = %q; want /a.mp3", last[0].Path)
	}
	if last[0].Title != "Edited Title" || last[0].Artist != "Edited Artist" {
		t.Errorf("player queue metadata = %q/%q; want Edited Title/Edited Artist", last[0].Title, last[0].Artist)
	}
}
