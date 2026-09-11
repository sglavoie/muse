package tui

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/playback"
)

type selectionListenPlayer struct {
	*fakeStoppablePlayer
	elapsed float64
	total   float64
}

func (p *selectionListenPlayer) ElapsedPosition() (float64, float64, error) {
	return p.elapsed, p.total, nil
}

func heardSelectionSeconds(m Model, p *selectionListenPlayer, seconds int) Model {
	for range seconds {
		p.elapsed++
		m, _ = m.handleTickMsg()
	}
	return m
}

func TestStaleSelectionReconciliationDoesNotDoubleCountTheHeldListen(t *testing.T) {
	for _, tc := range []struct {
		before, after, refusals int
		counted                 bool
	}{
		{40, 40, 1, true}, // each half independently counts
		{40, 40, 2, true}, // another refused jump must keep the same listen
		{20, 20, 1, true}, // only the combined measurement counts
		{2, 2, 1, false},  // neither half clears the capture floor alone
	} {
		for _, resultFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%+v/refusalFirst=%v", tc, resultFirst), func(t *testing.T) {
				m, base := stoppableReplayModel([]int{3, 1, 4, 0, 2}, 2)
				p := &selectionListenPlayer{fakeStoppablePlayer: base, total: 80}
				m.player = p
				entry := m.queue.Entries[4]
				hr := &stubHistoryRecorder{}
				m.historyRecorder = hr
				WithLibraryTracks([]library.Track{{Hash: entry.Hash, Artist: entry.Artist, Duration: p.total}})(&m)
				m = m.armPendingListen(entry.Hash, entry.Artist)
				m = heardSelectionSeconds(m, p, tc.before)
				for range tc.refusals {
					p.playFromIndexErrs = []error{errStubPlay}
					next, selectCmd := m.Update(playback.PlayQueueIndexMsg{Index: 1})
					m = next.(Model)
					next, pauseCmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
					m = next.(Model)
					request := m.playback.requestSeq
					refusal := runPlayCmd(t, selectCmd)
					if !errors.Is(refusal.Err, errStubPlay) {
						t.Fatalf("selection result = %+v", refusal)
					}
					pause := pauseCmd()
					if resultFirst {
						m = djWakeDrive(m, refusal, 4)
						m = djWakeDrive(m, pause, 4)
					} else {
						m = djWakeDrive(m, pause, 4)
						m = djWakeDrive(m, refusal, 4)
					}
					if m.playback.requestSeq != request || m.playback.playRequestSeq != refusal.Request || p.stopCalls != 0 {
						t.Fatal("reconciliation stopped or superseded the newer control")
					}
					if m.playback.pending.hash != entry.Hash || !m.playback.pending.armed || m.playback.listenedSecs != float64(tc.before) {
						t.Fatalf("outgoing listen not restored: %+v, heard=%v", m.playback.pending, m.playback.listenedSecs)
					}
					if len(hr.calls) != 0 || m.sessionTracksPlayed != 0 {
						t.Fatal("unconfirmed listen boundary was counted")
					}
					m = djWakeDrive(m, tea.KeyPressMsg{Code: tea.KeySpace}, 4)
					if p.paused || p.order[p.pos] != 4 {
						t.Fatal("resume did not retain the held row")
					}
				}
				m = heardSelectionSeconds(m, p, tc.after)
				m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
				// Repeated end sites must not add another record.
				m = djWakeDrive(m, playback.PlaybackEndedMsg{}, 4)
				if len(hr.calls) != 1 {
					t.Fatalf("persisted listens = %+v, want exactly one", hr.calls)
				}
				rec := hr.calls[0]
				if rec.Hash != entry.Hash || rec.Counted != tc.counted || rec.ListenedSecs != tc.before+tc.after || rec.DurationSecs != p.total {
					t.Fatalf("persisted record lost or split measurement: %+v", rec)
				}
				wantCount := 0
				if tc.counted {
					wantCount = 1
				}
				track, _ := m.library.Index.FindByHash(entry.Hash)
				if track.PlayCount != wantCount || m.sessionTracksPlayed != wantCount || m.mixes.PlaysSinceRefresh != wantCount {
					t.Fatalf("count mirrors disagree: library=%d session=%d mixes=%d", track.PlayCount, m.sessionTracksPlayed, m.mixes.PlaysSinceRefresh)
				}
				if tc.counted && (m.sessionListenSecs != float64(rec.ListenedSecs) || track.LastPlayedAt != rec.PlayedAt || !slices.Equal(m.radio.recentHashes, []string{entry.Hash})) {
					t.Fatalf("counted mirrors lost total time or recent history: seconds=%v track=%+v recent=%v", m.sessionListenSecs, track, m.radio.recentHashes)
				}
				if !tc.counted && len(m.radio.recentHashes) != 0 {
					t.Fatal("uncounted listen entered recent history")
				}
			})
		}
	}
}

