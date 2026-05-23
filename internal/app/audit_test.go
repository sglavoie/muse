package app_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// stubReader implements app.DurationReader for tests.
type stubReader struct {
	tracks []db.UnverifiedTrack
	err    error
}

func (s *stubReader) ReadUnverifiedDurations() ([]db.UnverifiedTrack, error) {
	return s.tracks, s.err
}

// stubVerifier implements app.DurationVerifier for tests, recording calls.
type stubVerifier struct {
	verified  []string         // hashes passed to MarkDurationVerified
	corrected map[string]int64 // hash → new durationMs from VerifyTrackDuration
	verifyErr error
	markErr   error
}

func newStubVerifier() *stubVerifier {
	return &stubVerifier{corrected: make(map[string]int64)}
}

func (s *stubVerifier) VerifyTrackDuration(hash string, durationMs int64) error {
	if s.verifyErr != nil {
		return s.verifyErr
	}
	s.corrected[hash] = durationMs
	return nil
}

func (s *stubVerifier) MarkDurationVerified(hash string) error {
	if s.markErr != nil {
		return s.markErr
	}
	s.verified = append(s.verified, hash)
	return nil
}

// drainAndWait reads all fixes from ch until it is closed.
func drainAndWait(t *testing.T, ch <-chan library.DurationFix) []library.DurationFix {
	t.Helper()
	var fixes []library.DurationFix
	for f := range ch {
		fixes = append(fixes, f)
	}
	return fixes
}

func runAudit(t *testing.T, reader *stubReader, writer *stubVerifier) <-chan library.DurationFix {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return app.RunDurationAudit(ctx, reader, writer)
}

// TestRunDurationAudit_NoTracks verifies that the audit closes its channel
// immediately when there are no unverified tracks.
func TestRunDurationAudit_NoTracks(t *testing.T) {
	reader := &stubReader{tracks: nil}
	writer := newStubVerifier()

	ch := runAudit(t, reader, writer)
	drainAndWait(t, ch)

	if len(writer.verified) != 0 {
		t.Errorf("MarkDurationVerified calls = %d; want 0", len(writer.verified))
	}
	if len(writer.corrected) != 0 {
		t.Errorf("VerifyTrackDuration calls = %d; want 0", len(writer.corrected))
	}
}

// TestRunDurationAudit_ReadError verifies that a reader error causes the
// channel to be closed without any writer calls.
func TestRunDurationAudit_ReadError(t *testing.T) {
	reader := &stubReader{err: errors.New("db error")}
	writer := newStubVerifier()

	ch := runAudit(t, reader, writer)
	drainAndWait(t, ch)

	if len(writer.verified) != 0 || len(writer.corrected) != 0 {
		t.Error("expected no writer calls on read error")
	}
}

// TestRunDurationAudit_FFProbeFailureSkips verifies that when ffprobe fails,
// the track is skipped: no fix is sent and no writer method is called.
func TestRunDurationAudit_FFProbeFailureSkips(t *testing.T) {
	restore := app.SetProbeFn(func(_ context.Context, _ string) (float64, error) {
		return 0, errors.New("ffprobe failed")
	})
	t.Cleanup(restore)

	reader := &stubReader{tracks: []db.UnverifiedTrack{
		{Hash: "hash-a", Path: "/any/track.mp3", DurationMs: 120000},
	}}
	writer := newStubVerifier()

	ch := runAudit(t, reader, writer)
	fixes := drainAndWait(t, ch)

	if len(fixes) != 0 {
		t.Errorf("fixes sent = %d; want 0 (ffprobe fail → skip)", len(fixes))
	}
	if len(writer.verified) != 0 {
		t.Errorf("MarkDurationVerified calls = %d; want 0", len(writer.verified))
	}
	if len(writer.corrected) != 0 {
		t.Errorf("VerifyTrackDuration calls = %d; want 0", len(writer.corrected))
	}
}

