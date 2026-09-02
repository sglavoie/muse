package tui

import (
	"context"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recommendation"
)

// stubRecReader is a minimal recommendationReader for mixes tests.
type stubRecReader struct {
	sufficient bool
	tracks     []library.Track
}

func (s *stubRecReader) HasSufficientHistory(_ int) (bool, error) {
	return s.sufficient, nil
}

func (s *stubRecReader) ReadRediscoverMix(_, _, _ int) ([]library.Track, error) {
	return s.tracks, nil
}

func (s *stubRecReader) ReadHiddenGemsMix(_, _, _, _ int) ([]library.Track, error) {
	return s.tracks, nil
}

func (s *stubRecReader) ReadOnRepeatMix(_, _, _ int) ([]library.Track, error) {
	return s.tracks, nil
}

func (s *stubRecReader) ReadDeepCutsMix(_, _ int, _ float64, _ int) ([]library.Track, error) {
	return s.tracks, nil
}

func (s *stubRecReader) ReadNeverPlayedMix(_ int) ([]library.Track, error) {
	return s.tracks, nil
}

func (s *stubRecReader) ReadCompletionRates(_ []string) (map[string]float64, error) {
	return nil, nil
}

func (s *stubRecReader) ReadSessionCoOccurrences(_ string, _ int) ([]db.SessionCoOccurrence, error) {
	return nil, nil
}

func (s *stubRecReader) ReadHourGenreAffinities() ([]db.HourGenreAffinity, error) {
	return nil, nil
}

func (s *stubRecReader) ReadAllMixProgress() (map[db.MixID]db.MixProgress, error) {
	return nil, nil
}

func newMixesModel(sufficient bool, tracks []library.Track) Model {
	return New(context.Background(), WithRecommendationReader(&stubRecReader{
		sufficient: sufficient,
		tracks:     tracks,
	}))
}

func sampleMixTracks() []library.Track {
	return []library.Track{
		{Hash: "mx1", Title: "Mix Track 1", Artist: "ArtistX", Path: "/mx1.flac"},
		{Hash: "mx2", Title: "Mix Track 2", Artist: "ArtistX", Path: "/mx2.flac"},
		{Hash: "mx3", Title: "Mix Track 3", Artist: "ArtistY", Path: "/mx3.flac"},
	}
}

// stubDismissalWriter records DismissTrack and UndismissTrack calls.
type stubDismissalWriter struct {
	dismissed   []string
	undismissed []string
}

func (s *stubDismissalWriter) DismissTrack(hash string) error {
	s.dismissed = append(s.dismissed, hash)
	return nil
}

func (s *stubDismissalWriter) UndismissTrack(hash string) error {
	s.undismissed = append(s.undismissed, hash)
	return nil
}

func setupMixesAtTrackLevel(opts ...Option) Model {
	tracks := sampleMixTracks()
	allOpts := []Option{WithRecommendationReader(&stubRecReader{sufficient: true, tracks: tracks})}
	allOpts = append(allOpts, opts...)
	m := New(context.Background(), allOpts...)
	m = m.switchToView(viewMixes)
	m.mixes.CachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.SelectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})
	return m
}

func seedMixRegistry(m Model) Model {
	m.mixes.Mixes = append([]recommendation.MixInfo(nil), recommendation.Registry()...)
	return m
}
