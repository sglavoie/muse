package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/overlay"
)

// insightsNotified reports whether any live notification carries substr.
func insightsNotified(m Model, substr string) bool {
	for _, n := range m.statusBar.Notifications {
		if strings.Contains(n.Msg, substr) {
			return true
		}
	}
	return false
}

func TestInsightsVisualSelectionWorksAtTrackLevel(t *testing.T) {
	m := newInsightsModel(t)

	// The briefing is not a track list, and v is advertised, so it has to say
	// what is missing rather than swallow the key.
	m = doVisual(m)
	if m.visual.active {
		t.Fatal("visual mode must not enter on the Insights briefing")
	}
	if !insightsNotified(m, "track list") {
		t.Errorf("expected a refusal naming the missing track list, got %+v", m.statusBar.Notifications)
	}

	// A session list is a list of sessions, not of tracks, and refuses too.
	m.insights.openSessionList("Sessions", []insights.SessionRow{{ID: "s1"}, {ID: "s2"}})
	m = doVisual(m)
	if m.visual.active {
		t.Fatal("visual mode must not enter on the Insights session list")
	}
	m.insights.back()

	tracks := m.insightsDrillTracks(insightsRow{drill: drillUnheardAll})
	if len(tracks) != 3 {
		t.Fatalf("precondition: want 3 never-played tracks, got %d", len(tracks))
	}
	m.insights.openDrill("Never played", tracks)
	// A selection made before v must survive every restore of the range.
	m.insights.trackView.SelectedHashes = map[string]struct{}{"h5": {}}

	m = doVisual(m)
	if !m.visual.active {
		t.Fatal("expected visual mode to enter at the Insights track level")
	}
	if m.visual.view != viewInsights {
		t.Errorf("visual view: want viewInsights, got %v", m.visual.view)
	}
	if m.visual.anchor != 0 {
		t.Errorf("anchor: want the cursor position 0, got %d", m.visual.anchor)
	}
	assertSelectedHashesExact(t, m.insights.trackView.SelectedHashes, "h2", "h5")

	m = doNav(m, keys.ActionCursorDown)
	assertSelectedHashesExact(t, m.insights.trackView.SelectedHashes, "h2", "h3", "h5")

	// Shrinking the range restores the snapshot rather than leaving h3 behind.
	m = doNav(m, keys.ActionCursorUp)
	assertSelectedHashesExact(t, m.insights.trackView.SelectedHashes, "h2", "h5")

	m.showFooterHints = true
	m.termWidth = 200
	hint := ansi.Strip(m.renderFooterHint())
	for _, want := range []string{"extend", "exit", "clear"} {
		if !strings.Contains(hint, want) {
			t.Errorf("visual footer hints missing %q: %q", want, hint)
		}
	}

	// v again exits, keeping what the range selected.
	m = doVisual(m)
	if m.visual.active {
		t.Fatal("expected a second v to exit visual mode")
	}
	assertSelectedHashesExact(t, m.insights.trackView.SelectedHashes, "h2", "h5")

	// esc leaves the drill, which is the list the anchor pointed into.
	m = doVisual(m)
	if !m.visual.active {
		t.Fatal("expected visual mode to re-enter")
	}
	m = m.handleInsightsBack()
	if m.visual.active {
		t.Error("stepping out of the drill must exit visual mode")
	}
	if m.insights.level != insightsLevelBriefing {
		t.Errorf("expected the briefing after back, got level %d", m.insights.level)
	}
}

// pressInsightsKey routes key through the real Update dispatch and keeps the
// command, so a hint whose key is bound to nothing fails here.
func pressInsightsKey(t *testing.T, m Model, key tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(key)
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	return updated, cmd
}

