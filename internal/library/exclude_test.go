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

func TestShouldExclude_FullPathPrefix(t *testing.T) {
	got := ShouldExclude("/Users/me/Music/ai/sub/file.wav", []string{"/Users/me/Music/ai"})
	if !got {
		t.Error("expected true: full directory path should match files underneath")
	}
}

func TestShouldExclude_FullPathWithTrailingSlash(t *testing.T) {
	got := ShouldExclude("/Users/me/Music/ai/file.wav", []string{"/Users/me/Music/ai/"})
	if !got {
		t.Error("expected true: trailing slash should be handled")
	}
}

func TestShouldExclude_FullPathNoFalsePrefix(t *testing.T) {
	got := ShouldExclude("/Users/me/Music/aiden/file.wav", []string{"/Users/me/Music/ai"})
	if got {
		t.Error("expected false: /Music/ai must not match /Music/aiden")
	}
}

func TestShouldExclude_FullPathExactDir(t *testing.T) {
	got := ShouldExclude("/Users/me/Music/ai", []string{"/Users/me/Music/ai"})
	if !got {
		t.Error("expected true: exact directory match")
	}
}

func TestShouldExclude_MixedPatterns(t *testing.T) {
	patterns := []string{"/Users/me/Music/ai", "Podcasts"}

	if !ShouldExclude("/Users/me/Music/ai/file.wav", patterns) {
		t.Error("expected true: prefix match on full path")
	}
	if !ShouldExclude("/other/Podcasts/ep.mp3", patterns) {
		t.Error("expected true: component match on Podcasts")
	}
	if ShouldExclude("/other/Music/track.mp3", patterns) {
		t.Error("expected false: neither pattern matches")
	}
}

// TestShouldExclude_RootOnlyPatternIsIgnored verifies that a degenerate
// pattern of separator characters only (e.g. "/" or "//") does not
// trigger a catch-all exclusion of every absolute path. Without the
// guard, every track would be silently filtered out.
func TestShouldExclude_RootOnlyPatternIsIgnored(t *testing.T) {
	cases := []string{"/", "//", "///"}
	for _, p := range cases {
		if ShouldExclude("/music/track.mp3", []string{p}) {
			t.Errorf("pattern %q must not exclude /music/track.mp3 (would hide entire library)", p)
		}
	}
}
