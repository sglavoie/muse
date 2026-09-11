package tui

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/radio"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/autodj"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
)

// This file connects the Tune panel to the DJ lifecycle.
//
// The panel edits a draft and knows nothing about sessions, queues, or writers;
// everything below is the translation in both directions. Outward: the applied
// snapshot and the catalog the pickers offer. Inward: one commit, routed to
// Apply when a session is running and to Start when none is, and one
// acknowledgement carrying the token the commit was issued with.
//
// The token is what makes a late answer harmless. djApply supersedes any refresh
// already in flight, so a result planned against the previous draft is filed as
// late by the refresh identity and never reaches the panel — and a panel closed
// and reopened between the commit and its answer holds no token at all, so the
// answer lands nowhere rather than on a draft that never asked the question. A
// commit whose refresh is withdrawn that way is answered by the withdrawal
// itself, because nothing downstream is left to answer it.

// autoDJPanelState is the root's half of one Tune-panel commit.
type autoDJPanelState struct {
	// token identifies the commit awaiting an acknowledgement. Zero when idle.
	token uint64
	// nextToken is the monotonic counter tokens are drawn from. It lives on the
	// Model rather than in the panel so closing and reopening the overlay cannot
	// mint a token an outstanding commit already used.
	nextToken uint64
}

// openAutoDJTunePanel opens the Tune panel on the applied settings.
func (m Model) openAutoDJTunePanel() (Model, tea.Cmd) {
	panel := autodj.New(autodj.Options{
		Applied: m.dj.settings,
		Catalog: m.autoDJCatalog(),
		Active:  m.dj.active,
	})
	m.overlay.autoDJ = &panel
	return m, nil
}

// autoDJCatalog snapshots the values the inclusion and exclusion pickers offer.
//
// Genres and artists are derived from the library index when the Browse view has
// not built them yet, the same fallback the Browse actions use: the panel is
// reachable from every view, so it cannot depend on the user having visited the
// one that happens to cache these lists.
func (m Model) autoDJCatalog() autodj.Catalog {
	artists := m.browse.Artists.Artists
	genres := m.browse.Genres.Genres
	if len(artists) == 0 || len(genres) == 0 {
		tracks := m.library.Index.Tracks()
		if len(artists) == 0 {
			artists = arttui.DeriveArtists(tracks)
		}
		if len(genres) == 0 {
			genres = gentui.DeriveGenres(tracks)
		}
	}
	return autodj.Catalog{
		Tags:    nonEmptyTags(m.tags, m.tagsView.TagCounts),
		Genres:  genres,
		Artists: artists,
	}
}

// handleAutoDJPanelKeyPress intercepts key presses while the Tune panel is open.
func (m Model) handleAutoDJPanelKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.autoDJ == nil {
		return m, nil, false
	}
	if msg.String() == "ctrl+c" {
		return m, tea.Quit, true
	}
	updated, action := m.overlay.autoDJ.HandleKey(msg)
	m.overlay.autoDJ = &updated
	switch action {
	case autodj.ActionCancel:
		return m.closeAutoDJPanel(), nil, true
	case autodj.ActionCommit:
		m, cmd := m.commitAutoDJDraft(time.Now())
		return m, cmd, true
	case autodj.ActionPreview:
		m, cmd := m.ensureAutoDJPreview(updated.Draft(), time.Now())
		return m, cmd, true
	}
	return m, nil, true
}

// closeAutoDJPanel dismisses the panel, discarding the draft.
//
// The outstanding token is deliberately kept. A commit already in flight still
// has durable work behind it, and clearing the token here would only mean the
// acknowledgement has nowhere to land — which it already does, because the panel
// it belonged to is gone.
//
// The held draft plan is dropped, though. It was computed for a draft that no
// longer exists, and a reply still in flight for it has no reader; superseding
// here is what keeps a late answer from landing on the next panel the user opens.
func (m Model) closeAutoDJPanel() Model {
	m.overlay.autoDJ = nil
	return m.djSupersedePreview()
}

