package player

import (
	"context"
	"strings"
	"testing"
)

// TestManager_DebugInfo_NotConnected verifies that DebugInfo returns a map
// with all "N/A" values (and no panic) when the manager is not connected.
func TestManager_DebugInfo_NotConnected(t *testing.T) {
	m := NewManager(shortSockPath(t, "dbg-noconn"))
	defer m.Close()

	info, err := m.DebugInfo()
	if err == nil {
		t.Fatal("DebugInfo: expected error when not connected, got nil")
	}
	for _, key := range []string{"codec", "sample_rate", "bitrate", "audio_device", "path"} {
		if info[key] != "N/A" {
			t.Errorf("DebugInfo[%q] = %q, want \"N/A\" when not connected", key, info[key])
		}
	}
}

// TestManager_DebugInfo_Connected verifies that DebugInfo returns property
// values from mpv when the manager is connected to a fake server.
func TestManager_DebugInfo_Connected(t *testing.T) {
	sock := shortSockPath(t, "dbg-conn")

	// Map of mpv property name → value the fake server returns.
	props := map[string]string{
		"audio-codec-name":        "mp3float",
		"audio-params/samplerate": "44100",
		"audio-bitrate":           "320000",
		"audio-device":            "auto",
		"path":                    "/music/track.mp3",
	}

	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		cmd, _ := req["command"].([]any)
		if len(cmd) >= 2 {
			if verb, _ := cmd[0].(string); verb == "get_property" {
				if prop, _ := cmd[1].(string); prop != "" {
					if val, ok := props[prop]; ok {
						return map[string]any{
							"error":      "success",
							"data":       val,
							"request_id": req["request_id"],
						}
					}
				}
			}
		}
		return successHandler(req)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath: sock,
		ctx:        ctx,
		cancel:     cancel,
	}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	info, err := m.DebugInfo()
	if err != nil {
		t.Fatalf("DebugInfo: unexpected error: %v", err)
	}

	cases := []struct{ key, want string }{
		{"codec", "mp3float"},
		{"sample_rate", "44100"},
		{"bitrate", "320000"},
		{"audio_device", "auto"},
		{"path", "/music/track.mp3"},
	}
	for _, tc := range cases {
		if info[tc.key] != tc.want {
			t.Errorf("DebugInfo[%q] = %q, want %q", tc.key, info[tc.key], tc.want)
		}
	}
}

// TestManager_DebugInfo_GracefulDegradation verifies that when some mpv
// properties return errors, the available ones are populated and the failing
// ones default to "N/A".
func TestManager_DebugInfo_GracefulDegradation(t *testing.T) {
	sock := shortSockPath(t, "dbg-degrade")

	// Only respond to path; all others get "property not found".
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		cmd, _ := req["command"].([]any)
		if len(cmd) >= 2 {
			if verb, _ := cmd[0].(string); verb == "get_property" {
				if prop, _ := cmd[1].(string); prop == "path" {
					return map[string]any{
						"error":      "success",
						"data":       "/some/track.flac",
						"request_id": req["request_id"],
					}
				}
				// All other properties fail.
				return map[string]any{
					"error":      "property not found",
					"data":       nil,
					"request_id": req["request_id"],
				}
			}
		}
		return successHandler(req)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath: sock,
		ctx:        ctx,
		cancel:     cancel,
	}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	info, err := m.DebugInfo()
	if err != nil {
		t.Fatalf("DebugInfo: unexpected error: %v", err)
	}

	if info["path"] != "/some/track.flac" {
		t.Errorf("DebugInfo[\"path\"] = %q, want \"/some/track.flac\"", info["path"])
	}
	for _, key := range []string{"codec", "sample_rate", "bitrate", "audio_device"} {
		if info[key] != "N/A" {
			t.Errorf("DebugInfo[%q] = %q, want \"N/A\" (failed property)", key, info[key])
		}
	}
}

// TestManager_DebugInfo_AudioFilters verifies that active audio filters are
// included in the DebugInfo result.
func TestManager_DebugInfo_AudioFilters(t *testing.T) {
	sock := shortSockPath(t, "dbg-filters")
	fakeServer(t, sock)

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath:   sock,
		ctx:          ctx,
		cancel:       cancel,
		audioFilters: map[string]string{"loudnorm": "lavfi=[loudnorm]"},
	}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	info, err := m.DebugInfo()
	if err != nil {
		t.Fatalf("DebugInfo: unexpected error: %v", err)
	}
	if !strings.Contains(info["audio_filters"], "loudnorm") {
		t.Errorf("DebugInfo[\"audio_filters\"] = %q, expected it to contain \"loudnorm\"", info["audio_filters"])
	}
}
