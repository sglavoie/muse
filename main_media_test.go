package main

import (
	"testing"

	"github.com/sglavoie/muse/internal/app"
)

func TestMediaControlsWanted(t *testing.T) {
	hosted := tuiOptions{ControlPath: "/tmp/x.sock"}
	tests := []struct {
		name    string
		goos    string
		enabled bool
		opts    tuiOptions
		want    bool
	}{
		{"linux, enabled", "linux", true, tuiOptions{}, true},
		{"linux, disabled in config", "linux", false, tuiOptions{}, false},
		{"linux, hosted", "linux", true, hosted, false},
		{"macOS terminal", "darwin", true, tuiOptions{}, false},
	}
	for _, tt := range tests {
		if got := mediaControlsWanted(tt.goos, tt.enabled, tt.opts); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestStartMediaControls_NotWantedLeavesShutdownAlone(t *testing.T) {
	var deps app.ShutdownDeps
	if svc := startMediaControls(false, tuiOptions{}, &deps); svc != nil {
		t.Fatal("disabled media controls must not start")
	}
	if deps.CloseMediaControls != nil {
		t.Fatal("no shutdown hook without a service")
	}
	if opts := mediaModelOptions(nil); opts != nil {
		t.Fatal("a nil service must add no TUI options")
	}
	serveMediaControls(nil, nil) // must not panic
}
