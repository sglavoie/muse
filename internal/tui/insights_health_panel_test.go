package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/insights"
	instui "github.com/sglavoie/muse/internal/tui/insights"
	"github.com/sglavoie/muse/internal/tui/keys"
)

func TestHealthPanelReportsEveryFindingKind(t *testing.T) {
	m := newHealthModel(t)

	want := map[instui.HealthRowKind]string{
		instui.HealthGenreCollision:     instui.HealthSectionCollisions,
		instui.HealthPlaceholderGenre:   instui.HealthSectionPlaceholders,
		instui.HealthMissingMetadata:    instui.HealthSectionMissing,
		instui.HealthUnverifiedDuration: instui.HealthSectionDurations,
		instui.HealthArtistDuplicate:    instui.HealthSectionArtists,
		instui.HealthDeepCutsFlat:       instui.HealthSectionDeepCuts,
	}
	seen := map[instui.HealthRowKind]bool{}
	for _, row := range m.insights.Health {
		seen[row.Kind] = true
		if section := want[row.Kind]; section != row.Section {
			t.Errorf("row kind %d has section %q, want %q", row.Kind, row.Section, section)
		}
		if len(row.Hashes) == 0 {
			t.Errorf("row %q has no tracks behind it", row.Label)
		}
	}
	for kind := range want {
		if !seen[kind] {
			t.Errorf("health panel is missing a row of kind %d", kind)
		}
	}
}

// The count a row reports and the list it opens are the same set: a row whose
// note disagreed with its drill would be exactly the drift this panel exists to
// report.
func TestHealthRowNoteMatchesItsDrill(t *testing.T) {
	m := newHealthModel(t)
	for i, row := range m.insights.Health {
		drilled := m.insightsHealthTracks(strconv.Itoa(i))
		if len(drilled) != len(row.Hashes) {
			t.Errorf("row %q notes %d tracks but drills to %d", row.Label, len(row.Hashes), len(drilled))
		}
	}
}

func TestHealthCollisionRowNamesEverySpelling(t *testing.T) {
	m := newHealthModel(t)
	row := m.insights.Health[healthRowIndex(t, m, instui.HealthGenreCollision)]

	for _, variant := range []string{"AMBIENT", "Ambient", "ambient"} {
		if !strings.Contains(row.Label, variant) {
			t.Errorf("collision label %q omits variant %q", row.Label, variant)
		}
	}
	if row.Canonical != "Ambient" {
		t.Errorf("canonical = %q, want the most-played spelling %q", row.Canonical, "Ambient")
	}
	if len(row.Hashes) != 3 {
		t.Errorf("collision covers %d tracks, want 3", len(row.Hashes))
	}
}

// A merge only writes once the user confirms, and it clears the Rhythm banner
// the collision was raising.
func TestMergeGenreConfirmClearsCollisionAndBanner(t *testing.T) {
	m := newHealthModel(t)
	m.insights.Cursor = healthRowIndex(t, m, instui.HealthGenreCollision)

	if !m.insights.Snap.Rhythm.GenresUnclean {
		t.Fatal("fixture should start with an unclean-genre banner")
	}

	m = m.handleInsightsMergeGenre()
	if m.insights.PendingMerge == nil {
		t.Fatal("m should arm a pending merge")
	}
	if !strings.Contains(m.confirmationBar, "Ambient") {
		t.Errorf("confirmation bar = %q, want it to name the canonical spelling", m.confirmationBar)
	}

	m, msg := runMergeWrite(t, m)
	if m.confirmationBar != "" || m.insights.PendingMerge != nil {
		t.Error("confirming should clear the bar and the plan")
	}
	if msg.err != nil || msg.failed != 0 {
		t.Fatalf("merge reported err %v and %d failed writes", msg.err, msg.failed)
	}
	m = m.applyMergeReply(t, msg)

	if m.insights.Snap.Rhythm.GenresUnclean {
		t.Error("banner should be gone once every spelling agrees")
	}
	for _, row := range m.insights.Health {
		if row.Kind == instui.HealthGenreCollision {
			t.Errorf("collision row %q survived the merge", row.Label)
		}
	}
}

