package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// The whole product journey, in the order a user performs it, over a real
// database file.
//
// Every other Auto-DJ test isolates one seam. This one deliberately does not: it
// starts the DJ with the `A` key, reads the draft plan on the Preview tab, builds
// a filter through the picker, applies it while a track is playing, saves the
// result as a recipe, relaunches Muse over the same file, loads the recipe
// somewhere else, and reseeds. What it is checking is the joins — that the
// settings the panel committed are the ones storage restored, that the recipe the
// Recipes tab saved is loadable by a launch that never saw it, and that none of
// those steps disturbed the row that was playing.
//
// It uses the real writer and the real queue tables because the interesting
// failures live there: a Model-only journey would pass with provenance, pending
// markers, and session snapshots that never reached disk.

// djJourney is one user's session, carried from step to step.
type djJourney struct {
	t      *testing.T
	path   string
	d      *db.DB
	w      *db.Writer
	cancel context.CancelFunc
	m      Model
	player *refreshPlayerStub
	tracks []library.Track
	// restoreErr is the DJ state error the next build restores with, the way
	// app.ApplyRestoredSession hands ReadDJSession's error to the Model. It
	// stays nil for this journey, whose launches all read cleanly; the
	// refused-restore tests set it before rebuilding.
	restoreErr error
	// djWriter is the session writer every launch is built with when it is set,
	// so a test can put a failure in front of the real writer and keep it there
	// across a relaunch. Nil means the real writer, which is what a journey that
	// is not exercising write failures wants.
	djWriter djSessionWriter
}

// djJourneyTracks is a library with both pools populated and three genres, so a
// genre filter narrows the candidates without emptying them.
func djJourneyTracks() []library.Track {
	genres := []string{"Shoegaze", "Jazz", "Folk"}
	recent := time.Now().AddDate(0, 0, -3).Unix()
	tracks := make([]library.Track, 0, 18)
	for i := range 18 {
		hash := fmt.Sprintf("jt%02d", i)
		t := library.Track{
			Hash:     hash,
			Path:     "/music/" + hash + ".flac",
			Artist:   fmt.Sprintf("Artist %02d", i%9),
			Title:    fmt.Sprintf("Title %02d", i),
			Album:    fmt.Sprintf("Album %02d", i%6),
			Genre:    genres[i%len(genres)],
			Duration: 190 + float64(i),
		}
		if i%2 == 0 {
			t.PlayCount = 5
			t.LastPlayedAt = recent
		}
		tracks = append(tracks, t)
	}
	return tracks
}

