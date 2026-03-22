package audio_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/audio"
)

func TestExtensions_ExpectedFormats(t *testing.T) {
	expected := []string{".mp3", ".flac", ".ogg", ".opus", ".m4a", ".wav", ".wma", ".aac", ".aiff", ".ape", ".wv"}
	for _, ext := range expected {
		if _, ok := audio.Extensions[ext]; !ok {
			t.Errorf("missing expected extension %q", ext)
		}
	}
	if len(audio.Extensions) != len(expected) {
		t.Errorf("Extensions has %d entries, expected %d", len(audio.Extensions), len(expected))
	}
}