// commitAutoDJDraft applies the panel's draft, starting a session when none is
// running.
//
// Both routes go through the same lifecycle entry points the rest of the DJ
// uses, so a commit is indistinguishable from any other Apply or start as far as
// persistence, supersession, and the refill are concerned. What differs is only
// how the answer gets back: a refresh actually in flight answers through
// djRefreshDoneMsg, and everything else is already settled by the time this
// returns and is acknowledged on the spot.
func (m Model) commitAutoDJDraft(now time.Time) (Model, tea.Cmd) {
	if m.overlay.autoDJ == nil {
		return m, nil
	}
	draft := m.overlay.autoDJ.Draft()

	// The draft plan is retired by the commit that acts on it. Whatever the
	// refill enqueues is planned fresh against the settings just adopted, so
	// leaving the old proposal on the page would keep offering to add tracks
	// that may already be in the queue. What replaces it is better evidence
	// anyway: the committed rows, carrying the reasons stored with them.
	m = m.djSupersedePreview()

	// The token is minted after the gesture, not before it. Applying supersedes
	// whatever refresh was already in flight, and that withdrawal answers the
	// commit the withdrawn refresh belonged to — a token minted first would be
	// answered by its own gesture before the work it waits on has begun.
	m, cmd, err := m.applyAutoDJDraft(draft, now)
	m.autoDJPanel.nextToken++
	token := m.autoDJPanel.nextToken
	m.autoDJPanel = autoDJPanelState{token: token, nextToken: token}

	submitted := m.overlay.autoDJ.Submit(token)
	m.overlay.autoDJ = &submitted

	if err != nil {
		return m.ackAutoDJPanel(token, err), nil
	}
	if m.djRefresh.pending {
		return m, cmd
	}
	return m.ackAutoDJPanel(token, nil), cmd
}

// applyAutoDJDraft routes the draft to Apply or to Start.
func (m Model) applyAutoDJDraft(draft radio.Settings, now time.Time) (Model, tea.Cmd, error) {
	if m.dj.active {
		return m.djApply(draft, now)
	}
	return m.djStartWithSettings(draft, now)
}

// djStartWithSettings adopts a settings snapshot and starts a session on it.
//
// The order is what matters. Adopting first means the seed is captured under the
// settings the user just chose, so the very first batch is planned against them
// rather than against whatever the previous session left behind. Radio mode is
// switched on alongside, because a started session that the refill is not
// running for would be a DJ that never plays anything.
func (m Model) djStartWithSettings(settings radio.Settings, now time.Time) (Model, tea.Cmd, error) {
	m, err := m.djApplySettings(settings)
	if err != nil {
		return m, nil, err
	}
	m = m.radioSetEnabled(true)
	m.radio.seedHash = m.playback.currentHash
	m, _ = m.djStartFor(djSeedCurrent, now)
	m, cmd := m.startRadioEnqueueCmd()
	return m, cmd, nil
}

// ackAutoDJPanel reports the outcome of the commit identified by token.
func (m Model) ackAutoDJPanel(token uint64, err error) Model {
	if m.autoDJPanel.token != token {
		return m
	}
	m.autoDJPanel.token = 0
	if m.overlay.autoDJ == nil {
		return m
	}
	acked := m.overlay.autoDJ.Ack(autodj.Applied{
		Token:    token,
		Settings: m.dj.settings,
		Active:   m.dj.active,
		Err:      err,
	})
	m.overlay.autoDJ = &acked
	return m
}

// ackAutoDJRefresh files a completed immediate refresh against the commit that
// launched it. A refresh started by anything other than the panel finds no
// outstanding token and passes straight through.
func (m Model) ackAutoDJRefresh(err error) Model {
	if m.autoDJPanel.token == 0 {
		return m
	}
	return m.ackAutoDJPanel(m.autoDJPanel.token, autoDJRefreshErr(err))
}

