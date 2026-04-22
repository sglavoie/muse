package tui

import (
	"context"

	"github.com/sglavoie/muse/internal/library"
)

// stubPreviewer is a minimal Previewer implementation for testing.
type stubPreviewer struct {
	startCalled    bool
	loadPath       string
	closeCalled    bool
	togglePauseErr error
	seekErr        error
	toggleCalls    int
	seekCalls      []float64
	elapsed, total float64
	elapsedErr     error
}

func (s *stubPreviewer) Start() error { s.startCalled = true; return nil }
func (s *stubPreviewer) LoadAndPlay(path string) error {
	s.loadPath = path
	return nil
}
func (s *stubPreviewer) TogglePause() error {
	s.toggleCalls++
	return s.togglePauseErr
}
func (s *stubPreviewer) SeekRelative(seconds float64) error {
	s.seekCalls = append(s.seekCalls, seconds)
	return s.seekErr
}
func (s *stubPreviewer) ElapsedPosition() (float64, float64, error) {
	return s.elapsed, s.total, s.elapsedErr
}
func (s *stubPreviewer) Close() error { s.closeCalled = true; return nil }

type stubPreviewMainPlayer struct {
	pauseCalls int
	seekCalls  []float64
}

func (s *stubPreviewMainPlayer) Play() error                { return nil }
func (s *stubPreviewMainPlayer) Pause() error               { s.pauseCalls++; return nil }
func (s *stubPreviewMainPlayer) Stop() error                { return nil }
func (s *stubPreviewMainPlayer) SeekAbsolute(float64) error { return nil }
func (s *stubPreviewMainPlayer) SeekRelative(seconds float64) error {
	s.seekCalls = append(s.seekCalls, seconds)
	return nil
}
func (s *stubPreviewMainPlayer) SetVolumeRelative(float64) error { return nil }
func (s *stubPreviewMainPlayer) Next() error                     { return nil }
func (s *stubPreviewMainPlayer) Previous() error                 { return nil }
func (s *stubPreviewMainPlayer) PlayFromIndex(int) error         { return nil }

// setupLibraryWithTrack creates a model with a single track in the library.
func setupLibraryWithTrack(t interface{ Helper() }) Model {
	t.Helper()
	m := New(context.Background())
	m.activeView = viewLibrary
	m.library.index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T",
	})
	return m
}

// errTest is a simple error type for test assertions.
type errTest string

func (e errTest) Error() string { return string(e) }
