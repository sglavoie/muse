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
		".m4r",
		".mp4",
		".wav",
		".w64",
		".wma",
		".aac",
		".ac3",
		".dts",
		".aif",
		".aifc",
		".aiff",
		".ape",
		".au",
		".snd",
		".caf",
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
		{name: "filename input", ext: "song.mp3", want: true},
		{name: "hidden filename input", ext: ".song.mp3", want: true},
		{name: "hidden uppercase filename input", ext: ".Song.FLAC", want: true},
		{name: "path input", ext: "/music/live/track.FLAC", want: true},
		{name: "windows path input", ext: `C:\Music\track.opus`, want: true},
		{name: "surrounding whitespace with dot", ext: " \t.m4a\n", want: true},
		{name: "surrounding whitespace without dot", ext: " \topus\n", want: true},
		{name: "AIFF-C variant", ext: ".aifc", want: true},
		{name: "MP4 container", ext: ".mp4", want: true},
		{name: "MP4 container uppercase", ext: ".MP4", want: true},
		{name: "MP4 container without dot", ext: "mp4", want: true},
		{name: "iPhone ringtone", ext: ".m4r", want: true},
		{name: "iPhone ringtone uppercase", ext: ".M4R", want: true},
		{name: "iPhone ringtone without dot", ext: "m4r", want: true},
		{name: "Dolby Digital", ext: ".ac3", want: true},
		{name: "Dolby Digital uppercase", ext: ".AC3", want: true},
		{name: "Dolby Digital without dot", ext: "ac3", want: true},
		{name: "DTS", ext: ".dts", want: true},
		{name: "DTS uppercase", ext: ".DTS", want: true},
		{name: "DTS without dot", ext: "dts", want: true},
		{name: "Core Audio Format", ext: ".caf", want: true},
		{name: "Core Audio Format uppercase", ext: ".CAF", want: true},
		{name: "Core Audio Format without dot", ext: "caf", want: true},
		{name: "Sun audio", ext: ".au", want: true},
		{name: "Sun audio uppercase", ext: ".AU", want: true},
		{name: "Sun audio without dot", ext: "au", want: true},
		{name: "NeXT sound", ext: ".snd", want: true},
		{name: "NeXT sound uppercase", ext: ".SND", want: true},
		{name: "NeXT sound without dot", ext: "snd", want: true},
		{name: "Sony Wave64", ext: ".w64", want: true},
		{name: "Sony Wave64 uppercase", ext: ".W64", want: true},
		{name: "Sony Wave64 without dot", ext: "w64", want: true},
		{name: "filename unknown extension", ext: "cover.jpg", want: false},
		{name: "path without extension", ext: "/music/album/track", want: false},
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
