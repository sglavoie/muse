package tui

import (
	"slices"
	"testing"

	"github.com/sglavoie/muse/internal/tui/playback"
)

// trackChangeFor builds the start-file report the player emits for queue row idx.
func trackChangeFor(m Model, idx int) playback.TrackChangedMsg {
	e := m.queue.Entries[idx]
	return playback.TrackChangedMsg{
		Hash: e.Hash, Index: idx, QueueID: e.ID, Artist: e.Artist, Title: e.Title,
	}
}

// duplicateRow makes queue row dup a second occurrence of the track already in
// row src: same file, same hash, same metadata, its own queue row id. It is the
// shape occurrence identity exists for — every other field the player reports is
// shared, so nothing but the id tells the two rows apart.
func duplicateRow(m Model, src, dup int) Model {
	e := m.queue.Entries[src]
	e.ID = m.queue.Entries[dup].ID
	m.queue.Entries[dup] = e
	return m
}

// TestLateStaleDuplicateTrackChangeCannotOverwriteNewestSelection is the same
// inverted delivery as the test above, aimed at the case the content hash cannot
// describe: the abandoned row and the selected row are two occurrences of one
// track.
//
// Hash, artist, title and path are identical for both, so a barrier keyed to the
// hash reads the abandoned occurrence's delayed start-file as the arrival of the
// selected one. It then admits the report, takes its still-in-range index as
// given, moves the current row onto the occurrence the player has already left,
// and retires the barrier the newer selection is still relying on — leaving the
// next stale report unguarded too. Only the queue row id separates them.
//
// The second subtest is the other way an index outlives its meaning: no
// duplicates at all, but the queue is reordered between the event and the
// handler, so the reported index stays in range while naming an unrelated row.
func TestLateStaleDuplicateTrackChangeCannotOverwriteNewestSelection(t *testing.T) {
	const olderTarget, newerTarget = 0, 1

	t.Run("a delayed report from the abandoned duplicate is refused", func(t *testing.T) {
		m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4
		// Rows 0 and 1 now hold the same track in two queue occurrences.
		m = duplicateRow(m, olderTarget, newerTarget)

		// The older selection reaches the player and is accepted: mpv loads that
		// occurrence and emits its start-file, which the reader is still holding.
		m, older := dispatchSelect(t, m, m.queue.FindEntryFilteredPosition(olderTarget))
		olderResult := runPlayCmd(t, older)
		// The user moves on to the *other* occurrence of the same track.
		m, newer := dispatchSelect(t, m, m.queue.FindEntryFilteredPosition(newerTarget))
		newerResult := runPlayCmd(t, newer)

		if olderResult.Err != nil || newerResult.Err != nil {
			t.Fatalf("test setup: both loads must be accepted, got %v / %v",
				olderResult.Err, newerResult.Err)
		}
		if olderResult.Hash != newerResult.Hash {
			t.Fatal("test setup: the two selections must name the same track")
		}
		if !slices.Equal(p.playFromIndexCalls, []int{olderTarget, newerTarget}) {
			t.Fatalf("PlayFromIndex calls = %v, want both requests to reach the player in order",
				p.playFromIndexCalls)
		}

		m = m.handlePlayFromIndexResultMsg(newerResult)
		m = m.handlePlayFromIndexResultMsg(olderResult) // superseded, discarded

		// The abandoned occurrence's start-file finally arrives.
		m, _ = m.handleTrackChangedMsg(trackChangeFor(m, olderTarget))
		if m.playback.currentIndex != newerTarget {
			t.Errorf("currentIndex = %d, want the newest selection's occurrence %d",
				m.playback.currentIndex, newerTarget)
		}
		if !m.playback.awaitingSelectionStart {
			t.Error("the abandoned occurrence's report must not retire the newer barrier")
		}
		if tr := p.QueueTraversal(); tr.Order[tr.CurrentPosition] != newerTarget {
			t.Errorf("player is on queue row %d, want the newest request's row %d",
				tr.Order[tr.CurrentPosition], newerTarget)
		}

		// The selected occurrence's own report still lands and retires the barrier.
		m, _ = m.handleTrackChangedMsg(trackChangeFor(m, newerTarget))
		if m.playback.currentIndex != newerTarget {
			t.Errorf("currentIndex = %d, want %d", m.playback.currentIndex, newerTarget)
		}
		if m.playback.awaitingSelectionStart {
			t.Error("the selected occurrence's own start-file must retire the barrier")
		}
	})

	t.Run("a reorder cannot make a stale index name another row", func(t *testing.T) {
		m, _ := replayModel([]int{3, 1, 4, 0, 2}, 2)
		const target = 2
		// Row 0 holds the same track in an occurrence of its own, so a search that
		// falls back to the content hash settles on it instead of the row that
		// actually started.
		m = duplicateRow(m, target, 0)

		m, cmd := dispatchSelect(t, m, m.queue.FindEntryFilteredPosition(target))
		m = m.handlePlayFromIndexResultMsg(runPlayCmd(t, cmd))

		// mpv emits the start-file for the selected row, and the user drags that
		// row to the end of the queue before the reader dispatches it. The report
		// still says index 2, which is in range and now holds a different track.
		report := trackChangeFor(m, target)
		moved := m.queue.Entries[target]
		m.queue.Entries = append(
			slices.Delete(slices.Clone(m.queue.Entries), target, target+1), moved)
		want := len(m.queue.Entries) - 1

		m, _ = m.handleTrackChangedMsg(report)

		if m.playback.currentIndex != want {
			t.Errorf("currentIndex = %d, want the moved row's new position %d",
				m.playback.currentIndex, want)
		}
		if m.playback.currentHash != moved.Hash {
			t.Errorf("currentHash = %q, want the started row's %q", m.playback.currentHash, moved.Hash)
		}
		if m.playback.awaitingSelectionStart {
			t.Error("the selected row's own start-file must retire the barrier after a reorder")
		}
	})
}

