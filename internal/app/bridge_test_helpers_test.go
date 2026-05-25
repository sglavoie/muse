package app

import (
	"sync"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

type stubUpsertBatcher struct {
	mu      sync.Mutex
	batches [][]library.Track
}

func (s *stubUpsertBatcher) UpsertBatch(tracks []library.Track) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]library.Track, len(tracks))
	copy(cp, tracks)
	s.batches = append(s.batches, cp)
	return nil
}

func (s *stubUpsertBatcher) allTracks() []library.Track {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []library.Track
	for _, b := range s.batches {
		all = append(all, b...)
	}
	return all
}

type flakyUpsertBatcher struct {
	mu        sync.Mutex
	failures  int
	err       error
	attempts  [][]library.Track
	persisted [][]library.Track
}

func (s *flakyUpsertBatcher) UpsertBatch(tracks []library.Track) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]library.Track, len(tracks))
	copy(cp, tracks)
	s.attempts = append(s.attempts, cp)
	if s.failures > 0 {
		s.failures--
		return s.err
	}
	s.persisted = append(s.persisted, cp)
	return nil
}

func assertTrackHashes(t *testing.T, tracks []library.Track, want []string) {
	t.Helper()
	if len(tracks) != len(want) {
		t.Fatalf("track count = %d, want %d", len(tracks), len(want))
	}
	for i, hash := range want {
		if tracks[i].Hash != hash {
			t.Fatalf("track %d hash = %q, want %q", i, tracks[i].Hash, hash)
		}
	}
}

type stubGenreWriter struct {
	stubUpsertBatcher
	upsertErr       error
	populateBatches [][]library.Track
}

func (s *stubGenreWriter) UpsertBatch(tracks []library.Track) error {
	err := s.stubUpsertBatcher.UpsertBatch(tracks)
	if err != nil {
		return err
	}
	return s.upsertErr
}

func (s *stubGenreWriter) PopulateTrackGenres(tracks []library.Track) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]library.Track, len(tracks))
	copy(cp, tracks)
	s.populateBatches = append(s.populateBatches, cp)
	return nil
}

type stubMissingMarker struct {
	mu     sync.Mutex
	hashes []string
}

func (s *stubMissingMarker) MarkTrackMissing(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hashes = append(s.hashes, hash)
	return nil
}
