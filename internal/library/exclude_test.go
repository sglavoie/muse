package library

import (
	"testing"
)

func TestShouldExclude_MatchesExactComponent(t *testing.T) {
	got := ShouldExclude("/music/Podcasts/episode.mp3", []string{"Podcasts"})
	if !got {
		t.Error("expected true for exact component match on 'Podcasts'")
	}
}

func TestShouldExclude_NoMatchOnPartialComponent(t *testing.T) {
	got := ShouldExclude("/music/Podcasts/ep.mp3", []string{"Pod"})
	if got {
		t.Error("expected false: 'Pod' should not partially match 'Podcasts'")
	}
}

func TestShouldExclude_EmptyListNeverExcludes(t *testing.T) {
	if ShouldExclude("/music/Podcasts/ep.mp3", nil) {
		t.Error("nil excludedPaths: expected false")
	}
	if ShouldExclude("/music/Podcasts/ep.mp3", []string{}) {
		t.Error("empty excludedPaths: expected false")
	}
}

func TestShouldExclude_MultiplePatterns(t *testing.T) {
	patterns := []string{"Podcasts", "Audiobooks"}
	if !ShouldExclude("/media/Audiobooks/chapter.mp3", patterns) {
		t.Error("expected true: 'Audiobooks' is in patterns")
	}
	if !ShouldExclude("/media/Podcasts/show.mp3", patterns) {
		t.Error("expected true: 'Podcasts' is in patterns")
	}
	if ShouldExclude("/media/Music/track.mp3", patterns) {
		t.Error("expected false: 'Music' is not in patterns")
	}
}

func TestShouldExclude_NestedMatch(t *testing.T) {
	// Pattern matches at any depth.
	got := ShouldExclude("/a/b/c/Podcasts/d/e/track.mp3", []string{"Podcasts"})
	if !got {
		t.Error("expected true: match at any depth in path")
	}
}

func TestShouldExclude_RootComponent(t *testing.T) {
	// Matches the root directory name itself.
	got := ShouldExclude("/Podcasts/episode.mp3", []string{"Podcasts"})
	if !got {
		t.Error("expected true: root component matches")
	}
}

func TestShouldExclude_CaseSensitive(t *testing.T) {
	// Must not match different case.
	got := ShouldExclude("/music/podcasts/ep.mp3", []string{"Podcasts"})
	if got {
		t.Error("expected false: match is case-sensitive")
	}
}

func TestShouldExclude_FileNameNotMatchedAsDir(t *testing.T) {
	// The final file component (the mp3 name) is also checked. Verify that an
	// excluded pattern equal to the filename itself is matched.
	got := ShouldExclude("/music/track.mp3", []string{"track.mp3"})
	if !got {
		t.Error("expected true: filename component is also checked")
	}
}
