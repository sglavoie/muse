package playlists

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func searchDestinations(m DestinationPickerModal, query string) DestinationPickerModal {
	m, _ = m.HandleKey("/")
	for _, r := range query {
		m, _ = m.HandleKeyPress(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func TestDestinationSearchConfirmsMatchingPlaylist(t *testing.T) {
	m := searchDestinations(NewDestinationPicker(fakePlaylistRows("Alpha", "Jazz/日本", "Zulu")), "jAZZ/日")
	if !m.search.Active || m.search.Text != "jAZZ/日" {
		t.Fatalf("search input lost characters or focus: %+v", m.search)
	}
	if got := strings.Join(m.destinations(), "|"); got != "Play Queue|Jazz/日本|New playlist…" {
		t.Fatalf("destinations = %q", got)
	}
	m, cmd := m.HandleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.search.Active {
		t.Fatal("first Enter should apply search without choosing a destination")
	}
	_, cmd = m.HandleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := cmd().(DestinationPickedMsg); got.PlaylistName != "Jazz/日本" || got.IsQueue {
		t.Fatalf("picked wrong filtered destination: %+v", got)
	}
}

func TestDestinationSearchEscapePreservesFocusedPlaylist(t *testing.T) {
	m := searchDestinations(NewDestinationPicker(fakePlaylistRows("Alpha", "Beta", "Zulu")), "zul")
	m, cmd := m.HandleKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || m.search.Active || m.search.Text != "" || m.cursor != 3 {
		t.Fatalf("clear search did not preserve focus: %+v", m)
	}
	_, cmd = m.HandleKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
	if _, ok := cmd().(DestinationPickerDismissedMsg); !ok {
		t.Fatal("second Escape should dismiss picker")
	}
}

func TestDestinationNoMatchesKeepsActionsAvailable(t *testing.T) {
	m := searchDestinations(NewDestinationPicker(fakePlaylistRows("Alpha")), "missing")
	if len(m.destinations()) != 2 || !strings.Contains(m.Render(60, 15, styles.NewStyles(true)), "No matching playlists") {
		t.Fatal("expected no-match feedback and queue/create actions")
	}
	m, _ = m.HandleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, cmd := m.HandleKey("enter")
	if got := cmd().(DestinationPickedMsg); !got.IsQueue {
		t.Fatal("queue action unavailable")
	}
	m, _ = m.HandleKey("end")
	_, cmd = m.HandleKey("enter")
	if _, ok := cmd().(DestinationPickerCreateMsg); !ok {
		t.Fatal("create action unavailable")
	}
}

func TestDestinationPickerPagesAndResizes(t *testing.T) {
	names := make([]string, 50)
	for i := range names {
		names[i] = fmt.Sprintf("Playlist %02d 日本", i)
	}
	m := NewDestinationPicker(fakePlaylistRows(names...))
	m.SetHeight(15)
	m, _ = m.HandleKey("pgdown")
	if m.cursor != 9 {
		t.Fatalf("page down cursor = %d, want 9", m.cursor)
	}
	m, _ = m.HandleKey("pgup")
	if m.cursor != 0 {
		t.Fatalf("page up cursor = %d, want 0", m.cursor)
	}
	for _, key := range []string{"end", "up", "pgup", "home"} {
		m, _ = m.HandleKey(key)
		for _, height := range []int{15, 30, 18} {
			out := ansi.Strip(m.Render(60, height, styles.NewStyles(true)))
			if len(strings.Split(out, "\n")) > height {
				t.Fatalf("%s at height %d overflows:\n%s", key, height, out)
			}
			for _, line := range strings.Split(out, "\n") {
				if ansi.StringWidth(line) > 60 {
					t.Fatalf("line too wide: %q", line)
				}
			}
			if !strings.Contains(out, m.destinations()[m.cursor]) {
				t.Fatalf("focused destination missing after %s at height %d", key, height)
			}
		}
	}
}