// TestInsightsSelectionFooterMatchesEnterAction makes a selection with a visual
// range, renders the footer the running program would show, and then presses
// each key it advertises. Both track levels are covered because only one of them
// is reached by drilling twice, and a footer naming an action its key does not
// perform is the bug this guards against.
func TestInsightsSelectionFooterMatchesEnterAction(t *testing.T) {
	cases := []struct {
		name         string
		open         func(t *testing.T, opts ...Option) Model
		wantSelected []string
		wantActedOn  []string
	}{
		{
			name: "an ordinary drill",
			open: func(t *testing.T, opts ...Option) Model {
				t.Helper()
				m := newInsightsModel(t, opts...)
				tracks := m.insightsDrillTracks(insightsRow{drill: drillUnheardAll})
				if len(tracks) != 3 {
					t.Fatalf("precondition: want 3 never-played tracks, got %d", len(tracks))
				}
				m.insights.openDrill("Never played", tracks)
				return m
			},
			wantSelected: []string{"h2", "h3"},
			wantActedOn:  []string{"h2", "h3"},
		},
		{
			name: "a session replay",
			open: func(t *testing.T, opts ...Option) Model {
				t.Helper()
				m := newSessionsModel(t, opts...)
				m, _ = m.handleInsightsOpen()
				m, _ = m.handleInsightsOpen()
				if m.insights.level != insightsLevelSessionTracks {
					t.Fatalf("precondition: want the session track level, got %d", m.insights.level)
				}
				return m
			},
			// The session plays h1 twice and a selection is held by hash, so
			// selecting its first row selects both of the rows showing h1.
			wantSelected: []string{"h1", "h6"},
			wantActedOn:  []string{"h1", "h6", "h1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			be := &stubBatchEnqueuer{}
			m := tc.open(t, WithBatchEnqueuer(be))
			m.showFooterHints = true
			m.termWidth = 200

			m = doVisual(m)
			m = doNav(m, keys.ActionCursorDown)
			m = doVisual(m)
			assertSelectedHashesExact(t, m.insights.trackView.SelectedHashes, tc.wantSelected...)

			hint := ansi.Strip(m.renderFooterHint())
			for _, want := range []string{"enter enqueue", "a bulk action", "t tag", "X clear"} {
				if !strings.Contains(hint, want) {
					t.Fatalf("selection footer missing %q: %q", want, hint)
				}
			}

			picked, _ := pressInsightsKey(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
			if picked.overlay.destPicker == nil {
				t.Error("a advertises a bulk action but opened no destination picker")
			}
			if got := hashesOf(picked.overlay.destPickerTracks); !slices.Equal(got, tc.wantActedOn) {
				t.Errorf("the picker staged %v, want %v", got, tc.wantActedOn)
			}

			tagged, _ := pressInsightsKey(t, m, tea.KeyPressMsg{Code: 't', Text: "t"})
			if tagged.overlay.tagPicker == nil {
				t.Error("t advertises tagging but opened no tag picker")
			}
			if got, _ := tagged.tagPickerHashes(); !slices.Equal(got, tc.wantSelected) {
				t.Errorf("the tag picker took %v, want the selection %v", got, tc.wantSelected)
			}

			cleared, _ := pressInsightsKey(t, m, tea.KeyPressMsg{Code: 'X', Text: "X"})
			if len(cleared.insights.trackView.SelectedHashes) != 0 {
				t.Errorf("X advertises a clear but left %d rows selected", len(cleared.insights.trackView.SelectedHashes))
			}

			// enter enqueues the same set and then drops the selection, so the
			// footer goes back to advertising the ordinary actions.
			m, cmd := pressInsightsKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("enter advertises an enqueue but issued no command")
			}
			if _, ok := cmd().(BatchEnqueuedMsg); !ok {
				t.Fatal("enter did not enqueue")
			}
			if len(be.appendTracksCalls) != 1 {
				t.Fatalf("want one AppendTracks call, got %d", len(be.appendTracksCalls))
			}
			if got := hashesOf(be.appendTracksCalls[0]); !slices.Equal(got, tc.wantActedOn) {
				t.Errorf("enter enqueued %v, want %v", got, tc.wantActedOn)
			}
			if got := be.appendTracksSources[0]; got != m.insights.drillLabel {
				t.Errorf("source should name the drill: want %q, got %q", m.insights.drillLabel, got)
			}
			if n := len(m.insights.trackView.SelectedHashes); n != 0 {
				t.Errorf("enter left %d rows selected", n)
			}
			if got := ansi.Strip(m.renderFooterHint()); strings.Contains(got, "enter enqueue") {
				t.Errorf("the selection footer outlived the selection: %q", got)
			}
		})
	}
}

func TestInsightsHelpSection(t *testing.T) {
	sections := overlay.FilteredHelpSections(keys.ContextInsights)
	// The status-bar legend is pinned ahead of everything; the view-specific
	// section still leads the rest, before any global section.
	if len(sections) < 2 {
		t.Fatalf("expected at least two help sections for contextInsights, got %d", len(sections))
	}
	if sections[0].Title != overlay.StatusBarHelpSectionTitle {
		t.Errorf("expected the pinned %q section first, got %q", overlay.StatusBarHelpSectionTitle, sections[0].Title)
	}
	if sections[1].Title != "Insights" {
		t.Errorf("expected the Insights section after the pinned legend, got %q", sections[1].Title)
	}
}
