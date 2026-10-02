package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"syscall"
	"testing"
)

func TestShutdownSignals_IncludesHangup(t *testing.T) {
	got := shutdownSignals()
	for _, want := range []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} {
		if !slices.Contains(got, want) {
			t.Errorf("shutdownSignals() = %v, missing %v", got, want)
		}
	}
}

func TestEndedBySignal(t *testing.T) {
	live := context.Background()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	runErr := errors.New("read /dev/tty: input/output error")

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{"clean quit", live, nil, false},
		{"signal then clean quit", cancelled, nil, false},
		{"TUI failure without a signal", live, runErr, false},
		{"error after a signal", cancelled, runErr, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := endedBySignal(tt.ctx, tt.err); got != tt.want {
				t.Errorf("endedBySignal = %v, want %v", got, tt.want)
			}
		})
	}
}
