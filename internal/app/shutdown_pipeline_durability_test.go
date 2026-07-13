package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/app"
)

// TestShutdownFlushesLibraryChangeBeforeWriterStops is the end-to-end
// regression for the "single changed track is durable before the database
// writer stops on normal quit" acceptance scenario. A brand-new file on disk is
// discovered by the asynchronous library pipeline but not yet size-flushed
// (batch size is far above one track), so it only reaches the database during
// the pipeline's final drain. app.Shutdown must run StopLibraryPipeline before
// draining the DB writer, otherwise that final upsert would be lost.
func TestShutdownFlushesLibraryChangeBeforeWriterStops(t *testing.T) {
	installLogger(t)

	dirs := setupDirs(t)
	musicDir := t.TempDir()

	// A new file on disk that is absent from the DB cache, so the diff walk
	// routes it through the scan bridge as a new track.
	newPath := filepath.Join(musicDir, "new.mp3")
	if err := os.WriteFile(newPath, []byte("fake-audio"), 0o644); err != nil {
		t.Fatalf("write new track: %v", err)
	}

	result, err := app.Run(t.Context(), dirs, noopDeps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	defer result.DB.Close()

	// A separate context for the pipeline; StopAndWait cancels the pipeline's own
	// child context, mirroring production where the outer signal context is not
	// cancelled on a normal quit.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// A batch size far above one track so the discovered track is retained in the
	// bridge's batch and only persisted on the pipeline's final drain — never by
	// a size-cap flush.
	libResult, err := app.LoadLibrary(ctx, result.DB, result.Writer, []string{musicDir}, nil, stubExtractor{}, 100)
	if err != nil {
		t.Fatalf("LoadLibrary: %v", err)
	}

	// Wait for the pipeline to actually discover the new track (it appears on the
	// scan channel once extraction produces it). At this point the track sits in
	// the scan bridge's batch buffer, unflushed, exactly as it would on a quit
	// issued mid-session.
	select {
	case tr, ok := <-libResult.TuiScanCh:
		if !ok {
			t.Fatal("scan channel closed before the new track was discovered")
		}
		if tr.Path != newPath {
			t.Fatalf("unexpected scanned track %q, want %q", tr.Path, newPath)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("pipeline never discovered the new track")
	}

	// Wire the production shutdown ordering: stop and flush the pipeline, then
	// drain the DB writer.
	app.Shutdown(app.ShutdownDeps{
		StopLibraryPipeline: libResult.StopAndWait,
		CancelDBWriter:      result.CancelWriter,
		WaitDBWriter:        result.Writer.Wait,
	}, app.ShutdownTimeout)

	// The new track must be present in the database now that the pipeline was
	// flushed before the writer stopped.
	tracks, err := result.DB.ReadTracks()
	if err != nil {
		t.Fatalf("ReadTracks: %v", err)
	}
	found := false
	for _, tr := range tracks {
		if tr.Path == newPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("new track %q was not persisted before the writer stopped; got %d tracks", newPath, len(tracks))
	}
}
