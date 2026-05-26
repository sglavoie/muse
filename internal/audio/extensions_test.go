package audio_test

import (
	"slices"
	"testing"

	"github.com/sglavoie/muse/internal/audio"
)

func TestExtensions_ExpectedFormats(t *testing.T) {
	expected := []string{
		".mp3",
		".mp2",
		".mpa",
		".flac",
		".ogg",
		".oga",
		".opus",
		".m4a",
		".m4b",
		".wav",
		".wma",
		".aac",
		".aif",
		".aifc",
		".aiff",
		".ape",
		".mka",
		".wv",
	}
	for _, ext := range expected {
		if !audio.IsSupportedExtension(ext) {
			t.Errorf("missing expected extension %q", ext)
		}
	}
	if got := audio.SupportedExtensions(); len(got) != len(expected) {
		t.Errorf("SupportedExtensions has %d entries, expected %d", len(got), len(expected))
	}
}

func TestIsSupportedExtension(t *testing.T) {
	tests := []struct {
		name string
		ext  string
		want bool
	}{
		{name: "lowercase with dot", ext: ".mp3", want: true},
		{name: "uppercase with dot", ext: ".FLAC", want: true},
		{name: "MPEG layer II", ext: ".mp2", want: true},
		{name: "MPEG audio", ext: ".mpa", want: true},
		{name: "without dot", ext: "aiff", want: true},
		{name: "surrounding whitespace with dot", ext: " \t.m4a\n", want: true},
		{name: "surrounding whitespace without dot", ext: " \topus\n", want: true},
		{name: "AIFF-C variant", ext: ".aifc", want: true},
		{name: "double dot", ext: "..mp3", want: false},
		{name: "spaces and dots", ext: " ...FLAC ", want: false},
		{name: "dots only", ext: "...", want: false},
		{name: "whitespace only", ext: " \t\n", want: false},
		{name: "empty", ext: "", want: false},
		{name: "unknown", ext: ".txt", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := audio.IsSupportedExtension(tt.ext); got != tt.want {
				t.Fatalf("IsSupportedExtension(%q) = %v, want %v", tt.ext, got, tt.want)
			}
		})
	}
}

func TestSupportedExtensions_ReturnsSortedCopy(t *testing.T) {
	got := audio.SupportedExtensions()
	if !slices.IsSorted(got) {
		t.Fatalf("SupportedExtensions() = %v, want sorted extensions", got)
	}

	got[0] = ".txt"

	if audio.IsSupportedExtension(".txt") {
		t.Fatal("mutating SupportedExtensions result changed supported extensions")
	}
	if !audio.IsSupportedExtension(".aac") {
		t.Fatal("mutating SupportedExtensions result removed supported extension")
	}
}
