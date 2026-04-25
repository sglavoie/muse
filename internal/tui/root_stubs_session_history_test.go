package tui

// stubSessionWriter records WriteRepeatMode, WriteShuffle,
// UpdateSessionPosition, and WriteVolume calls for assertion in tests.
type stubSessionWriter struct {
	calls         []string
	shuffleCalls  []bool
	positionCalls []struct {
		hash       string
		positionMs int64
	}
	volumeCalls []int
}

func (s *stubSessionWriter) WriteRepeatMode(mode string) error {
	s.calls = append(s.calls, mode)
	return nil
}

func (s *stubSessionWriter) WriteShuffle(enabled bool) error {
	s.shuffleCalls = append(s.shuffleCalls, enabled)
	return nil
}

func (s *stubSessionWriter) UpdateSessionPosition(hash string, positionMs int64) error {
	s.positionCalls = append(s.positionCalls, struct {
		hash       string
		positionMs int64
	}{hash, positionMs})
	return nil
}

func (s *stubSessionWriter) WriteVolume(volume int) error {
	s.volumeCalls = append(s.volumeCalls, volume)
	return nil
}

// stubHistoryRecorder records RecordPlay calls for assertion in tests.
type stubHistoryRecorder struct {
	calls []struct {
		hash       string
		playedAt   int64
		listenedSs int
	}
}

func (s *stubHistoryRecorder) RecordPlay(hash string, playedAt int64, listenedSecs int) error {
	s.calls = append(s.calls, struct {
		hash       string
		playedAt   int64
		listenedSs int
	}{hash, playedAt, listenedSecs})
	return nil
}

type stubDurationWriter struct {
	calls []struct {
		hash       string
		durationMs int64
	}
}

func (s *stubDurationWriter) UpdateTrackDuration(hash string, durationMs int64) error {
	s.calls = append(s.calls, struct {
		hash       string
		durationMs int64
	}{hash, durationMs})
	return nil
}