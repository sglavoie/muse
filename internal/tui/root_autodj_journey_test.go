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
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/autodj"
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
	j.m = New(context.Background(),
		WithLibraryTracks(j.tracks),
		WithQueueEntries(entries),
		WithBatchEnqueuer(j.w),
		WithQueueDequeuer(j.w),
		WithDJWriter(j.w),
		WithDJQueueWriter(j.w),
		WithDJRecipeReader(j.d),
		WithDJRecipeWriter(j.w),
		WithSessionWriter(j.w),
		WithRecommendationReader(j.d),
		WithTagReader(j.d),
		WithPlayer(p),
		WithRadioMode(radioMode),
		WithDJSession(session, nil),
	)
	j.m.playback.currentIndex = 0
	j.m.playback.currentHash = entries[0].Hash
	j.m.playback.state = playback.StatePlaying
}

// relaunch closes the launch and rebuilds the Model from what is on disk, which
// is exactly what restarting Muse does.
func (j *djJourney) relaunch() {
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
	session, err := j.d.ReadDJSession()
	if err != nil {
		j.t.Fatalf("read the restored DJ session: %v", err)
	}
	// Radio mode lives on the session row, not the DJ row, and a relaunch that
	// dropped it would restore a session the refill guard refuses to serve.
	row, err := j.d.ReadSession()
	if err != nil {
		j.t.Fatalf("read the restored session row: %v", err)
	}
	if row == nil {
		j.t.Fatal("the restored launch found no session row")
	}
	j.build(entries, session, row.RadioMode)
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

func TestAutoDJProductJourney(t *testing.T) {
	j := newDJJourney(t)
	playingHash := j.m.playback.currentHash
	var recipeName = "Late Night"

	t.Run("A starts a Balanced session and fills the queue", func(t *testing.T) {
		next, cmd, handled := j.m.radioToggle()
		if !handled || cmd == nil {
			t.Fatalf("A did not start a refill: handled=%v cmd=%v", handled, cmd == nil)
		}
		j.m = next.(Model)
		if !j.m.DJActive() {
			t.Fatal("A left no DJ session running")
		}
		if got := j.m.DJSettings().Preset(); got != radio.PresetBalanced {
			t.Fatalf("started at the %q preset, want Balanced", got)
		}
		if got := j.m.DJSeed().TrackHash; got != playingHash {
			t.Fatalf("seeded from %q, want the playing track %q", got, playingHash)
		}
		stored := j.session()
		if !stored.Active || stored.Seed.TrackHash != playingHash {
			t.Fatalf("persisted session = %+v, want an active session seeded from %q", stored, playingHash)
		}

		j.runBatch(cmd)

		owned := j.djOwnedHashes()
		if len(owned) != radioBatchSize {
			t.Fatalf("durable DJ-owned rows = %v, want a batch of %d", owned, radioBatchSize)
		}
		if got := j.visibleQueue()[0].Hash; got != playingHash {
			t.Fatalf("the first durable row is %q, want the manual row %q still in front", got, playingHash)
		}
		for _, row := range j.visibleQueue() {
			if row.DJOwned && len(row.DJReasons) == 0 {
				t.Errorf("DJ row %q was published with no reasons, so the queue cannot explain it", row.Hash)
			}
		}
	})

	t.Run("the Preview tab reads an Explore draft without changing anything", func(t *testing.T) {
		beforeQueue := j.djOwnedHashes()
		beforeSettings := j.session().Settings

		j.press("ctrl+a")
		panel := djPanel(t, j.m)
		if panel.Dirty() {
			t.Fatal("the panel opened dirty")
		}
		if got := panel.CommitLabel(); got != "Apply" {
			t.Fatalf("commit label = %q, want Apply with a session running", got)
		}

		// Preset row is focused first: one step right is Explore.
		j.press("right")
		if got := j.m.overlay.autoDJ.Draft().Preset(); got != radio.PresetExplore {
			t.Fatalf("draft preset = %q, want Explore", got)
		}
		draftCmd := j.press("tab")
		if !j.m.overlay.autoDJ.PreviewActive() {
			t.Fatal("tab did not reach the Preview tab")
		}
		if got := j.m.Snapshot().Overlay; got != "autodj_preview" {
			t.Fatalf("overlay snapshot = %q, want autodj_preview", got)
		}
		if draftCmd == nil {
			t.Fatal("arriving on the Preview tab asked for no draft plan")
		}
		j.deliver(draftCmd)
		preview := j.m.autoDJPanelPreview()
		if preview.Status != autodj.PreviewReady || len(preview.Tracks) == 0 {
			t.Fatalf("preview = %+v, want a ready draft batch", preview)
		}
		if !preview.Settings.Equal(j.m.overlay.autoDJ.Draft()) {
			t.Fatalf("preview settings = %+v, want the draft %+v", preview.Settings, j.m.overlay.autoDJ.Draft())
		}

		// Reading a plan is not accepting one.
		if got := j.djOwnedHashes(); len(got) != len(beforeQueue) {
			t.Errorf("the preview changed the durable queue: %v, want %v", got, beforeQueue)
		}
		if got := j.session().Settings; !got.Equal(beforeSettings) {
			t.Errorf("the preview changed the persisted settings to %+v", got)
		}
		if got := j.m.DJSettings().Preset(); got != radio.PresetBalanced {
			t.Fatalf("the applied preset moved to %q while only the draft was edited", got)
		}
	})

	t.Run("a genre filter applies during playback and refreshes in place", func(t *testing.T) {
		wantGenre := j.genreOf(playingHash)
		beforeCurrent := j.player.tracks[j.player.current].QueueID

		// Back to the Tune tab, down to "Include genres", and into the picker.
		j.press("shift+tab", "down", "down", "down", "down", "enter")
		// The picker is a search box: type the genre, take the top row, confirm.
		for _, r := range wantGenre {
			j.press(string(r))
		}
		j.press("tab", "enter")
		draft := j.m.overlay.autoDJ.Draft()
		if len(draft.Include.Genres) != 1 || draft.Include.Genres[0] != wantGenre {
			t.Fatalf("draft include genres = %v, want [%s]", draft.Include.Genres, wantGenre)
		}
		if !j.m.overlay.autoDJ.Dirty() {
			t.Fatal("a filtered draft is not reported dirty")
		}

		cmd := j.press("ctrl+s")
		if cmd == nil {
			t.Fatal("ctrl+s launched no refresh")
		}
		if !j.m.overlay.autoDJ.Busy() {
			t.Fatal("the panel is not busy while its commit is in flight")
		}
		msg := cmd()
		j.m = djDeliver(j.m, msg)

		if err := j.m.overlay.autoDJ.Err(); err != nil {
			t.Fatalf("the commit was refused: %v", err)
		}
		if j.m.overlay.autoDJ.Busy() {
			t.Fatal("the panel stayed busy after its acknowledgement")
		}
		applied := j.m.DJSettings()
		if len(applied.Include.Genres) != 1 || applied.Include.Genres[0] != wantGenre {
			t.Fatalf("applied settings = %+v, want the filtered draft", applied)
		}
		if got := applied.Preset(); got != radio.PresetExplore {
			t.Fatalf("applied preset = %q, want the Explore draft", got)
		}
		stored := j.session()
		if !stored.Settings.Equal(applied) {
			t.Fatalf("persisted settings = %+v, want the applied %+v", stored.Settings, applied)
		}
		if stored.Seed.TrackHash != playingHash {
			t.Errorf("applying settings moved the seed to %q", stored.Seed.TrackHash)
		}

		// Playback is where it was, and the refreshed rows respect the filter.
		if got := j.player.tracks[j.player.current].QueueID; got != beforeCurrent {
			t.Fatalf("the playing row changed from %d to %d across the Apply", beforeCurrent, got)
		}
		if got := j.visibleQueue()[0].Hash; got != playingHash {
			t.Fatalf("the manual row in front became %q", got)
		}
		owned := j.djOwnedHashes()
		if len(owned) == 0 {
			t.Fatal("the refresh left no DJ-owned rows")
		}
		for _, hash := range owned {
			if got := j.genreOf(hash); got != wantGenre {
				t.Errorf("refreshed row %q is %s, which the include filter excludes", hash, got)
			}
		}
	})

	t.Run("the tuned settings save as a recipe without touching playback", func(t *testing.T) {
		beforeQueue := j.djOwnedHashes()
		beforeCurrent := j.player.tracks[j.player.current].QueueID

		// Tune -> Preview -> Recipes, then save under a typed name.
		j.press("tab", "tab")
		if !j.m.overlay.autoDJ.RecipesActive() {
			t.Fatal("two tabs did not reach the Recipes tab")
		}
		if got := j.m.Snapshot().Overlay; got != "autodj_recipes" {
			t.Fatalf("overlay snapshot = %q, want autodj_recipes", got)
		}
		j.press("s")
		for _, key := range []string{"L", "a", "t", "e", "space", "N", "i", "g", "h", "t"} {
			j.press(key)
		}
		j.press("enter")

		if err := j.m.overlay.autoDJ.Err(); err != nil {
			t.Fatalf("saving the recipe failed: %v", err)
		}
		saved := j.recipes()
		if len(saved) != 1 {
			t.Fatalf("stored recipes = %+v, want exactly the one just saved", saved)
		}
		if saved[0].Recipe.Name != recipeName {
			t.Fatalf("saved recipe name = %q, want %q", saved[0].Recipe.Name, recipeName)
		}
		if !saved[0].Recipe.Settings.Equal(j.m.DJSettings()) {
			t.Fatalf("saved settings = %+v, want the applied %+v", saved[0].Recipe.Settings, j.m.DJSettings())
		}
		if got := j.djOwnedHashes(); len(got) != len(beforeQueue) {
			t.Errorf("saving a recipe changed the queue: %v, want %v", got, beforeQueue)
		}
		if got := j.player.tracks[j.player.current].QueueID; got != beforeCurrent {
			t.Errorf("saving a recipe moved playback to row %d", got)
		}
	})

	t.Run("a relaunch restores the session, the queue, and the recipe", func(t *testing.T) {
		beforeSettings := j.m.DJSettings()
		beforeSeed := j.m.DJSeed()
		beforeOwned := j.djOwnedHashes()

		j.relaunch()

		if !j.m.DJActive() {
			t.Fatal("the restored launch has no DJ session running")
		}
		if err := j.m.DJRestoreErr(); err != nil {
			t.Fatalf("restoring the session reported %v", err)
		}
		if got := j.m.DJSettings(); !got.Equal(beforeSettings) {
			t.Fatalf("restored settings = %+v, want %+v", got, beforeSettings)
		}
		if got := j.m.DJSeed(); got.TrackHash != beforeSeed.TrackHash || got.Label() != beforeSeed.Label() {
			t.Fatalf("restored seed = %+v, want the captured %+v", got, beforeSeed)
		}
		if got := j.djOwnedHashes(); len(got) != len(beforeOwned) {
			t.Fatalf("restored DJ-owned rows = %v, want %v", got, beforeOwned)
		}
		for i, row := range j.m.queue.Entries {
			if i == 0 {
				continue
			}
			if !row.DJOwned || len(row.DJReasons) == 0 {
				t.Errorf("restored row %q lost its provenance: owned=%v reasons=%v",
					row.Hash, row.DJOwned, row.DJReasons)
			}
		}
		if saved := j.recipes(); len(saved) != 1 || saved[0].Recipe.Name != recipeName {
			t.Fatalf("restored recipes = %+v, want the saved %q", saved, recipeName)
		}
	})

	t.Run("loading the recipe elsewhere edits the draft only", func(t *testing.T) {
		// Stop the session first: this is the "somewhere else" case — a user who
		// reopens the panel later, with nothing running, and loads a saved recipe.
		next, _, _ := j.m.radioToggle()
		j.m = next.(Model)
		if j.m.DJActive() {
			t.Fatal("the toggle left the session running")
		}
		stoppedSettings := j.m.DJSettings()
		beforeQueue := j.djOwnedHashes()

		j.press("ctrl+a", "tab", "tab")
		panel := djPanel(t, j.m)
		if !panel.RecipesActive() {
			t.Fatal("the Recipes tab is not open")
		}
		if got := panel.CommitLabel(); got != "Start" {
			t.Fatalf("commit label = %q, want Start with no session running", got)
		}
		j.press("enter")

		loaded := j.m.overlay.autoDJ.Draft()
		if !loaded.Equal(j.recipes()[0].Recipe.Settings) {
			t.Fatalf("loaded draft = %+v, want the recipe's settings", loaded)
		}
		if got := j.m.DJSettings(); !got.Equal(stoppedSettings) {
			t.Errorf("loading a recipe changed the applied settings to %+v", got)
		}
		if got := j.djOwnedHashes(); len(got) != len(beforeQueue) {
			t.Errorf("loading a recipe changed the queue: %v, want %v", got, beforeQueue)
		}
		if j.m.DJActive() {
			t.Error("loading a recipe started a session")
		}
		j.press("esc")
		if j.m.overlay.autoDJ != nil {
			t.Error("esc left the panel open")
		}
	})

	t.Run("reseeding from another track keeps the settings and resets the carry", func(t *testing.T) {
		settings := j.m.DJSettings()
		// The listener moves to a different row and turns the DJ back on, which
		// is the ordinary reseed gesture.
		last := len(j.m.queue.Entries) - 1
		j.m.playback.currentIndex = last
		j.m.playback.currentHash = j.m.queue.Entries[last].Hash
		j.player.current = last

		next, cmd, _ := j.m.radioToggle()
		j.m = next.(Model)
		if cmd == nil {
			t.Fatal("restarting the DJ launched no refill")
		}
		if got := j.m.DJSeed().TrackHash; got != j.m.queue.Entries[last].Hash {
			t.Fatalf("reseeded from %q, want the newly playing %q", got, j.m.queue.Entries[last].Hash)
		}
		if got := j.m.DJSettings(); !got.Equal(settings) {
			t.Fatalf("reseeding changed the settings to %+v, want the kept %+v", got, settings)
		}
		if got := j.m.DJAllocation(); !got.IsZero() {
			t.Errorf("reseeding kept the carry %+v from the previous seed", got)
		}
		stored := j.session()
		if !stored.Active || stored.Seed.TrackHash != j.m.DJSeed().TrackHash {
			t.Fatalf("persisted session = %+v, want the reseeded snapshot", stored)
		}
		if !stored.Settings.Equal(settings) {
			t.Errorf("persisted settings = %+v, want the kept %+v", stored.Settings, settings)
		}
	})
}
