package audio_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/audio"
)

func TestExtensions_ExpectedFormats(t *testing.T) {
	expected := []string{
		".mp3",
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
		if _, ok := audio.Extensions[ext]; !ok {
			t.Errorf("missing expected extension %q", ext)
		}
	}
	if len(audio.Extensions) != len(expected) {
		t.Errorf("Extensions has %d entries, expected %d", len(audio.Extensions), len(expected))
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
		{name: "without dot", ext: "aiff", want: true},
		{name: "AIFF-C variant", ext: ".aifc", want: true},
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

func TestIsSupportedExtension_IgnoresExtensionsMutation(t *testing.T) {
	original := make(map[string]struct{}, len(audio.Extensions))
	for ext := range audio.Extensions {
		original[ext] = struct{}{}
	}
	t.Cleanup(func() {
		for ext := range audio.Extensions {
			delete(audio.Extensions, ext)
		}
		for ext := range original {
			audio.Extensions[ext] = struct{}{}
		}
	})

	delete(audio.Extensions, ".mp3")
	audio.Extensions[".txt"] = struct{}{}

	if !audio.IsSupportedExtension(".mp3") {
		t.Fatal("IsSupportedExtension(\".mp3\") = false after Extensions mutation, want true")
	}
	if audio.IsSupportedExtension(".txt") {
		t.Fatal("IsSupportedExtension(\".txt\") = true after Extensions mutation, want false")
	}
}
