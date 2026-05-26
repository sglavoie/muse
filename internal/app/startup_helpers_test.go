package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
)

func TestRunStartupTasks_CancelsSiblingOnFirstError(t *testing.T) {
	scanStarted := make(chan struct{})
	scanCanceled := make(chan struct{})
	mpvErr := errors.New("mpv failed")

	done := make(chan error, 1)
	go func() {
		done <- runStartupTasks(context.Background(), nil, "socket", Deps{
			LaunchMPV: func(context.Context, string) error {
				<-scanStarted
				return mpvErr
			},
			RunScan: func(ctx context.Context, _ *db.DB) error {
				close(scanStarted)
				<-ctx.Done()
				close(scanCanceled)
				return ctx.Err()
			},
		})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, mpvErr) {
			t.Fatalf("runStartupTasks error = %v, want wrapped %v", err, mpvErr)
		}
		if !strings.Contains(err.Error(), "startup: mpv") {
			t.Fatalf("runStartupTasks error = %v, want mpv context", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runStartupTasks timed out waiting for canceled sibling to exit")
	}

	select {
	case <-scanCanceled:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("RunScan did not observe cancellation")
	}
}