// A merge that dies halfway must not leave memory claiming a spelling the
// database refused: the lost rows keep their old genre, the collision stays
// reported, and pressing m again writes exactly what the first attempt lost
// instead of reporting that the genres already agree.
func TestMergeGenreWriteFailureKeepsMemoryAndDatabaseRetryable(t *testing.T) {
	m := newHealthModel(t)
	m.insights.Cursor = healthRowIndex(t, m, instui.HealthGenreCollision)
	row := m.insights.Health[m.insights.Cursor]

	before := make(map[string]string, len(row.Hashes))
	for _, hash := range row.Hashes {
		tr, ok := m.library.Index.FindByHash(hash)
		if !ok {
			t.Fatalf("collision track %q is not in the library index", hash)
		}
		before[hash] = tr.Genre
	}

	flaky := &flakyMetadataWriter{inner: m.metadataWriter, succeed: 1}
	m.metadataWriter = flaky

	m = m.handleInsightsMergeGenre()
	m, msg := runMergeWrite(t, m)

	if msg.err == nil {
		t.Fatal("the injected write failure should be reported")
	}
	if len(msg.applied) != 1 || msg.failed != 1 {
		t.Fatalf("reply applied %d rows and lost %d, want 1 and 1", len(msg.applied), msg.failed)
	}
	persisted := msg.applied[0].hash

	m = m.applyMergeReply(t, msg)

	for hash, was := range before {
		tr, _ := m.library.Index.FindByHash(hash)
		if hash == persisted {
			if tr.Genre != row.Canonical {
				t.Errorf("track %q persisted but reads %q in memory, want %q", hash, tr.Genre, row.Canonical)
			}
			continue
		}
		if tr.Genre != was {
			t.Errorf("track %q was never written but memory moved from %q to %q", hash, was, tr.Genre)
		}
	}

	if !m.insights.Snap.Rhythm.GenresUnclean {
		t.Error("the banner should survive a merge the database refused")
	}
	action, ok := m.undo.slot.action.(undoBulkEditMetadataAction)
	if !ok {
		t.Fatalf("undo slot holds %T, want a bulk metadata edit", m.undo.slot.action)
	}
	if len(action.originals) != 1 {
		t.Errorf("undo covers %d tracks, want only the 1 that persisted", len(action.originals))
	}
	if _, covered := action.originals[persisted]; !covered {
		t.Errorf("undo does not cover %q, the row that actually persisted", persisted)
	}

	// The retry: the collision is still there to focus, and confirming it
	// writes the row the first attempt lost.
	m.metadataWriter = flaky.inner
	m.insights.Cursor = healthRowIndex(t, m, instui.HealthGenreCollision)
	m = m.handleInsightsMergeGenre()
	m, retry := runMergeWrite(t, m)

	if retry.err != nil || len(retry.applied) != 1 || retry.failed != 0 {
		t.Fatalf("retry applied %d rows and lost %d (err %v), want the 1 row the first merge lost",
			len(retry.applied), retry.failed, retry.err)
	}
	if retry.applied[0].hash == persisted {
		t.Errorf("retry rewrote %q, which had already persisted", persisted)
	}

	m = m.applyMergeReply(t, retry)
	if m.insights.Snap.Rhythm.GenresUnclean {
		t.Error("banner should clear once the retry lands the last row")
	}
	for _, r := range m.insights.Health {
		if r.Kind == instui.HealthGenreCollision {
			t.Errorf("collision row %q survived the retry", r.Label)
		}
	}
}

