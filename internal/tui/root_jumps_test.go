package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/playback"
)

var (
	jumpBackKey    = tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
	jumpForwardKey = tea.KeyPressMsg{Code: 'i', Mod: tea.ModCtrl}
)

func jumpTestPress(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	return browsingTestDrain(t, next.(Model), cmd)
}

func jumpTestRune(t *testing.T, m Model, r rune) Model {
	t.Helper()
	return jumpTestPress(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
}

func jumpTestFixture() Model {
	m := newRichFixture()
	return m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
}

func TestJumpKeysResolve(t *testing.T) {
	if got := jumpBackKey.String(); got != "ctrl+o" {
		t.Fatalf("ctrl+o reported as %q", got)
	}
	if got := jumpForwardKey.String(); got != "ctrl+i" {
		t.Fatalf("ctrl+i reported as %q", got)
	}
	m := jumpTestFixture()
	if a := m.resolveKey("ctrl+o"); a != keys.ActionJumpBack {
		t.Fatalf("ctrl+o resolves to %v", a)
	}
	if a := m.resolveKey("ctrl+i"); a != keys.ActionJumpForward {
		t.Fatalf("ctrl+i resolves to %v", a)
	}
}

func TestJumpListWalksViewsBackAndForward(t *testing.T) {
	m := jumpTestFixture()
	if m.ActiveView != viewLibrary {
		t.Fatalf("fixture starts on %v", m.ActiveView)
	}
	m = jumpTestRune(t, m, '4')
	m = jumpTestRune(t, m, '2')
	if m.ActiveView != viewPlaylists {
		t.Fatalf("expected playlists, got %v", m.ActiveView)
	}

	steps := []struct {
		key  tea.KeyPressMsg
		want viewID
	}{
		{jumpBackKey, viewBrowse},
		{jumpBackKey, viewLibrary},
		{jumpForwardKey, viewBrowse},
		{jumpForwardKey, viewPlaylists},
	}
	for i, s := range steps {
		m = jumpTestPress(t, m, s.key)
		if m.ActiveView != s.want {
			t.Fatalf("step %d: want view %v, got %v", i, s.want, m.ActiveView)
		}
	}
	// Nothing left in either direction.
	m = jumpTestPress(t, m, jumpForwardKey)
	if m.ActiveView != viewPlaylists {
		t.Fatalf("forward past the newest jump moved to %v", m.ActiveView)
	}
}

func TestJumpBackAtOldestStays(t *testing.T) {
	m := jumpTestFixture()
	m = jumpTestPress(t, m, jumpBackKey)
	if m.ActiveView != viewLibrary {
		t.Fatalf("empty jump list moved to %v", m.ActiveView)
	}
}

func TestJumpListReturnsIntoCollections(t *testing.T) {
	m := jumpTestFixture()
	m.browse.Mode = browse.BrowseModeArtists
	m = jumpTestRune(t, m, '4')
	m = browsingTestEnter(t, m) // artist → albums
	albums := m.captureBrowsingSession()
	m = browsingTestEnter(t, m) // album → tracks
	if len(m.browsingKeys()) > 1 {
		p := m.browsingPosition()
		p.Key, p.Cursor = m.browsingKeys()[1], 1
		m.restoreBrowsingPosition(p)
	}
	tracks := m.captureBrowsingSession()
	if len(tracks.Steps) != 3 {
		t.Fatalf("expected an artist track list, got %d steps", len(tracks.Steps))
	}
	queueLen := len(m.queue.Entries)

	m = jumpTestRune(t, m, '1')
	if m.ActiveView != viewLibrary {
		t.Fatalf("expected library, got %v", m.ActiveView)
	}

	m = jumpTestPress(t, m, jumpBackKey)
	if got := m.captureBrowsingSession(); !sameBrowsingSession(got, tracks) {
		t.Fatalf("ctrl+o did not return to the artist tracks:\n got %+v\nwant %+v", got.Steps, tracks.Steps)
	}
	m = jumpTestPress(t, m, jumpBackKey)
	if got := m.captureBrowsingSession(); !sameBrowsingSession(got, albums) {
		t.Fatalf("second ctrl+o did not return to the artist albums:\n got %+v\nwant %+v", got.Steps, albums.Steps)
	}
	m = jumpTestPress(t, m, jumpForwardKey)
	if got := m.captureBrowsingSession(); !sameBrowsingSession(got, tracks) {
		t.Fatalf("ctrl+i did not return to the artist tracks:\n got %+v\nwant %+v", got.Steps, tracks.Steps)
	}
	m = jumpTestPress(t, m, jumpForwardKey)
	if m.ActiveView != viewLibrary {
		t.Fatalf("second ctrl+i should reach the library, got %v", m.ActiveView)
	}
	if len(m.queue.Entries) != queueLen || m.playback.state != playback.StateIdle {
		t.Fatal("jumping changed the queue or playback")
	}
}

func TestNewNavigationDiscardsForwardJumps(t *testing.T) {
	m := jumpTestFixture()
	m = jumpTestRune(t, m, '4')
	m = jumpTestRune(t, m, '2')
	m = jumpTestPress(t, m, jumpBackKey) // browse
	m = jumpTestRune(t, m, '3')          // new branch: queue
	m = jumpTestPress(t, m, jumpForwardKey)
	if m.ActiveView != viewQueue {
		t.Fatalf("forward list survived new navigation; now on %v", m.ActiveView)
	}
	m = jumpTestPress(t, m, jumpBackKey)
	if m.ActiveView != viewBrowse {
		t.Fatalf("ctrl+o after branching should return to browse, got %v", m.ActiveView)
	}
}

func TestLongMotionRecordsJump(t *testing.T) {
	m := jumpTestFixture()
	if len(m.browsingKeys()) < 3 {
		t.Skip("fixture library too small")
	}
	top := m.browsingPosition().Key
	m = jumpTestRune(t, m, 'G')
	if m.browsingPosition().Key == top {
		t.Fatal("G did not move the cursor")
	}
	m = jumpTestPress(t, m, jumpBackKey)
	if got := m.browsingPosition().Key; got != top {
		t.Fatalf("ctrl+o after G returned to %q, want %q", got, top)
	}
	m = jumpTestPress(t, m, jumpForwardKey)
	if got := m.browsingPosition().Key; got == top {
		t.Fatal("ctrl+i did not return to the bottom")
	}
}

func TestCursorMovesDoNotRecordJumps(t *testing.T) {
	m := jumpTestFixture()
	m = jumpTestRune(t, m, 'j')
	m = jumpTestRune(t, m, 'j')
	if n := len(m.jumps.back); n != 0 {
		t.Fatalf("plain cursor moves recorded %d jumps", n)
	}
}

func TestTabStillCyclesViews(t *testing.T) {
	m := jumpTestFixture()
	m = jumpTestPress(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.ActiveView != nextView(viewLibrary) {
		t.Fatalf("tab no longer cycles views; on %v", m.ActiveView)
	}
	// The tab switch is itself a jump, so ctrl+o undoes it.
	m = jumpTestPress(t, m, jumpBackKey)
	if m.ActiveView != viewLibrary {
		t.Fatalf("ctrl+o after tab should return to library, got %v", m.ActiveView)
	}
}

func TestJumpListIsBounded(t *testing.T) {
	m := jumpTestFixture()
	for i := range jumpListLimit + 20 {
		if i%2 == 0 {
			m = jumpTestRune(t, m, '2')
		} else {
			m = jumpTestRune(t, m, '1')
		}
	}
	if n := len(m.jumps.back); n > jumpListLimit {
		t.Fatalf("jump list grew to %d entries", n)
	}
}
