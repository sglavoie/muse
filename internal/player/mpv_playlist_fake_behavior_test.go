package player

import (
	"slices"
	"testing"
)

// The tests in this file pin the fake playlist's own semantics. Every assertion
// the transaction tests make about mpv's behaviour is only as trustworthy as the
// model below, so the model is tested directly.

// TestFakePlaylist_ReplaceResetsPlaylist verifies that `loadfile ... replace`
// discards the entire playlist — including a prefetched successor — and leaves
// the new file as the only, current entry. This is why a successful replace is
// authoritative prefetch invalidation and must not be followed by a removal.
func TestFakePlaylist_ReplaceResetsPlaylist(t *testing.T) {
	f := newFakePlaylist(t, "fp-replace")
	c := newFakePlaylistClient(t, f)

	mustSend(t, c, cmdLoadfile, "a.flac", "append-play")
	mustSend(t, c, cmdLoadfile, "b.flac", "append")
	if got := f.entriesSnapshot(); !slices.Equal(got, []string{"a.flac", "b.flac"}) {
		t.Fatalf("entries = %v, want [a.flac b.flac]", got)
	}
	if got := f.currentPos(); got != 0 {
		t.Fatalf("currentPos = %d, want 0", got)
	}

	mustSend(t, c, cmdLoadfile, "c.flac", "replace", 0, "pause=no")

	if got := f.entriesSnapshot(); !slices.Equal(got, []string{"c.flac"}) {
		t.Fatalf("entries after replace = %v, want [c.flac]", got)
	}
	if got := f.currentPos(); got != 0 {
		t.Fatalf("currentPos after replace = %d, want 0", got)
	}
	if got := f.playingPath(); got != "c.flac" {
		t.Fatalf("playingPath after replace = %q, want c.flac", got)
	}
}

// TestFakePlaylist_AppendPlayStartsIdlePlaylist verifies the difference between
// `append` and `append-play` on an idle playlist: only append-play starts the
// entry it adds. That is the property prefetch relies on to recover a playlist
// that reached its end before Go noticed.
func TestFakePlaylist_AppendPlayStartsIdlePlaylist(t *testing.T) {
	f := newFakePlaylist(t, "fp-appendplay")
	c := newFakePlaylistClient(t, f)

	if !f.idle() {
		t.Fatal("a fresh playlist should be idle")
	}

	// Plain append into an idle playlist parks the entry without playing it.
	mustSend(t, c, cmdLoadfile, "a.flac", "append")
	if !f.idle() {
		t.Fatalf("append should leave an idle playlist idle; currentPos = %d", f.currentPos())
	}

	// append-play into the same idle playlist starts what it appends.
	mustSend(t, c, cmdLoadfile, "b.flac", "append-play")
	if f.idle() {
		t.Fatal("append-play should start playback on an idle playlist")
	}
	if got := f.playingPath(); got != "b.flac" {
		t.Fatalf("playingPath = %q, want b.flac", got)
	}

	// append-play into a playing playlist only appends; the current entry stays.
	mustSend(t, c, cmdLoadfile, "c.flac", "append-play")
	if got := f.playingPath(); got != "b.flac" {
		t.Fatalf("playingPath after second append-play = %q, want b.flac", got)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, []string{"a.flac", "b.flac", "c.flac"}) {
		t.Fatalf("entries = %v, want [a.flac b.flac c.flac]", got)
	}
}

// TestFakePlaylist_RemoveShiftsCurrentAndRejectsOutOfRange covers removal: an
// out-of-range index is an error, removing before the current entry shifts it
// down, and removing the current entry itself leaves the playlist idle.
func TestFakePlaylist_RemoveShiftsCurrentAndRejectsOutOfRange(t *testing.T) {
	f := newFakePlaylist(t, "fp-remove")
	c := newFakePlaylistClient(t, f)

	mustSend(t, c, cmdLoadfile, "a.flac", "append-play")
	mustSend(t, c, cmdLoadfile, "b.flac", "append")
	mustSend(t, c, cmdLoadfile, "c.flac", "append")
	f.finishCurrent() // gapless transition to b.flac
	if got := f.currentPos(); got != 1 {
		t.Fatalf("currentPos after finishing a.flac = %d, want 1", got)
	}

	if _, err := c.SendCommand(cmdPlaylistRemove, 9); err == nil {
		t.Fatal("playlist-remove with an out-of-range index should fail")
	}
	if got := f.entriesSnapshot(); len(got) != 3 {
		t.Fatalf("a rejected removal must not mutate the playlist; entries = %v", got)
	}

	// Removing the completed entry ahead of the current one shifts it down.
	mustSend(t, c, cmdPlaylistRemove, 0)
	if got := f.entriesSnapshot(); !slices.Equal(got, []string{"b.flac", "c.flac"}) {
		t.Fatalf("entries = %v, want [b.flac c.flac]", got)
	}
	if got := f.currentPos(); got != 0 {
		t.Fatalf("currentPos = %d, want 0 (shifted down)", got)
	}

	// Removing the successor slot leaves the current entry playing.
	mustSend(t, c, cmdPlaylistRemove, 1)
	if got := f.playingPath(); got != "b.flac" {
		t.Fatalf("playingPath = %q, want b.flac", got)
	}

	// Removing the current entry stops playback.
	mustSend(t, c, cmdPlaylistRemove, 0)
	if !f.idle() {
		t.Fatalf("removing the current entry should leave the playlist idle; currentPos = %d", f.currentPos())
	}
	if got := f.entriesSnapshot(); len(got) != 0 {
		t.Fatalf("entries = %v, want empty", got)
	}
}

