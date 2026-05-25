package player

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestManager_AutoRestart_RestoresAudioFiltersAndGapless verifies that after
// an mpv restart, active audio filters (e.g. loudnorm) and gapless-audio
// settings are re-applied to the new mpv process.
func TestManager_AutoRestart_RestoresAudioFiltersAndGapless(t *testing.T) {
	sock := shortSockPath(t, "restart-filters")

	// Phase 1: a fake server that handles a couple of pings then closes the
	// connection, simulating an mpv crash.
	os.Remove(sock)
	ln1, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen phase 1: %v", err)
	}
	t.Cleanup(func() { ln1.Close(); os.Remove(sock) })

	connClosed := make(chan struct{})
	go func() {
		conn, err := ln1.Accept()
		if err != nil {
			return
		}
		dec := json.NewDecoder(conn)
		enc := json.NewEncoder(conn)
		for range 2 {
			var req ipcTestRequest
			if err := dec.Decode(&req); err != nil {
				break
			}
			_ = enc.Encode(successHandler(req))
		}
		conn.Close()
		ln1.Close()
		close(connClosed)
	}()

	var mu sync.Mutex
	var received [][]any

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	m := &Manager{
		socketPath:     sock,
		rootCtx:        rootCtx,
		rootCancel:     rootCancel,
		ctx:            ctx,
		cancel:         cancel,
		disconnectCh:   make(chan error, 1),
		pingInterval:   50 * time.Millisecond,
		lastTrackPath:  "/music/track.flac",
		lastElapsed:    10.0,
		audioFilters:   map[string]string{"loudnorm": "loudnorm"},
		gaplessEnabled: true,
	}
	m.callbacks.OnStatus = func(string) {}
	m.launchFn = func(_ context.Context, socketPath string) (*exec.Cmd, error) {
		fakeServerWithHandler(t, socketPath, func(req ipcTestRequest) map[string]any {
			if cmd := req.Command; len(cmd) > 0 {
				mu.Lock()
				received = append(received, cmd)
				mu.Unlock()
			}
			return successHandler(req)
		})
		return nil, nil
	}

	// Manually connect the initial client (bypasses real mpv startup).
	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	go m.healthCheck(ctx, client)
	go m.watchAndRestart()

	// Wait for phase 1 to close the connection.
	select {
	case <-connClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("phase 1 server did not close in time")
	}

	// Wait for commands to arrive at the phase 2 server.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		// We expect at least: set_property af, set_property gapless-audio,
		// set_property prefetch-playlist, loadfile.
		if n >= 4 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	foundAF := false
	foundGapless := false
	foundPrefetch := false
	foundLoadfile := false
	for _, args := range received {
		if len(args) < 2 {
			continue
		}
		name, _ := args[0].(string)
		switch name {
		case "set_property":
			prop, _ := args[1].(string)
			switch prop {
			case "af":
				foundAF = true
				if len(args) > 2 {
					val, _ := args[2].(string)
					if !strings.Contains(val, "loudnorm") {
						t.Errorf("af property does not contain loudnorm: %v", args)
					}
				}
			case "gapless-audio":
				foundGapless = true
			case "prefetch-playlist":
				foundPrefetch = true
			}
		case "loadfile":
			foundLoadfile = true
		}
	}

	if !foundAF {
		t.Errorf("expected set_property af command after restart; commands: %v", received)
	}
	if !foundGapless {
		t.Errorf("expected set_property gapless-audio command after restart; commands: %v", received)
	}
	if !foundPrefetch {
		t.Errorf("expected set_property prefetch-playlist command after restart; commands: %v", received)
	}
	if !foundLoadfile {
		t.Errorf("expected loadfile command after restart; commands: %v", received)
	}
}