// newDJJourney opens a database file, seeds the library and one manual queue row,
// and builds the Model the way main_tui.go wires it.
func newDJJourney(t *testing.T) *djJourney {
	t.Helper()
	j := &djJourney{t: t, path: filepath.Join(t.TempDir(), "journey.db"), tracks: djJourneyTracks()}
	j.open()

	now := time.Now().Unix()
	for _, track := range j.tracks {
		if _, err := j.d.Exec(
			`INSERT INTO tracks (hash, path, artist, title, album, genre, duration_ms, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			track.Hash, track.Path, track.Artist, track.Title, track.Album, track.Genre,
			int64(track.Duration*1000), now, now,
		); err != nil {
			t.Fatalf("seed track %s: %v", track.Hash, err)
		}
	}
	ids, err := j.w.AppendTracks(j.tracks[:1], "Manual")
	if err != nil {
		t.Fatalf("seed the queue: %v", err)
	}
	j.build(newQueueEntries(j.tracks[:1], ids, "Manual"), db.DJSession{}, false)
	return j
}

// open starts a launch over the journey's file.
func (j *djJourney) open() {
	j.t.Helper()
	d, err := db.Open(j.path)
	if err != nil {
		j.t.Fatalf("open %s: %v", j.path, err)
	}
	if err := db.Migrate(d.DB); err != nil {
		d.Close()
		j.t.Fatalf("migrate %s: %v", j.path, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	j.d, j.w, j.cancel = d, db.NewWriter(ctx, d.DB), cancel
	j.t.Cleanup(func() { j.close() })
}

// close ends the current launch. It is idempotent.
func (j *djJourney) close() {
	if j.d == nil {
		return
	}
	j.cancel()
	j.w.Wait()
	_ = j.d.Close()
	j.d = nil
}

// build assembles the Model over the current launch, restoring the given session
// state the way app.ApplyRestoredSession does.
func (j *djJourney) build(entries []queue.Entry, session db.DJSession, radioMode bool) {
	j.t.Helper()
	p := &refreshPlayerStub{current: 0, forward: map[int64]bool{}, unscheduled: map[int64]bool{}}
	for _, e := range entries {
		p.tracks = append(p.tracks, queueTrackFromEntry(e))
	}
	j.player = p
	sessions := djSessionWriter(j.w)
	if j.djWriter != nil {
		sessions = j.djWriter
	}
	j.m = New(context.Background(),
		WithLibraryTracks(j.tracks),
		WithQueueEntries(entries),
		WithBatchEnqueuer(j.w),
		WithQueueDequeuer(j.w),
		WithDJWriter(sessions),
		WithDJQueueWriter(j.w),
		WithDJRecipeReader(j.d),
		WithDJRecipeWriter(j.w),
		WithSessionWriter(j.w),
		WithRecommendationReader(j.d),
		WithTagReader(j.d),
		WithTagWriter(j.w),
		WithPlayer(p),
		WithRadioMode(radioMode),
		WithDJSession(session, j.restoreErr),
	)
	j.m.playback.currentIndex = 0
	j.m.playback.currentHash = entries[0].Hash
	j.m.playback.state = playback.StatePlaying
}

// relaunch closes the launch and rebuilds the Model from what is on disk, which
// is exactly what restarting Muse does.
func (j *djJourney) relaunch() {
	j.t.Helper()
	if err := j.relaunchRestoring(); err != nil {
		j.t.Fatalf("read the restored DJ session: %v", err)
	}
}

// relaunchRestoring is relaunch for launches that expect to meet saved DJ state
// this build refuses: it restores the refusal instead of failing on it, exactly
// as app.ApplyRestoredSession does, and returns it so the caller can assert on
// it. Everything else about the launch is unchanged.
func (j *djJourney) relaunchRestoring() error {
	j.t.Helper()
	j.close()
	j.open()
	rows, err := j.d.ReadQueue()
	if err != nil {
		j.t.Fatalf("read the restored queue: %v", err)
	}
	entries := make([]queue.Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, queue.Entry{
			ID: row.ID, Hash: row.Hash, Path: row.Path, Artist: row.Artist, Title: row.Title,
			SourcePlaylist: row.SourcePlaylist, DJOwned: row.DJOwned, DJReasons: row.DJReasons,
		})
	}
	session, restoreErr := j.d.ReadDJSession()
	// Radio mode lives on the session row, not the DJ row, and a relaunch that
	// dropped it would restore a session the refill guard refuses to serve.
	row, err := j.d.ReadSession()
	if err != nil {
		j.t.Fatalf("read the restored session row: %v", err)
	}
	if row == nil {
		j.t.Fatal("the restored launch found no session row")
	}
	j.restoreErr = restoreErr
	j.build(entries, session, row.RadioMode)
	return restoreErr
}

// deliver runs a command the runtime would have run and delivers its message.
func (j *djJourney) deliver(cmd tea.Cmd) {
	j.t.Helper()
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		j.m = djDeliver(j.m, msg)
	}
}

// press sends keys through the whole update loop.
func (j *djJourney) press(keys ...string) tea.Cmd {
	j.t.Helper()
	m, cmd := djPress(j.t, j.m, keys...)
	j.m = m
	return cmd
}

// runBatch runs a refill command the way the runtime does and delivers its
// result, so the batch reaches both the queue and the database.
func (j *djJourney) runBatch(cmd tea.Cmd) {
	j.t.Helper()
	if cmd == nil {
		j.t.Fatal("there was no refill command to run")
	}
	msg, ok := cmd().(RadioBatchEnqueuedMsg)
	if !ok {
		j.t.Fatal("the refill command did not produce a batch message")
	}
	if msg.Err != nil {
		j.t.Fatalf("planning the batch failed: %v", msg.Err)
	}
	if len(msg.Entries) == 0 {
		j.t.Fatal("the refill produced no batch, so the assertions below prove nothing")
	}
	m, _ := j.m.handleRadioBatchEnqueuedMsg(msg)
	j.m = m
}

// session reads the persisted DJ session.
func (j *djJourney) session() db.DJSession {
	j.t.Helper()
	s, err := j.d.ReadDJSession()
	if err != nil {
		j.t.Fatalf("ReadDJSession: %v", err)
	}
	return s
}

// visibleQueue reads the durable queue.
func (j *djJourney) visibleQueue() []db.QueueRow {
	j.t.Helper()
	rows, err := j.d.ReadQueue()
	if err != nil {
		j.t.Fatalf("ReadQueue: %v", err)
	}
	return rows
}

// recipes reads the saved recipes.
func (j *djJourney) recipes() []db.DJRecipe {
	j.t.Helper()
	rows, err := j.d.ReadRecipes()
	if err != nil {
		j.t.Fatalf("ReadRecipes: %v", err)
	}
	return rows
}

// djOwnedHashes returns the hashes of the durable DJ-owned rows, in queue order.
func (j *djJourney) djOwnedHashes() []string {
	j.t.Helper()
	var out []string
	for _, row := range j.visibleQueue() {
		if row.DJOwned {
			out = append(out, row.Hash)
		}
	}
	return out
}

// genreOf returns a track's genre, by hash.
func (j *djJourney) genreOf(hash string) string {
	for _, t := range j.tracks {
		if t.Hash == hash {
			return t.Genre
		}
	}
	return ""
}
