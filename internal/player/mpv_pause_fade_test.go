package player

import (
	"context"
	"sync"
	"testing"
)

// fadeHarness wires a Manager to a fake mpv that records every command.
func fadeHarness(t *testing.T, name string) (*Manager, func() []ipcTestCommand) {
	t.Helper()
	sock := shortSockPath(t, name)
	var mu sync.Mutex
	var received []ipcTestCommand
	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		if cmd := req.Command; len(cmd) > 0 {
			mu.Lock()
			received = append(received, cmd)
			mu.Unlock()
		}
		return successHandler(req)
	})
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel}
	t.Cleanup(func() { _ = m.Close() })
	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.lastVolume, m.volumeKnown = 80, true
	m.mu.Unlock()
	m.SetPauseFade(true)
	return m, func() []ipcTestCommand {
		mu.Lock()
		defer mu.Unlock()
		return append([]ipcTestCommand(nil), received...)
	}
}

// volumeTrail returns the volumes set, in order, and the index of the pause
// command among all set_property calls.
func volumeTrail(cmds []ipcTestCommand) (volumes []float64, pauseAt int) {
	pauseAt = -1
	for _, c := range cmds {
		if !c.stringIs(0, "set_property") {
			continue
		}
		if c.stringIs(1, "volume") {
			if v, ok := c.float64At(2); ok {
				volumes = append(volumes, v)
			}
		}
		if c.stringIs(1, "pause") {
			pauseAt = len(volumes)
		}
	}
	return volumes, pauseAt
}

func TestPauseFadesOutThenRestoresVolume(t *testing.T) {
	m, received := fadeHarness(t, "fadeout")
	if err := m.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	volumes, pauseAt := volumeTrail(received())
	if len(volumes) != pauseFadeSteps+1 || pauseAt != pauseFadeSteps {
		t.Fatalf("want %d fade steps, the pause, then a restore; got volumes %v (pause after %d)", pauseFadeSteps, volumes, pauseAt)
	}
	for i := 1; i < pauseFadeSteps; i++ {
		if volumes[i] >= volumes[i-1] {
			t.Fatalf("fade-out is not decreasing: %v", volumes)
		}
	}
	if volumes[pauseFadeSteps-1] != 0 || volumes[pauseFadeSteps] != 80 {
		t.Fatalf("want silence at the pause and 80 restored after; got %v", volumes)
	}
}

func TestPlayFadesInOnlyFromPause(t *testing.T) {
	m, received := fadeHarness(t, "fadein")
	m.mu.Lock()
	m.lastPaused = true
	m.mu.Unlock()
	if err := m.Play(); err != nil {
		t.Fatalf("Play: %v", err)
	}
	volumes, pauseAt := volumeTrail(received())
	if pauseAt != 1 || volumes[0] != 0 || volumes[len(volumes)-1] != 80 || len(volumes) != pauseFadeSteps+1 {
		t.Fatalf("want silence, unpause, then a ramp to 80; got %v (unpause after %d)", volumes, pauseAt)
	}

	// Already playing: Play must not dip the volume.
	m2, received2 := fadeHarness(t, "nofade")
	if err := m2.Play(); err != nil {
		t.Fatalf("Play: %v", err)
	}
	if volumes, _ := volumeTrail(received2()); len(volumes) != 0 {
		t.Fatalf("Play while playing changed the volume: %v", volumes)
	}
}

func TestPauseWithoutFadeSendsNoVolume(t *testing.T) {
	m, received := fadeHarness(t, "fadeoff")
	m.SetPauseFade(false)
	if err := m.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if volumes, pauseAt := volumeTrail(received()); len(volumes) != 0 || pauseAt != 0 {
		t.Fatalf("fade disabled but volumes %v sent (pause after %d)", volumes, pauseAt)
	}
}