// TestRunDurationAudit_CorrectionsFlowToChannel verifies that when ffprobe
// returns a duration that differs by more than 2 seconds from the stored value,
// a DurationFix is sent to the channel and VerifyTrackDuration is called.
func TestRunDurationAudit_CorrectionsFlowToChannel(t *testing.T) {
	restore := app.SetProbeFn(func(_ context.Context, path string) (float64, error) {
		// Return 300s for any path; stored value is 120s → diff 180s > 2s.
		return 300.0, nil
	})
	t.Cleanup(restore)

	reader := &stubReader{tracks: []db.UnverifiedTrack{
		{Hash: "hash-a", Path: "/track-a.mp3", DurationMs: 120000},
		{Hash: "hash-b", Path: "/track-b.mp3", DurationMs: 120000},
	}}
	writer := newStubVerifier()

	ch := runAudit(t, reader, writer)
	fixes := drainAndWait(t, ch)

	if len(fixes) != 2 {
		t.Errorf("fixes sent = %d; want 2", len(fixes))
	}
	for _, f := range fixes {
		if f.DurationMs != 300000 {
			t.Errorf("fix DurationMs = %d; want 300000", f.DurationMs)
		}
	}
	if len(writer.corrected) != 2 {
		t.Errorf("VerifyTrackDuration calls = %d; want 2", len(writer.corrected))
	}
	if len(writer.verified) != 0 {
		t.Errorf("MarkDurationVerified calls = %d; want 0", len(writer.verified))
	}
}

// TestRunDurationAudit_AlreadyCorrectTracksGetMarkedVerified verifies that
// when ffprobe confirms the stored duration (diff ≤ 2s), MarkDurationVerified
// is called and no fix is sent to the channel.
func TestRunDurationAudit_AlreadyCorrectTracksGetMarkedVerified(t *testing.T) {
	restore := app.SetProbeFn(func(_ context.Context, _ string) (float64, error) {
		// Return 120.5s; stored 120000ms → diff 500ms < 2s → verified.
		return 120.5, nil
	})
	t.Cleanup(restore)

	reader := &stubReader{tracks: []db.UnverifiedTrack{
		{Hash: "hash-ok", Path: "/track-ok.mp3", DurationMs: 120000},
	}}
	writer := newStubVerifier()

	ch := runAudit(t, reader, writer)
	fixes := drainAndWait(t, ch)

	if len(fixes) != 0 {
		t.Errorf("fixes sent = %d; want 0 (duration already correct)", len(fixes))
	}
	if len(writer.verified) != 1 || writer.verified[0] != "hash-ok" {
		t.Errorf("MarkDurationVerified calls = %v; want [hash-ok]", writer.verified)
	}
	if len(writer.corrected) != 0 {
		t.Errorf("VerifyTrackDuration calls = %d; want 0", len(writer.corrected))
	}
}

// TestRunDurationAudit_ProbeHookCanChangeWhileAuditRuns verifies that the
// test probe hook is safe to swap while audit workers are active. Without the
// probeFn lock, this trips the race detector: workers read probeFn while tests
// restore or replace it.
func TestRunDurationAudit_ProbeHookCanChangeWhileAuditRuns(t *testing.T) {
	var calls int32
	restore := app.SetProbeFn(func(_ context.Context, _ string) (float64, error) {
		atomic.AddInt32(&calls, 1)
		return 120.0, nil
	})
	t.Cleanup(restore)

	tracks := make([]db.UnverifiedTrack, 200)
	for i := range tracks {
		tracks[i] = db.UnverifiedTrack{
			Hash:       "hash",
			Path:       "/track.mp3",
			DurationMs: 120000,
		}
	}
	reader := &stubReader{tracks: tracks}
	writer := newStubVerifier()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := app.RunDurationAudit(ctx, reader, writer)

	var wg sync.WaitGroup
	wg.Go(func() {
		for range ch {
		}
	})

	for range 100 {
		restoreSwap := app.SetProbeFn(func(_ context.Context, _ string) (float64, error) {
			atomic.AddInt32(&calls, 1)
			return 120.0, nil
		})
		restoreSwap()
	}

	wg.Wait()
	if atomic.LoadInt32(&calls) == 0 {
		t.Fatal("probe hook was never called")
	}
}
