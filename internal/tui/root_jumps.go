package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// jumpListLimit matches Vim's 'jumplist' length: older locations fall off.
const jumpListLimit = 100

// jumpList is a Vim-style jump list: ctrl+o walks back through the places
// the user left, ctrl+i walks forward again. Entries reuse the browsing
// session snapshot so a jump replays through the same guarded restore path as
// startup: it never replays Enter on a track and falls back to the parent when
// a collection disappeared.
type jumpList struct {
	back    []browsingSession
	forward []browsingSession
	// markPending asks Update to record the pre-action location even though the
	// view and collection stay the same (g / G / f move far within one list).
	markPending bool
	// jumping suppresses recording for the update that performs the jump.
	jumping bool
}

// jumpSpot is a cheap fingerprint of where the user is: the view, its modes,
// its level and the collection that is open. Cursor movement never changes it,
// so Update can compare it on every message and only capture a full browsing
// session when the place actually changed.
type jumpSpot struct {
	view   viewID
	modes  string
	level  string
	opened string
}

func (m Model) currentJumpSpot() jumpSpot {
	s := jumpSpot{
		view:  m.ActiveView,
		modes: fmt.Sprintf("%d.%d", m.library.Mode, m.browse.Mode),
		level: m.browsingLevel(),
	}
	switch m.ActiveView {
	case viewMixes:
		if m.mixes.Level != 0 {
			s.opened = fmt.Sprintf("mix:%d", m.mixes.ActiveMix)
		}
	case viewInsights:
		s.modes += fmt.Sprintf(".%d.%d", m.insights.Panel, m.insights.WindowIdx)
		s.opened = m.insights.DrillLabel + "\x00" + m.insights.SessionLabel
	default:
		// Each parent entry names the row that was opened from it, so two
		// sibling collections at the same depth still read as different places.
		var b strings.Builder
		for _, k := range slices.Sorted(maps.Keys(m.navigationParents)) {
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(m.navigationParents[k].position.Key)
			b.WriteByte(';')
		}
		s.opened = b.String()
	}
	return s
}

func isJumpMotion(action keys.Action) bool {
	switch action {
	case keys.ActionGoToTop, keys.ActionGoToBottom, keys.ActionFocusNowPlaying:
		return true
	}
	return false
}

func sameBrowsingSession(a, b browsingSession) bool {
	return a.View == b.View && a.LibraryMode == b.LibraryMode && a.BrowseMode == b.BrowseMode &&
		a.Panel == b.Panel && a.Window == b.Window && slices.Equal(a.Steps, b.Steps)
}

// pushJump appends s to list, dropping an identical top entry and the oldest
// entries past jumpListLimit.
func pushJump(list []browsingSession, s browsingSession) []browsingSession {
	if n := len(list); n > 0 && sameBrowsingSession(list[n-1], s) {
		return list
	}
	list = append(slices.Clip(list), s)
	if len(list) > jumpListLimit {
		list = slices.Clone(list[len(list)-jumpListLimit:])
	}
	return list
}

// recordJump runs once per Update: when the message moved the user to a new
// place (or a long motion asked for it), the place they left joins the back
// list and the forward list is discarded, as in Vim.
func (m Model) recordJump(before Model) Model {
	if m.jumps.jumping {
		m.jumps.jumping = false
		m.jumps.markPending = false
		return m
	}
	mark := m.jumps.markPending
	m.jumps.markPending = false
	// A restore in flight (startup or a jump) moves through intermediate
	// places that the user never chose.
	if before.browsingRestore != nil || m.browsingRestore != nil {
		return m
	}
	if !mark && before.currentJumpSpot() == m.currentJumpSpot() {
		return m
	}
	left := before.captureBrowsingSession()
	if mark && sameBrowsingSession(left, m.captureBrowsingSession()) {
		return m
	}
	m.jumps.back = pushJump(m.jumps.back, left)
	m.jumps.forward = nil
	return m
}

// handleJump moves one entry back (ctrl+o) or forward (ctrl+i) through the
// jump list, saving the current place on the opposite list.
func (m Model) handleJump(back bool) (tea.Model, tea.Cmd) {
	from, to := &m.jumps.back, &m.jumps.forward
	if !back {
		from, to = to, from
	}
	here := m.captureBrowsingSession()
	// Skip entries that are where the user already is.
	for len(*from) > 0 && sameBrowsingSession((*from)[len(*from)-1], here) {
		*from = (*from)[:len(*from)-1]
	}
	if len(*from) == 0 {
		if back {
			m.statusBar.Notify("Already at the oldest jump")
		} else {
			m.statusBar.Notify("Already at the newest jump")
		}
		return m, nil
	}
	target := (*from)[len(*from)-1]
	*from = slices.Clip((*from)[:len(*from)-1])
	*to = pushJump(*to, here)
	m.jumps.jumping = true
	return m.restoreJumpTarget(target)
}

// restoreJumpTarget rewinds the target view to its root and replays the saved
// steps through the browsing restore machinery, which waits for each
// collection load before opening the next level.
func (m Model) restoreJumpTarget(target browsingSession) (tea.Model, tea.Cmd) {
	// The tag filter is a lens over every view, not a place: keep the current one.
	target.TagFilter = nil
	m.library = m.library.SwitchMode(target.LibraryMode)
	m.browse.Mode = target.BrowseMode
	m = m.switchToView(target.View)
	for {
		parent, ok := m.browsingBack()
		if !ok {
			break
		}
		m = parent
	}
	m.browsingRestore = &browsingRestore{Session: target}
	next, cmd := m.resumeBrowsing(browsingResumeMsg{})
	updated := next.(Model)
	updated.jumps.jumping = true
	return updated, cmd
}
