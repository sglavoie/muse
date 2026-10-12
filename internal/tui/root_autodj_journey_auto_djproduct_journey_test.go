package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/autodj"
)

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
