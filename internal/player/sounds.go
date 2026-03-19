package player

import (
	"context"
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

//go:embed sounds/queue_finished.wav
var queueFinishedWAV []byte

//go:embed sounds/error.aiff
var errorSound []byte

//go:embed sounds/enqueued.aiff
var enqueuedSound []byte

//go:embed sounds/removed.aiff
var removedSound []byte

// Sound identifies a notification sound.
type Sound int

const (
	SoundQueueFinished Sound = iota
	SoundError
	SoundEnqueued
	SoundRemoved
)

var soundData = map[Sound]struct {
	data []byte
	name string
}{
	SoundQueueFinished: {queueFinishedWAV, "queue-finished.wav"},
	SoundError:         {errorSound, "error.aiff"},
	SoundEnqueued:      {enqueuedSound, "enqueued.aiff"},
	SoundRemoved:       {removedSound, "removed.aiff"},
}

var (
	soundPaths map[Sound]string
	soundOnce  sync.Once
	// Muted suppresses all sound playback when true. Set to true in tests.
	Muted bool

	soundMu  sync.Mutex
	soundCtx context.Context
)

// SetSoundContext sets the parent context used by PlaySound for cancellation.
// Call this once after the signal context is created.
func SetSoundContext(ctx context.Context) {
	soundMu.Lock()
	soundCtx = ctx
	soundMu.Unlock()
}

func ensureSoundFiles() {
	soundOnce.Do(func() {
		dir := filepath.Join(os.TempDir(), "muse-sounds")
		_ = os.MkdirAll(dir, 0o755)
		soundPaths = make(map[Sound]string, len(soundData))
		for s, info := range soundData {
			p := filepath.Join(dir, info.name)
			_ = os.WriteFile(p, info.data, 0o644)
			soundPaths[s] = p
		}
	})
}

// PlaySound plays a notification sound using a fire-and-forget mpv subprocess.
// It is a no-op when Muted is true.
func PlaySound(s Sound) {
	if Muted {
		return
	}
	ensureSoundFiles()
	path, ok := soundPaths[s]
	if !ok {
		return
	}
	soundMu.Lock()
	parent := soundCtx
	soundMu.Unlock()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	cmd := exec.CommandContext(ctx, "mpv", "--no-video", "--no-terminal", "--really-quiet", path)
	_ = cmd.Start()
	go func() {
		_ = cmd.Wait()
		cancel()
	}()
}

// PlayQueueFinishedSound plays the queue-finished notification sound.
func PlayQueueFinishedSound() { PlaySound(SoundQueueFinished) }
