package insights

import (
	"reflect"
	"testing"
)

// TestFoldGenresCollapsesCaseAndSpacingVariants pins the collision found in
// the real library: "Ambient" and "ambient" are counted as two separate genres,
// which splits the genre chart in half.
func TestFoldGenresCollapsesCaseAndSpacingVariants(t *testing.T) {
	groups := foldGenres([]GenreRow{
		{Genre: "Ambient", Plays: 81, ListenedSecs: 8100, Tracks: 20},
		{Genre: "ambient", Plays: 61, ListenedSecs: 6100, Tracks: 15},
		{Genre: "AMBIENT ", Plays: 5, ListenedSecs: 500, Tracks: 2},
	})

	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1: %+v", len(groups), groups)
	}
	g := groups[0]
	if g.Canonical != "Ambient" {
		t.Errorf("Canonical = %q, want %q (the most played spelling)", g.Canonical, "Ambient")
	}
	wantVariants := []string{"AMBIENT ", "Ambient", "ambient"}
	if !reflect.DeepEqual(g.Variants, wantVariants) {
		t.Errorf("Variants = %q, want %q preserved verbatim", g.Variants, wantVariants)
	}
	if g.Plays != 147 || g.ListenedSecs != 14700 || g.Tracks != 37 {
		t.Errorf("group totals = %d plays/%d secs/%d tracks, want 147/14700/37",
			g.Plays, g.ListenedSecs, g.Tracks)
	}
}

func TestFoldGenresKeepsDistinctGenresApartAndSortsByPlays(t *testing.T) {
	groups := foldGenres([]GenreRow{
		{Genre: "jazz", Plays: 10},
		{Genre: "Ambient", Plays: 5},
		{Genre: "ambient", Plays: 6},
		{Genre: "   ", Plays: 99},
		{Genre: "", Plays: 99},
	})

	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2 (blank genres dropped): %+v", len(groups), groups)
	}
	// Folding is what puts ambient ahead of jazz: 5 + 6 = 11 plays. The
	// canonical spelling is the most played single variant, "ambient".
	if groups[0].Canonical != "ambient" || groups[0].Plays != 11 {
		t.Errorf("groups[0] = %s/%d, want ambient/11", groups[0].Canonical, groups[0].Plays)
	}
	if groups[1].Canonical != "jazz" || groups[1].Plays != 10 {
		t.Errorf("groups[1] = %s/%d, want jazz/10", groups[1].Canonical, groups[1].Plays)
	}
}

func TestFoldGenresEmptyIsNil(t *testing.T) {
	if got := foldGenres(nil); got != nil {
		t.Errorf("foldGenres(nil) = %+v, want nil", got)
	}
}

func TestQualityReportsCollisionsAsFixable(t *testing.T) {
	groups := foldGenres([]GenreRow{
		{Genre: "Ambient", Plays: 81},
		{Genre: "ambient", Plays: 61},
		{Genre: "jazz", Plays: 10},
	})

	q := computeQuality(groups, QualityCounts{})

	if len(q.Collisions) != 1 || q.Collisions[0].Canonical != "Ambient" {
		t.Fatalf("Collisions = %+v, want only the Ambient group", q.Collisions)
	}
	if q.CollisionPlays != 142 {
		t.Errorf("CollisionPlays = %d, want 142", q.CollisionPlays)
	}
	issue := findIssue(t, q, IssueGenreCollision)
	if !issue.Fixable {
		t.Error("genre collision issue is not marked fixable")
	}
	if issue.Affected != 142 {
		t.Errorf("issue.Affected = %d, want 142", issue.Affected)
	}
}

func TestQualityReportsPlaceholderGenresAsFixable(t *testing.T) {
	groups := foldGenres([]GenreRow{
		{Genre: "Unknown", Plays: 40},
		{Genre: "jazz", Plays: 10},
		{Genre: "Misc", Plays: 7},
	})

	q := computeQuality(groups, QualityCounts{})

	if len(q.Placeholders) != 2 {
		t.Fatalf("Placeholders = %+v, want Unknown and Misc", q.Placeholders)
	}
	if q.PlaceholderPlays != 47 {
		t.Errorf("PlaceholderPlays = %d, want 47", q.PlaceholderPlays)
	}
	issue := findIssue(t, q, IssuePlaceholderGenre)
	if !issue.Fixable {
		t.Error("placeholder genres are editable metadata, so the issue must be fixable")
	}
	if issue.Affected != 47 {
		t.Errorf("issue.Affected = %d, want 47", issue.Affected)
	}
}

func TestIsPlaceholderGenreIgnoresCaseAndSpacing(t *testing.T) {
	for _, name := range []string{"Unknown", "  unknown  ", "N/A", "Untagged", "-"} {
		if !IsPlaceholderGenre(name) {
			t.Errorf("IsPlaceholderGenre(%q) = false, want true", name)
		}
	}
	// "Various" is a placeholder; "Various Artists" is a compilation credit that
	// happens to start with it, and folding the two would erase a real tag.
	for _, name := range []string{"jazz", "Various Artists", "Unknown Mortal Orchestra", ""} {
		if IsPlaceholderGenre(name) {
			t.Errorf("IsPlaceholderGenre(%q) = true, want false", name)
		}
	}
}

func TestQualityFlagsInertDeepCutsFilter(t *testing.T) {
	counts := QualityCounts{
		TracksTotal:        2382,
		DeepCutsCandidates: 1000,
		DeepCutsQualifying: 989,
		PlaysWithoutGenre:  217,
		TracksWithoutGenre: 90,
		TracksMissing:      4,
	}

	q := computeQuality(nil, counts)

	if q.DeepCutsShare != 0.989 {
		t.Errorf("DeepCutsShare = %v, want 0.989", q.DeepCutsShare)
	}
	for _, kind := range []IssueKind{IssueGenreMissing, IssueDeepCutsFlat, IssueMissingFiles} {
		issue := findIssue(t, q, kind)
		if issue.Fixable {
			t.Errorf("%s is marked fixable but has no in-app fix", kind)
		}
	}
	if findIssue(t, q, IssueGenreMissing).Affected != 217 {
		t.Error("missing-genre issue must be sized in plays, not tracks")
	}
}

func TestQualityDiscriminatingDeepCutsIsNotAnIssue(t *testing.T) {
	q := computeQuality(nil, QualityCounts{DeepCutsCandidates: 1000, DeepCutsQualifying: 400})

	for _, issue := range q.Issues {
		if issue.Kind == IssueDeepCutsFlat {
			t.Errorf("reported %v for a filter that excludes 60%% of candidates", issue.Kind)
		}
	}
}

func TestQualityDivisionByZero(t *testing.T) {
	q := computeQuality(nil, QualityCounts{TracksTotal: 10})
	if q.DeepCutsShare != 0 {
		t.Errorf("DeepCutsShare = %v, want 0 with no candidates", q.DeepCutsShare)
	}
}

func TestQualityEmptyIsZero(t *testing.T) {
	if got := computeQuality(nil, QualityCounts{}); !reflect.DeepEqual(got, Quality{}) {
		t.Errorf("computeQuality(nil, zero) = %+v, want zero Quality", got)
	}
}

func findIssue(t *testing.T, q Quality, kind IssueKind) HealthIssue {
	t.Helper()
	for _, issue := range q.Issues {
		if issue.Kind == kind {
			return issue
		}
	}
	t.Fatalf("no %s issue in %+v", kind, q.Issues)
	return HealthIssue{}
}