func TestMergeGenreCancelsOnAnyOtherKey(t *testing.T) {
	m := newHealthModel(t)
	m.insights.Cursor = healthRowIndex(t, m, instui.HealthGenreCollision)
	m = m.handleInsightsMergeGenre()

	m, cmd, handled := m.handleInsightsMergeKeyPress(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !handled || cmd != nil {
		t.Fatalf("n should be consumed without writing, got handled=%v cmd=%v", handled, cmd)
	}
	if m.insights.PendingMerge != nil || m.confirmationBar != "" {
		t.Error("canceling should clear both the plan and the bar")
	}
}

func TestMergeGenreRefusesNonCollisionRows(t *testing.T) {
	m := newHealthModel(t)
	m.insights.Cursor = healthRowIndex(t, m, instui.HealthMissingMetadata)

	if got := m.handleInsightsMergeGenre(); got.insights.PendingMerge != nil {
		t.Error("only a collision row should arm a merge")
	}
}

// The bulk editor opens over everything the finding counted, not over one
// focused track: a Data Health row names a set.
func TestEditMetadataOnHealthRowOpensBulkEditor(t *testing.T) {
	m := newHealthModel(t)
	i := healthRowIndex(t, m, instui.HealthGenreCollision)
	m.insights.Cursor = i

	m, _, handled := m.handleMetadataKeyAction(keys.ActionEditMetadata)
	if !handled {
		t.Fatal("M should be consumed on a health row")
	}
	if m.overlay.metadataEditor == nil {
		t.Fatal("M should open the metadata editor")
	}
	if got := len(m.overlay.metadataEditor.Hashes()); got != len(m.insights.Health[i].Hashes) {
		t.Errorf("editor covers %d tracks, want the row's %d", got, len(m.insights.Health[i].Hashes))
	}
}

func TestMergeGenreVariantsKeepsUnrelatedGenres(t *testing.T) {
	fold := map[string]struct{}{instui.GenreKey("ambient"): {}, instui.GenreKey("AMBIENT"): {}}

	// SplitGenres sorts, so the surviving order is its order, not the tag's.
	got := mergeGenreVariants("rock, AMBIENT", fold, "Ambient")
	if got != "Ambient, rock" {
		t.Errorf("mergeGenreVariants = %q, want %q", got, "Ambient, rock")
	}
	// Folding two variants of one genre onto a track must not leave it tagged
	// with the canonical spelling twice.
	if got := mergeGenreVariants("ambient, AMBIENT", fold, "Ambient"); got != "Ambient" {
		t.Errorf("mergeGenreVariants = %q, want %q", got, "Ambient")
	}
}

func TestIsPlaceholderGenreDrivesItsOwnRow(t *testing.T) {
	m := newHealthModel(t)
	row := m.insights.Health[healthRowIndex(t, m, instui.HealthPlaceholderGenre)]

	if !insights.IsPlaceholderGenre(row.Label) {
		t.Errorf("placeholder row labeled %q, which is not a placeholder genre", row.Label)
	}
}

// Data Health puts six section headers over nine rows, which is what exposed
// scrolling that budgeted rows but rendered headers too: the cursor could land
// below the last line the panel had room to draw.
func TestHealthCursorStaysOnScreenWhenSectionsCrowdThePanel(t *testing.T) {
	m := newHealthModel(t)
	m.insights.Cursor = len(m.insights.Health) - 1
	m.insights.ClampCursor()

	v := m.insights
	if got := instui.PanelBodyLines(v.Rows(), v.ScrollOffset, v.Cursor); got > v.BodyRows() {
		t.Errorf("cursor needs %d lines but only %d fit; scrollOffset %d is too high up",
			got, v.BodyRows(), v.ScrollOffset)
	}
}

func TestHealthArtistKeyFoldsArticlesAndAmpersands(t *testing.T) {
	same := [][2]string{
		{"Alice", "The Alice"},
		{"Bob & Co.", "Bob and Co"},
		{"  Deep   Blue ", "deepblue"},
	}
	for _, pair := range same {
		if instui.HealthArtistKey(pair[0]) != instui.HealthArtistKey(pair[1]) {
			t.Errorf("%q and %q should fold together, got %q vs %q",
				pair[0], pair[1], instui.HealthArtistKey(pair[0]), instui.HealthArtistKey(pair[1]))
		}
	}
	// A misspelling is left alone: the merge rewrites tags on disk, so a false
	// positive costs more than a missed duplicate.
	if instui.HealthArtistKey("Alice") == instui.HealthArtistKey("Alise") {
		t.Error("misspellings should not fold together")
	}
}
