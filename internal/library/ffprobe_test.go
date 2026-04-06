package library_test

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestFFProbeAvailable verifies that FFProbeAvailable returns a bool without
// panicking. Whether it returns true or false depends on the environment.
func TestFFProbeAvailable(t *testing.T) {
	available := library.FFProbeAvailable()
	t.Logf("FFProbeAvailable = %v", available)
}

// TestFFProbeDuration_SkipIfNotInstalled skips when ffprobe is not available.
// When ffprobe is available, it verifies that probing a non-existent file
// returns an error rather than silently returning zero.
func TestFFProbeDuration_SkipIfNotInstalled(t *testing.T) {
	if !library.FFProbeAvailable() {
		t.Skip("ffprobe not installed; skipping")
	}

	_, err := library.FFProbeDuration(context.Background(), "/nonexistent/file.mp3")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}
