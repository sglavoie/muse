package library

import (
	"context"
	"path/filepath"
	"slices"
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

// A bare-name pattern only matches components below the music root, so
// excluding "Music" with a root of ~/Music hides a nested Music folder but
// never the whole library.
func TestShouldExcludeUnder_IgnoresRootAncestry(t *testing.T) {
	roots := []string{"/Users/me/Music"}
	patterns := []string{"Music", "me"}
	if ShouldExcludeUnder("/Users/me/Music/Album/a.mp3", roots, patterns) {
		t.Error("root's own components must not match bare-name patterns")
	}
	if ShouldExcludeUnder("/Users/me/Music", roots, patterns) {
		t.Error("the root itself must not be excluded by a bare-name pattern")
	}
	if !ShouldExcludeUnder("/Users/me/Music/Rips/Music/a.mp3", roots, patterns) {
		t.Error("a component below the root must still match")
	}
	// Nested roots: reachable from the inner root, so not excluded.
	nested := []string{"/lib", "/lib/Podcasts"}
	if ShouldExcludeUnder("/lib/Podcasts/ep.mp3", nested, []string{"Podcasts"}) {
		t.Error("a path reachable from a nested root must not be excluded")
	}
	if !ShouldExcludeUnder("/other/Podcasts/ep.mp3", nested, []string{"Podcasts"}) {
		t.Error("a path under no root checks every component")
	}
}

func TestWalk_ExcludedNameMatchingRootStillWalksRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	mkfile(t, filepath.Join(root, "a.mp3"))
	mkfile(t, filepath.Join(root, "Sub", "Music", "b.mp3"))

	got := collectNames(Walk(context.Background(), []string{root}, []string{"Music"}))
	if !slices.Equal(got, []string{"a.mp3"}) {
		t.Fatalf("Walk = %v, want [a.mp3]", got)
	}
	known := map[string]KnownFile{filepath.Join(root, "gone.mp3"): {ModTime: 1, SizeBytes: 1}}
	_, deleted, err := FilteredWalk(context.Background(), []string{root}, []string{"Music"}, known, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 {
		t.Fatalf("deleted = %v, want the missing track under the root reported", deleted)
	}
}

// Directory-prefix patterns ignore case on macOS, whose volumes are
// case-insensitive by default, and stay exact elsewhere.
func TestShouldExclude_PrefixCaseFolding(t *testing.T) {
	orig := foldPathPrefixes
	t.Cleanup(func() { foldPathPrefixes = orig })
	patterns := []string{"/users/me/music/AI"}

	foldPathPrefixes = true
	if !ShouldExclude("/Users/me/Music/ai/file.wav", patterns) {
		t.Error("folding: expected a case-insensitive prefix match")
	}
	if ShouldExclude("/Users/me/Music/aiden/file.wav", patterns) {
		t.Error("folding: /music/AI must not match /Music/aiden")
	}
	foldPathPrefixes = false
	if ShouldExclude("/Users/me/Music/ai/file.wav", patterns) {
		t.Error("no folding: prefix match must be case-sensitive")
	}
}