// TestReplaySelectionDistinguishesDuplicateOccurrence covers what must keep
// working once occurrence identity guards the barrier: the player's report of
// the occurrence it is actually on still lands, a reconnect re-emitting it does
// not slide the current row onto the other copy, Enter on it is still a replay,
// and Enter on the other copy is still a fresh selection.
func TestReplaySelectionDistinguishesDuplicateOccurrence(t *testing.T) {
	m, p := replayModel([]int{0, 1, 2}, 0) // playing queue row 0
	// Row 2 becomes a second occurrence of row 0's track.
	m = duplicateRow(m, 0, 2)
	const target = 2

	m, cmd := dispatchSelect(t, m, m.queue.FindEntryFilteredPosition(target))
	m = m.handlePlayFromIndexResultMsg(runPlayCmd(t, cmd))
	m, _ = m.handleTrackChangedMsg(trackChangeFor(m, target))
	if m.playback.currentIndex != target {
		t.Fatalf("currentIndex = %d, want the selected occurrence %d", m.playback.currentIndex, target)
	}

	// mpv reconnects and re-emits the occurrence it is holding. The first copy of
	// the track sits earlier in the queue and must not attract the current row.
	m, _ = m.handleTrackChangedMsg(trackChangeFor(m, target))
	if m.playback.currentIndex != target {
		t.Errorf("currentIndex = %d after a reconnect re-emit, want %d", m.playback.currentIndex, target)
	}

	// Enter on the confirmed occurrence restarts it without moving the timeline.
	m, result := runSelect(t, m, m.queue.FindEntryFilteredPosition(target))
	if !result.Replay || p.replayCalls != 1 {
		t.Errorf("replay = %v after %d ReplayCurrent calls, want a replay of the current occurrence",
			result.Replay, p.replayCalls)
	}

	// Enter on the other copy of the same track is a genuine selection.
	_, result = runSelect(t, m, m.queue.FindEntryFilteredPosition(0))
	if result.Replay {
		t.Error("selecting the other occurrence of the current track is not a replay")
	}
	if !slices.Equal(p.playFromIndexCalls, []int{target, 0}) {
		t.Errorf("PlayFromIndex calls = %v, want [%d 0]", p.playFromIndexCalls, target)
	}
}

// assertCurrentRow checks the TUI is showing queue row idx as the current track.
func assertCurrentRow(t *testing.T, m Model, idx int) {
	t.Helper()
	if m.playback.currentIndex != idx {
		t.Errorf("currentIndex = %d, want %d", m.playback.currentIndex, idx)
	}
	if want := m.queue.Entries[idx].Hash; m.playback.currentHash != want {
		t.Errorf("currentHash = %q, want row %d's %q", m.playback.currentHash, idx, want)
	}
	if m.playback.state != playback.StatePlaying {
		t.Errorf("state = %v, want playing", m.playback.state)
	}
}