// withdrawAutoDJPanelWait answers a commit whose refresh was withdrawn before it
// could report.
//
// A withdrawal is not a failure, for the same reason a cancelled refresh is not
// one: djApply persists the settings before it launches anything, so the
// snapshot the user committed is in force either way, and the gesture that
// superseded the refresh is the user's own newer decision. What the panel must
// not do is keep waiting. It refuses a second commit while one is in flight, so
// a commit nothing ever answers would leave the Apply button dead for as long as
// the overlay stays open.
func (m Model) withdrawAutoDJPanelWait() Model {
	if m.autoDJPanel.token == 0 {
		return m
	}
	return m.ackAutoDJPanel(m.autoDJPanel.token, nil)
}

// autoDJRefreshErr translates a refresh outcome into what the panel should say.
//
// Only a genuine failure is reported. The settings were adopted and persisted
// before the refresh was ever launched, so a queue with no DJ rows to take over,
// or a refresh withdrawn by a newer gesture, is an Apply that did exactly what it
// promised — the new settings are in force, and the ordinary refill is where they
// reach the queue.
func autoDJRefreshErr(err error) error {
	switch {
	case err == nil,
		errors.Is(err, errDJRefreshNoSlots),
		errors.Is(err, errDJRefreshCancelled):
		return nil
	}
	return fmt.Errorf("queue refresh: %w", err)
}

// radioSetEnabled switches radio mode on or off and persists the change. It is
// the shared half of the radio toggle and the panel's Start, so both routes
// leave the same durable state behind.
func (m Model) radioSetEnabled(enabled bool) Model {
	if m.radio.enabled == enabled {
		return m
	}
	m.radio.enabled = enabled
	if m.sessionWriter != nil {
		if err := m.sessionWriter.WriteRadioMode(enabled); err != nil {
			slog.Warn("session: write radio mode", "err", err)
		}
	}
	return m
}

// autoDJPanelStatus is what the panel renders under the draft: the session
// state the user cannot edit but needs to read.
func (m Model) autoDJPanelStatus() []autodj.StatusLine {
	var lines []autodj.StatusLine
	if err := m.dj.restoreErr; err != nil {
		lines = append(lines, autodj.StatusLine{
			Text: "Saved settings could not be read: " + err.Error(),
			Tone: autodj.ToneError,
		})
	}
	if line, ok := m.djRefreshStatusLine(); ok {
		lines = append(lines, line)
	}
	if b := m.djPlan.block; b.active {
		lines = append(lines, autodj.StatusLine{
			Text: djPlanBlockText(b),
			Tone: autodj.ToneWarn,
		})
	}
	return lines
}

// djRefreshStatusLine summarizes what the last immediate refresh did.
func (m Model) djRefreshStatusLine() (autodj.StatusLine, bool) {
	st := m.djRefresh.status
	if st.at.IsZero() {
		return autodj.StatusLine{}, false
	}
	if st.err != nil {
		return autodj.StatusLine{Text: "Last refresh: " + st.err.Error(), Tone: autodj.ToneWarn}, true
	}
	if st.replaced == 0 && st.retired == 0 {
		return autodj.StatusLine{Text: "Last refresh: nothing upcoming to replace", Tone: autodj.ToneInfo}, true
	}
	return autodj.StatusLine{
		Text: fmt.Sprintf("Last refresh: %d replaced, %d retired", st.replaced, st.retired),
		Tone: autodj.ToneInfo,
	}, true
}

// djPlanBlockText explains why the refill is parked.
func djPlanBlockText(b djPlanBlock) string {
	if b.err != nil {
		return "Additions paused: " + b.err.Error()
	}
	if len(b.reasons) > 0 {
		return "Additions paused: " + b.reasons[0].String()
	}
	return "Additions paused: no candidates match these settings"
}
