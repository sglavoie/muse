package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/config"
)

func TestExitCodeFor(t *testing.T) {
	running := &config.AlreadyRunningError{PID: 42, Path: "/run/muse.pid"}
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"generic failure", errors.New("load config: boom"), 1},
		{"already running", running, 3},
		{"already running wrapped by startup", fmt.Errorf("startup: %w", running), 3},
		{"same text but untyped", errors.New(running.Error()), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCodeFor(tt.err); got != tt.want {
				t.Errorf("exitCodeFor(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