// TestPendingSelectionIsNotMisclassifiedAsReplay covers the other half of the
// race: Enter on a row the player has not been told about yet. The optimistic
// update writes the selected row into currentIndex/currentHash immediately, so a
// second Enter arriving before the player answers would match it and go to
// ReplayCurrent — which restarts the entry the player is still holding, i.e. the
// previous track, not the one the user pointed at twice.
func TestPendingSelectionIsNotMisclassifiedAsReplay(t *testing.T) {
	m, p := replayModel([]int{3, 1, 4, 0, 2}, 2) // playing queue row 4

	// Two Enters on row 2. Neither has reached the player: the first request's
	// goroutine has not run, so the player is still sitting on row 4.
	m, first := dispatchSelect(t, m, otherDisplayRow)
	if !m.playback.pendingSelection {
		t.Fatal("a dispatched selection must be marked unconfirmed")
	}
	m, second := dispatchSelect(t, m, otherDisplayRow)

	// The second request reaches the player first, which supersedes the first
	// one: it is answered without ever being sent.
	secondResult := runPlayCmd(t, second)
	firstResult := runPlayCmd(t, first)

	if p.replayCalls != 0 {
		t.Errorf("ReplayCurrent calls = %d, want 0 — the selected row was never confirmed", p.replayCalls)
	}
	if secondResult.Replay || firstResult.Replay {
		t.Error("neither request may report itself as a replay")
	}
	if !slices.Equal(p.playFromIndexCalls, []int{2}) {
		t.Errorf("PlayFromIndex calls = %v, want only the newest press to reach the player",
			p.playFromIndexCalls)
	}
	// The point of the whole guard: row 4 must never be restarted. A misclassified
	// replay would have loaded it, because that is where the player still was.
	if slices.Contains(p.loaded, 4) {
		t.Errorf("loaded = %v, want the previously playing row 4 left alone", p.loaded)
	}

	// Once the newest request is answered the selection is confirmed, and Enter on
	// the row is a replay again — the guard covers the unconfirmed window only.
	m = m.handlePlayFromIndexResultMsg(secondResult)
	if m.playback.pendingSelection {
		t.Fatal("the latest request's result must confirm the selection")
	}
	m, third := dispatchSelect(t, m, m.queue.FindEntryFilteredPosition(2))
	thirdResult := runPlayCmd(t, third)

	if !thirdResult.Replay || p.replayCalls != 1 {
		t.Errorf("replay = %v after %d ReplayCurrent calls, want a replay of the confirmed row",
			thirdResult.Replay, p.replayCalls)
	}
	if !slices.Equal(p.playFromIndexCalls, []int{2}) {
		t.Errorf("PlayFromIndex calls = %v, want the confirmed row replayed instead of reanchored",
			p.playFromIndexCalls)
	}

	// The abandoned first request finally reports back. It names the same row as
	// everything since, so only its identity marks it as spent.
	m = m.handlePlayFromIndexResultMsg(firstResult)
	if m.playback.currentIndex != 2 || m.playback.state != playback.StatePlaying {
		t.Errorf("after the abandoned result: currentIndex = %d state = %v; want 2 playing",
			m.playback.currentIndex, m.playback.state)
	}
}

// TestTrackChangedConfirmsPendingSelection verifies the player's own report of
// the row it started also closes the unconfirmed window. It is the confirmation
// that arrives in normal playback — the start-file event — and without it a
// selection whose result message is still in flight would leave every later
// Enter on that row reanchoring the shuffle cycle instead of replaying.
func TestTrackChangedConfirmsPendingSelection(t *testing.T) {
	m, p := replayModel([]int{3, 1, 4, 0, 2}, 2)

	m, cmd := dispatchSelect(t, m, otherDisplayRow)
	if !m.playback.pendingSelection {
		t.Fatal("a dispatched selection must be marked unconfirmed")
	}
	runPlayCmd(t, cmd)

	entry := m.queue.Entries[2]
	m, _ = m.handleTrackChangedMsg(playback.TrackChangedMsg{
		Hash: entry.Hash, Index: 2, Artist: entry.Artist, Title: entry.Title,
	})
	if m.playback.pendingSelection {
		t.Fatal("the player's start-file report must confirm the selection")
	}

	_, result := runSelect(t, m, m.queue.FindEntryFilteredPosition(2))
	if !result.Replay || p.replayCalls != 1 {
		t.Errorf("replay = %v after %d ReplayCurrent calls, want a replay once the row is confirmed",
			result.Replay, p.replayCalls)
	}
}
