package library_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

func TestFFProbeDuration_RejectsInvalidDurations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell executable is Unix-specific")
	}

	cases := []string{"NaN", "+Inf", "-Inf", "-1"}
	for _, output := range cases {
		t.Run(output, func(t *testing.T) {
			dir := t.TempDir()
			ffprobe := filepath.Join(dir, "ffprobe")
			script := "#!/bin/sh\nprintf '%s\\n' '" + output + "'\n"
			if err := os.WriteFile(ffprobe, []byte(script), 0o755); err != nil {
				t.Fatalf("write fake ffprobe: %v", err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

			_, err := library.FFProbeDuration(context.Background(), "track.mp3")
			if err == nil {
				t.Fatal("expected invalid duration error, got nil")
			}
			if !strings.Contains(err.Error(), "invalid duration") {
				t.Fatalf("error = %q, want invalid duration", err)
			}
		})
	}
}

// A probe killed by its deadline must report the context error, not just
// "signal: killed", so the duration audit can tell a slow probe from a file
// ffprobe cannot read.
func TestFFProbeDuration_TimeoutWrapsContextError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell executable is Unix-specific")
	}

	dir := t.TempDir()
	ffprobe := filepath.Join(dir, "ffprobe")
	if err := os.WriteFile(ffprobe, []byte("#!/bin/sh\nexec sleep 5\n"), 0o755); err != nil {
		t.Fatalf("write fake ffprobe: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := library.FFProbeDuration(ctx, "track.mp3")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want it to wrap context.DeadlineExceeded", err)
	}
}
