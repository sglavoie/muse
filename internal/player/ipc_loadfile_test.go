package player

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLoadFileReplaceCompatibility(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "modern"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			for _, options := range []string{"pause=no", "start=7.5,pause=yes", "start=7.5,pause=no"} {
				t.Run(options, func(t *testing.T) {
					var mu sync.Mutex
					var received []ipcTestCommand
					client := newHandlerClient(t, "load-compat", func(req ipcTestRequest) ipcTestResponse {
						recordCommand(&mu, &received, req.Command)
						if legacy && len(req.Command) == 5 {
							return errorHandler("invalid parameter")(req)
						}
						return successHandler(req)
					})
					path := "/music/a song, with spaces.flac"
					if err := client.loadFileReplace(path, options); err != nil {
						t.Fatalf("loadFileReplace: %v", err)
					}
					mu.Lock()
					defer mu.Unlock()
					wantCalls, wantArgs := 1, 5
					if legacy {
						wantCalls, wantArgs = 2, 4
					}
					if len(received) != wantCalls {
						t.Fatalf("commands = %v, want %d calls", received, wantCalls)
					}
					cmd := received[len(received)-1]
					if len(cmd) != wantArgs || !cmd.stringIs(0, cmdLoadfile) ||
						!cmd.stringIs(1, path) || !cmd.stringIs(2, "replace") || !cmd.stringIs(wantArgs-1, options) {
						t.Fatalf("load command lost its path, mode or options: %v", cmd)
					}
				})
			}
		})
	}
}

func TestLoadFileReplaceErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		first     string
		wantCalls int
	}{
		{"playback failure is not retried", "error running command", 1},
		{"legacy rejection is returned", "invalid parameter", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var received []ipcTestCommand
			client := newHandlerClient(t, "load-error", func(req ipcTestRequest) ipcTestResponse {
				recordCommand(&mu, &received, req.Command)
				return errorHandler(tc.first)(req)
			})
			if err := client.loadFileReplace("/music/a.flac", "pause=no"); !errors.Is(err, ErrPlaybackFailed) {
				t.Fatalf("error = %v, want ErrPlaybackFailed", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(received) != tc.wantCalls {
				t.Fatalf("commands = %v, want %d calls", received, tc.wantCalls)
			}
		})
	}

	t.Run("transport failure", func(t *testing.T) {
		client := newTestClient(t)
		client.Close()
		if err := client.loadFileReplace("/music/a.flac", "pause=no"); !errors.Is(err, ErrIPCTransport) {
			t.Fatalf("error = %v, want ErrIPCTransport", err)
		}
	})
}

// Exercise each caller against the argument layout shipped in Ubuntu 24.04.
func TestLegacyMPVLoadPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "track.wav")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		options string
		load    func(*IPCClient) error
	}{
		{"play", "pause=no", func(c *IPCClient) error {
			m := NewManager("")
			m.client = c
			return m.PlayQueueEntry(QueueTrack{Path: path, Hash: "track", QueueID: 1}, nil)
		}},
		{"restore", "start=7.5,pause=yes", func(c *IPCClient) error {
			m := NewManager("")
			m.client = c
			return m.LoadFilePaused(path, 7500)
		}},
		{"restart", "start=7.5,pause=no", func(c *IPCClient) error {
			m := NewManager("")
			m.client = c
			return m.loadFileAt(path, 7.5, false)
		}},
		{"preview", "pause=no", func(c *IPCClient) error {
			p := &PreviewPlayer{client: c}
			return p.LoadAndPlay(path)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var accepted []ipcTestCommand
			client := newHandlerClient(t, "legacy-path", func(req ipcTestRequest) ipcTestResponse {
				if req.Command.stringIs(0, cmdLoadfile) {
					if len(req.Command) != 4 || !req.Command.stringIs(3, tc.options) {
						return errorHandler("invalid parameter")(req)
					}
					recordCommand(&mu, &accepted, req.Command)
				}
				return successHandler(req)
			})
			if err := tc.load(client); err != nil {
				t.Fatalf("load: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(accepted) != 1 || !accepted[0].stringIs(1, path) {
				t.Fatalf("accepted loads = %v, want one load of %s", accepted, path)
			}
		})
	}
}
