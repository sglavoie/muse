package player

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

//go:embed sounds/queue_finished.wav
var queueFinishedSound []byte

var (
	soundTempPath string
	soundOnce     sync.Once
)

// ensureSoundFile writes the embedded WAV to a temp file once.
func ensureSoundFile() string {
	soundOnce.Do(func() {
		dir := os.TempDir()
		path := filepath.Join(dir, "muse-queue-finished.wav")
		_ = os.WriteFile(path, queueFinishedSound, 0o644)
		soundTempPath = path
	})
	return soundTempPath
}

// PlayQueueFinishedSound plays the queue-finished notification sound
// using a fire-and-forget mpv subprocess. Non-blocking.
func PlayQueueFinishedSound() {
	path := ensureSoundFile()
	if path == "" {
		return
	}
	cmd := exec.Command("mpv", "--no-video", "--no-terminal", "--really-quiet", path)
	_ = cmd.Start()
	// Fire-and-forget: reap the process in the background.
	go func() { _ = cmd.Wait() }()
}
