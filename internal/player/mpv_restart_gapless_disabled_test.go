package player

import (
	"context"
	"sync"
	"testing"
)

// TestRestoreStableProperties_GaplessDisabledSendsExplicitNo verifies that when
// gapless was disabled before an mpv restart, restoration sends explicit "no"
// for both gapless-audio and prefetch-playlist rather than skipping the send
// (which would let mpv fall back to its default weak gapless mode while the
// manager still reports gapless off). It also asserts the restore does not
// mutate the manager's gapless state.
func TestRestoreStableProperties_GaplessDisabledSendsExplicitNo(t *testing.T) {
	sock := shortSockPath(t, "restore-gapless-off")

	var mu sync.Mutex
	var received []ipcTestCommand
	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		recordCommand(&mu, &received, req.Command)
		return successHandler(req)
	})

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	m := &Manager{
		socketPath:     sock,
		rootCtx:        rootCtx,
		rootCancel:     rootCancel,
		audioFilters:   map[string]string{},
		gaplessEnabled: false,
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	m.restoreStableProperties(restoreState{gapless: false})

	mu.Lock()
	defer mu.Unlock()

	var gaplessVal, prefetchVal string
	var foundGapless, foundPrefetch bool
	for _, args := range received {
		if !args.stringIs(0, "set_property") {
			continue
		}
		prop, _ := args.stringAt(1)
		switch prop {
		case "gapless-audio":
			foundGapless = true
			gaplessVal, _ = args.stringAt(2)
		case "prefetch-playlist":
			foundPrefetch = true
			prefetchVal, _ = args.stringAt(2)
		}
	}

	if !foundGapless {
		t.Fatalf("expected set_property gapless-audio command; commands: %v", received)
	}
	if gaplessVal != "no" {
		t.Errorf("gapless-audio value = %q, want \"no\"", gaplessVal)
	}
	if !foundPrefetch {
		t.Fatalf("expected set_property prefetch-playlist command; commands: %v", received)
	}
	if prefetchVal != "no" {
		t.Errorf("prefetch-playlist value = %q, want \"no\"", prefetchVal)
	}

	m.mu.RLock()
	stillDisabled := !m.gaplessEnabled
	m.mu.RUnlock()
	if !stillDisabled {
		t.Errorf("gaplessEnabled = true after restore, want unchanged (false)")
	}
}
