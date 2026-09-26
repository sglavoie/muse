package app

import (
	"errors"
	"slices"
	"testing"
)

type recordingPurger struct {
	calls  []string
	before []int64
	fail   map[string]bool
}

func (p *recordingPurger) record(table string, before int64) error {
	p.calls = append(p.calls, table)
	p.before = append(p.before, before)
	if p.fail[table] {
		return errors.New("purge " + table + " failed")
	}
	return nil
}

func (p *recordingPurger) PurgeDeletedTags(b int64) error      { return p.record("tags", b) }
func (p *recordingPurger) PurgeDeletedPlaylists(b int64) error { return p.record("playlists", b) }
func (p *recordingPurger) PurgeDeletedPlaylistTracks(b int64) error {
	return p.record("playlist_tracks", b)
}
func (p *recordingPurger) PurgeDeletedQueueEntries(b int64) error { return p.record("queue", b) }
func (p *recordingPurger) PurgeDeletedRecipes(b int64) error      { return p.record("dj_recipes", b) }

// TestPurgeStaleSoftDeletesContinuesPastFailures checks the sweep is
// best-effort: one table failing must neither stop the others nor surface as
// an error that could block startup.
func TestPurgeStaleSoftDeletesContinuesPastFailures(t *testing.T) {
	p := &recordingPurger{fail: map[string]bool{"tags": true, "queue": true}}

	purgeStaleSoftDeletes(p, 1234)

	want := []string{"tags", "playlist_tracks", "playlists", "queue", "dj_recipes"}
	if !slices.Equal(p.calls, want) {
		t.Errorf("purged tables = %v, want %v", p.calls, want)
	}
	for i, b := range p.before {
		if b != 1234 {
			t.Errorf("%s purged with cutoff %d, want 1234", p.calls[i], b)
		}
	}
}