func TestSelectionListenConfirmedBoundaries(t *testing.T) {
	for _, boundary := range []string{"result", "track event", "stop", "quit", "different occurrence"} {
		t.Run(boundary, func(t *testing.T) {
			m, p := stoppableReplayModel([]int{0, 1, 2, 3, 4}, 0)
			hr := &stubHistoryRecorder{}
			m.historyRecorder = hr
			entry := m.queue.Entries[0]
			m = m.armPendingListen(entry.Hash, entry.Artist)
			m.playback.listenedSecs = 40
			m.playback.peakTotalSecs = 100
			next, cmd := m.Update(playback.PlayQueueIndexMsg{Index: 2})
			m = next.(Model)
			result := runPlayCmd(t, cmd)
			switch boundary {
			case "result":
				m = djWakeDrive(m, result, 4)
				m = djWakeDrive(m, trackChangeFor(m, 2), 4)
			case "track event":
				m = djWakeDrive(m, trackChangeFor(m, 2), 4)
				m = djWakeDrive(m, result, 4)
			case "stop":
				m = djWakeDrive(m, tea.KeyPressMsg{Code: 's', Text: "s"}, 4)
			case "quit":
				rec, ok := m.TakePendingListen()
				if !ok {
					t.Fatal("quit lost suspended listen")
				}
				_ = hr.RecordListen(rec)
				if _, ok := m.TakePendingListen(); ok {
					t.Fatal("quit captured suspended listen twice")
				}
			case "different occurrence":
				// A newer control has moved to another occurrence of the same
				// hash. Hash equality must not turn this into a continuation.
				m.queue.Entries[1].Hash = entry.Hash
				p.pos = 1
				m = m.restorePlaybackDisplayFromPlayer()
				var flush tea.Cmd
				m, flush = m.drainListenFlushes()
				drainCmd(flush)
				if m.playback.listenedSecs != 0 || !m.playback.pending.armed {
					t.Fatal("different occurrence inherited outgoing listen")
				}
			}
			if len(hr.calls) != 1 || hr.calls[0].Hash != entry.Hash || hr.calls[0].ListenedSecs != 40 || !hr.calls[0].Counted {
				t.Fatalf("boundary lost or duplicated outgoing listen: %+v", hr.calls)
			}
		})
	}
}

func TestCoalescedSelectionsRetainOutgoingListen(t *testing.T) {
	m, p := stoppableReplayModel([]int{0, 1, 2, 3, 4}, 0)
	hr := &stubHistoryRecorder{}
	m.historyRecorder = hr
	entry := m.queue.Entries[0]
	m = m.armPendingListen(entry.Hash, entry.Artist)
	m.playback.listenedSecs = 40
	m.playback.peakTotalSecs = 100
	next, old := m.Update(playback.PlayQueueIndexMsg{Index: 1})
	m = next.(Model)
	next, latest := m.Update(playback.PlayQueueIndexMsg{Index: 2})
	m = next.(Model)
	next, pause := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = next.(Model)
	cancelled := runPlayCmd(t, old)
	if cancelled.Ran || cancelled.Err != nil {
		t.Fatalf("coalesced result = %+v", cancelled)
	}
	m = djWakeDrive(m, cancelled, 4)
	// A delayed report from the held row must not commit the saved listen.
	m = djWakeDrive(m, trackChangeFor(m, 0), 4)
	p.playFromIndexErrs = []error{errStubPlay}
	refusal := runPlayCmd(t, latest)
	m = djWakeDrive(m, pause(), 4)
	m = djWakeDrive(m, refusal, 4)
	if len(hr.calls) != 0 || !m.playback.pending.armed || m.playback.pending.hash != entry.Hash || m.playback.listenedSecs != 40 {
		t.Fatalf("coalesced selection or rejected event ended the listen: records=%+v pending=%+v", hr.calls, m.playback.pending)
	}
}
