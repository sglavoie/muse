package library

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const ffprobeDurationTimeout = 10 * time.Second

// FFProbeAvailable returns true if ffprobe is on PATH.
func FFProbeAvailable() bool {
	_, err := exec.LookPath("ffprobe")
	return err == nil
}

// FFProbeDuration runs ffprobe on the given audio file and returns the
// duration in seconds. Returns an error if ffprobe fails or the output is
// unparseable. Times out after 10 seconds per file.
func FFProbeDuration(ctx context.Context, path string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, ffprobeDurationTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}

	s := strings.TrimSpace(string(out))
	dur, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe: parse %q: %w", s, err)
	}
	if math.IsNaN(dur) || math.IsInf(dur, 0) || dur < 0 {
		return 0, fmt.Errorf("ffprobe: invalid duration %q", s)
	}
	return dur, nil
}
