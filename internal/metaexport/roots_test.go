package metaexport

import (
	"path/filepath"
	"testing"
)

func TestRootsLocate(t *testing.T) {
	tests := []struct {
		name    string
		dirs    []string
		abs     string
		wantID  int
		wantRel string
	}{
		{
			name:    "sibling root is not a prefix match",
			dirs:    []string{"/Music", "/Music2"},
			abs:     "/Music2/a/b.mp3",
			wantID:  2,
			wantRel: "a/b.mp3",
		},
		{
			name:    "shared prefix stays with its own root",
			dirs:    []string{"/Music", "/Music2"},
			abs:     "/Music/a/b.mp3",
			wantID:  1,
			wantRel: "a/b.mp3",
		},
		{
			name:    "nested root wins over its parent",
			dirs:    []string{"/Music", "/Music/Archive"},
			abs:     "/Music/Archive/a.flac",
			wantID:  2,
			wantRel: "a.flac",
		},
		{
			name:    "nested root declared first still wins",
			dirs:    []string{"/Music/Archive", "/Music"},
			abs:     "/Music/Archive/a.flac",
			wantID:  1,
			wantRel: "a.flac",
		},
		{
			name:    "trailing separator is cleaned off",
			dirs:    []string{"/Music/"},
			abs:     "/Music/a.mp3",
			wantID:  1,
			wantRel: "a.mp3",
		},
		{
			name:    "unmatched path falls back to root 0",
			dirs:    []string{"/Music"},
			abs:     "/tmp/loose.mp3",
			wantID:  0,
			wantRel: "/tmp/loose.mp3",
		},
		{
			name:    "empty root list matches nothing",
			dirs:    nil,
			abs:     "/Music/a.mp3",
			wantID:  0,
			wantRel: "/Music/a.mp3",
		},
		{
			name:    "blank root never matches",
			dirs:    []string{"  ", "/Music"},
			abs:     "/Music/a.mp3",
			wantID:  2,
			wantRel: "a.mp3",
		},
		{
			name:    "the root itself",
			dirs:    []string{"/Music"},
			abs:     "/Music",
			wantID:  1,
			wantRel: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRoots(tt.dirs)
			id, rel := r.Locate(tt.abs)
			if id != tt.wantID || rel != tt.wantRel {
				t.Fatalf("Locate(%q) = (%d, %q), want (%d, %q)", tt.abs, id, rel, tt.wantID, tt.wantRel)
			}
			if id == 0 {
				return
			}
			if got := filepath.Join(r.Dirs()[id-1], rel); got != tt.abs {
				t.Fatalf("rejoin: filepath.Join(%q, %q) = %q, want %q", r.Dirs()[id-1], rel, got, tt.abs)
			}
		})
	}
}

func TestRootsDirsAlignWithIDs(t *testing.T) {
	r := NewRoots([]string{"/Music/Archive/", "/Music"})
	want := []string{"/Music/Archive", "/Music"}
	dirs := r.Dirs()
	if len(dirs) != len(want) {
		t.Fatalf("Dirs() = %v, want %v", dirs, want)
	}
	for i := range want {
		if dirs[i] != want[i] {
			t.Fatalf("Dirs()[%d] = %q, want %q", i, dirs[i], want[i])
		}
	}
}