// TestFakePlaylist_EmitsStartAndEndFileEvents pins the async event stream: a
// replacement ends the outgoing file with reason "replace" (which must not
// auto-advance), and reaching the end of a file emits "eof" followed by a
// start-file only when a successor exists.
func TestFakePlaylist_EmitsStartAndEndFileEvents(t *testing.T) {
	f := newFakePlaylist(t, "fp-events")
	events := f.subscribe(t)
	c := newFakePlaylistClient(t, f)

	mustSend(t, c, cmdLoadfile, "a.flac", "append-play")
	mustSend(t, c, cmdLoadfile, "b.flac", "replace", 0, "pause=no")
	mustSend(t, c, cmdLoadfile, "c.flac", "append")
	f.finishCurrent() // b.flac -> c.flac
	f.finishCurrent() // c.flac -> idle

	want := []string{
		"start-file",       // append-play started a.flac
		"end-file:replace", // a.flac displaced by b.flac
		"start-file",       // b.flac
		"end-file:eof",     // b.flac finished
		"start-file",       // gapless transition to c.flac
		"end-file:eof",     // c.flac finished, nothing left
	}
	for i, w := range want {
		if got := nextEvent(t, events); got != w {
			t.Fatalf("event %d = %q, want %q", i, got, w)
		}
	}
	if !f.idle() {
		t.Fatal("playlist should be idle after the last entry finished")
	}
}

// TestFakePlaylist_ForcedCommandFailures verifies that an injected failure is
// reported to the caller and leaves the modelled playlist untouched, and that
// one-shot failures apply to exactly one command.
func TestFakePlaylist_ForcedCommandFailures(t *testing.T) {
	f := newFakePlaylist(t, "fp-fail")
	c := newFakePlaylistClient(t, f)

	f.failCommandOnce("loadfile:append-play", "boom")
	if _, err := c.SendCommand(cmdLoadfile, "a.flac", "append-play"); err == nil {
		t.Fatal("append-play should have failed")
	}
	if got := f.entriesSnapshot(); len(got) != 0 {
		t.Fatalf("a failed command must not mutate the playlist; entries = %v", got)
	}
	// The one-shot failure is spent, so the retry lands.
	mustSend(t, c, cmdLoadfile, "a.flac", "append-play")
	if got := f.playingPath(); got != "a.flac" {
		t.Fatalf("playingPath = %q, want a.flac", got)
	}

	f.failCommand("loadfile:replace", "device error")
	if _, err := c.SendCommand(cmdLoadfile, "b.flac", "replace", 0, "pause=no"); err == nil {
		t.Fatal("replace should have failed")
	}
	if _, err := c.SendCommand(cmdLoadfile, "b.flac", "replace", 0, "pause=no"); err == nil {
		t.Fatal("a persistent failure should apply to every matching command")
	}
	if got := f.playingPath(); got != "a.flac" {
		t.Fatalf("playingPath after failed replaces = %q, want a.flac", got)
	}

	f.clearFailure("loadfile:replace")
	mustSend(t, c, cmdLoadfile, "b.flac", "replace", 0, "pause=no")
	if got := f.playingPath(); got != "b.flac" {
		t.Fatalf("playingPath = %q, want b.flac", got)
	}
}

// TestFakePlaylist_BarrierParksCommandUntilReleased verifies the deterministic
// barrier the concurrency tests depend on: a matching command reaches the fake,
// is observable as arrived, and applies no state change until released.
func TestFakePlaylist_BarrierParksCommandUntilReleased(t *testing.T) {
	f := newFakePlaylist(t, "fp-gate")
	c := newFakePlaylistClient(t, f)

	gate := f.block(t, "loadfile:append-play")
	done := make(chan error, 1)
	go func() {
		_, err := c.SendCommand(cmdLoadfile, "a.flac", "append-play")
		done <- err
	}()

	gate.waitArrived(t, "append-play")
	if got := f.entriesSnapshot(); len(got) != 0 {
		t.Fatalf("a parked command must not have applied yet; entries = %v", got)
	}
	select {
	case err := <-done:
		t.Fatalf("SendCommand returned while parked at the barrier: %v", err)
	default:
	}

	f.unblock("loadfile:append-play")
	if err := <-done; err != nil {
		t.Fatalf("SendCommand after release: %v", err)
	}
	if got := f.playingPath(); got != "a.flac" {
		t.Fatalf("playingPath = %q, want a.flac", got)
	}
}
