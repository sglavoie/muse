package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/recap"
)

// loadRecap runs the history read a recap was opened with and installs it.
func loadRecap(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("opening the recap issued no load")
	}
	msg, ok := cmd().(recapLoadedMsg)
	if !ok {
		t.Fatal("the recap load did not answer with recapLoadedMsg")
	}
	m, _ = m.handleRecapLoaded(msg)
	return m
}

// recapFixtureAt is the recap fixture at a size with its recap open.
func recapFixtureAt(t *testing.T, width, height int, rolling bool) Model {
	t.Helper()
	m := newRecapFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: width, Height: height})
	m, cmd := m.openRecap(rolling)
	return loadRecap(t, m, cmd)
}

func recapScreen(m Model) string { return ansi.Strip(m.View().Content) }

func TestRecapOpensFromInsightsAndClosesWithEsc(t *testing.T) {
	m := newRecapFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = m.switchToView(viewInsights)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'Y', Text: "Y"})
	m = next.(Model)
	if m.ActiveOverlayName() != "recap" || !m.overlay.recap.loading {
		t.Fatalf("Y in Insights should open the recap and start loading, overlay %q", m.ActiveOverlayName())
	}
	if !strings.Contains(recapScreen(m), "Gathering your listening history") {
		t.Fatalf("a loading recap says so:\n%s", recapScreen(m))
	}
	m = loadRecap(t, m, cmd)
	plain := recapScreen(m)
	for _, want := range []string{"Your year in muse", "2025 in muse", "of listening", "● ○"} {
		if !strings.Contains(plain, want) {
			t.Errorf("first page lacks %q:\n%s", want, plain)
		}
	}
	m = pressEsc(m)
	if m.overlay.recap != nil {
		t.Fatal("esc closes the recap")
	}
}

func TestRecapWithoutAReaderSaysSo(t *testing.T) {
	m := newModelWithTracks()
	m, cmd := m.openRecap(false)
	if cmd != nil || m.overlay.recap != nil {
		t.Fatal("without a history reader the recap does not open")
	}
}

func TestRecapDefaultPeriod(t *testing.T) {
	loc := time.UTC
	now := time.Date(2025, 6, 1, 0, 0, 0, 0, loc)
	periods := func(years ...int) []recap.Period {
		var out []recap.Period
		for _, y := range years {
			out = append(out, recap.YearPeriod(y, loc))
		}
		return append(out, recap.RollingPeriod(now, loc))
	}
	cases := []struct {
		name    string
		periods []recap.Period
		rolling bool
		want    string
	}{
		{"the current year", periods(2023, 2024, 2025), false, "2025"},
		{"the latest year when this one is silent", periods(2022, 2023), false, "2023"},
		{"the rolling period when asked", periods(2024, 2025), true, "Last 12 months"},
		{"the rolling period when nothing was ever played", periods(), false, "Last 12 months"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := recapDefaultPeriod(tc.periods, 2025, tc.rolling)
			if got := tc.periods[idx].Label(); got != tc.want {
				t.Errorf("default period = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRecapEveryPageFitsEverySize(t *testing.T) {
	for _, size := range [][2]int{{60, 15}, {80, 24}, {120, 36}} {
		m := recapFixtureAt(t, size[0], size[1], false)
		pages := len(m.overlay.recap.pages)
		if pages != 10 {
			t.Fatalf("the recap fixture should fill all ten pages, got %d: %v", pages, m.overlay.recap.pages)
		}
		for i := range pages {
			lines := strings.Split(recapScreen(m), "\n")
			if len(lines) != size[1] {
				t.Fatalf("%dx%d page %d: %d lines", size[0], size[1], i, len(lines))
			}
			for _, l := range lines {
				if w := ansi.StringWidth(l); w > size[0] {
					t.Fatalf("%dx%d page %d: line %d wide: %q", size[0], size[1], i, w, l)
				}
			}
			if last := lines[len(lines)-1]; !strings.Contains(last, "esc close") {
				t.Fatalf("%dx%d page %d: the key hint belongs on the last row, got %q", size[0], size[1], i, last)
			}
			m = pressKey(m, "l")
		}
		if m.overlay.recap.currentPage() != recapPageSummary {
			t.Fatalf("%dx%d: turning past the last page stays on the summary", size[0], size[1])
		}
	}
}

func TestRecapSummaryFitsTheMinimumScreen(t *testing.T) {
	m := recapFixtureAt(t, 60, 15, false)
	m = pressKey(m, "G")
	plain := recapScreen(m)
	for _, want := range []string{"Top artist", "Aurora Fields", "Top track", "Top album", "Prime time", "Streak", "Discovery", "On repeat", "Genres"} {
		if !strings.Contains(plain, want) {
			t.Errorf("summary at 60x15 lacks %q:\n%s", want, plain)
		}
	}
}

func TestRecapPagesFollowTheData(t *testing.T) {
	m := recapFixtureAt(t, 80, 24, false)
	want := []recapPage{
		recapPageTotals, recapPageArtists, recapPageTracks, recapPageAlbum, recapPageClock,
		recapPageStreak, recapPageDiscovery, recapPageRepeat, recapPageGenres, recapPageSummary,
	}
	got := m.overlay.recap.pages
	if len(got) != len(want) {
		t.Fatalf("pages = %v, want %v", got, want)
	}
	r := m.overlay.recap.data
	r.Discovery, r.RepeatDay, r.Genres = recap.Discovery{}, recap.RepeatDay{}, nil
	skipped := recapPagesFor(r)
	for _, p := range skipped {
		if p == recapPageDiscovery || p == recapPageRepeat || p == recapPageGenres {
			t.Errorf("a page with no data is shown: %v", skipped)
		}
	}
	if pages := recapPagesFor(recap.Recap{}); len(pages) != 1 || pages[0] != recapPageEmpty {
		t.Errorf("an empty period has the one empty page, got %v", pages)
	}
}

func TestRecapYearsSwitchAndKeepThePage(t *testing.T) {
	m := recapFixtureAt(t, 80, 24, false)
	for m.overlay.recap.currentPage() != recapPageClock {
		m = pressKey(m, " ")
	}
	m = pressKey(m, "[")
	o := m.overlay.recap
	if o.data.Period.Label() != "2024" || o.currentPage() != recapPageClock {
		t.Fatalf("[ moves to 2024 on the same card, got %s page %v", o.data.Period.Label(), o.currentPage())
	}
	m = pressKey(m, "[")
	if m.overlay.recap.data.Period.Label() != "2024" {
		t.Fatal("[ stops at the oldest year with plays")
	}
	m = pressKey(pressKey(pressKey(m, "]"), "]"), "]")
	if !m.overlay.recap.data.Period.Rolling() {
		t.Fatal("] ends on the last 12 months")
	}
	if !strings.Contains(recapScreen(m), "Last 12 months") {
		t.Fatalf("the header names the rolling period:\n%s", recapScreen(m))
	}
}

func TestRecapPaletteOpensTheRollingPeriod(t *testing.T) {
	m := recapFixtureAt(t, 80, 24, true)
	if !m.overlay.recap.data.Period.Rolling() {
		t.Fatalf("the rolling entry opens on the last 12 months, got %s", m.overlay.recap.data.Period.Label())
	}
	if !strings.Contains(recapScreen(m), "The last 12 months in muse") {
		t.Fatalf("rolling totals page:\n%s", recapScreen(m))
	}
}

func TestRecapEmptyHistory(t *testing.T) {
	m := New(t.Context(), WithInsightsReader(&uitestInsights{}))
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, cmd := m.openRecap(false)
	m = loadRecap(t, m, cmd)
	plain := recapScreen(m)
	if !strings.Contains(plain, "Nothing played in the last 12 months") || strings.Contains(plain, "●") {
		t.Fatalf("an empty history shows the one empty card:\n%s", plain)
	}
	if next, cmd := m.saveRecapPlaylist(); cmd != nil || next.overlay.recap == nil {
		t.Fatal("an empty recap saves nothing")
	}
}

func TestRecapIgnoresAnAbandonedLoad(t *testing.T) {
	m := newRecapFixture()
	m, cmd := m.openRecap(false)
	stale := cmd().(recapLoadedMsg)
	m, _ = m.openRecap(true)
	stale.token--
	m, _ = m.handleRecapLoaded(stale)
	if !m.overlay.recap.loading {
		t.Fatal("a reply for an earlier open must not install itself")
	}
}

func TestRecapLoadFailureClosesIt(t *testing.T) {
	m := newRecapFixture()
	m, _ = m.openRecap(false)
	m, _ = m.handleRecapLoaded(recapLoadedMsg{token: m.overlay.recap.token, err: errors.New("boom")})
	if m.overlay.recap != nil {
		t.Fatal("a failed read closes the recap")
	}
}

func TestRecapSavesTheTopTracksAsAPlaylist(t *testing.T) {
	m := recapFixtureAt(t, 80, 24, false)
	next, cmd, _ := m.handleRecapKeyPress(tea.KeyPressMsg{Code: 'S', Text: "S"})
	if cmd == nil {
		t.Fatal("S saves the recap")
	}
	saved, ok := cmd().(mixSavedAsPlaylistMsg)
	if !ok {
		t.Fatal("the save reports through the shared playlist-saved message")
	}
	if saved.playlistName != "Muse recap: 2025" || saved.count != len(m.overlay.recap.data.Playlist) {
		t.Fatalf("saved %q with %d tracks", saved.playlistName, saved.count)
	}
	next.playlists.Playlists = saved.playlists
	if _, cmd := next.saveRecapPlaylist(); cmd != nil {
		t.Fatal("saving the same recap twice is refused")
	}
}

func TestRecapStaysStillWithoutMotion(t *testing.T) {
	m := recapFixtureAt(t, 80, 24, false)
	m = pressKey(m, " ")
	if !m.overlay.recap.shownAt.IsZero() || m.recapAnimating(time.Now()) {
		t.Fatal("fixtures and reduce_motion draw every page at once")
	}
	if p := m.recapProgress(time.Now(), recapCountDuration); p != 1 {
		t.Fatalf("progress = %v, want 1 without motion", p)
	}
}

func TestRecapPagesEaseInAndCountUpWithMotion(t *testing.T) {
	m := recapFixtureAt(t, 80, 24, false)
	m.motion.enabled = true
	m = pressKey(m, " ")
	start := m.overlay.recap.shownAt
	if start.IsZero() || !m.recapAnimating(start) || !m.transitionActive(start) {
		t.Fatal("a page turn starts a short entrance on the motion loop")
	}
	if p := m.recapProgress(start, recapCountDuration); p != 0 {
		t.Fatalf("the count-up starts from zero, got %v", p)
	}
	if m.recapAnimating(start.Add(recapCountDuration)) {
		t.Fatal("the entrance is a brief burst")
	}
	m = pressKey(m, "h")
	if lines := recapTotalsLines(m.overlay.recap.data, *m.styles, 20, 0); !strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "0 plays") {
		t.Fatalf("at the start of the count-up the numbers read zero:\n%s", strings.Join(lines, "\n"))
	}
}

func TestRecapCoverPagesFollowTheCover(t *testing.T) {
	m := recapFixtureAt(t, 120, 36, false)
	for m.overlay.recap.currentPage() != recapPageAlbum {
		m = pressKey(m, " ")
	}
	covers := m.recapCovers()
	if len(covers) != 1 || covers[0].rows == 0 {
		t.Fatalf("the album page draws one big cover, got %+v", covers)
	}
	art, _ := m.artFor(covers[0].track)
	if art == nil {
		t.Fatal("the fixture's generated cover should be loaded")
	}
	st := m.recapStyles()
	if art.HasAccent && st.Primary == m.styles.Primary {
		t.Fatal("a cover page takes its accent from the cover")
	}
	m = pressKey(m, " ")
	if st := m.recapStyles(); st.Primary != m.styles.Primary {
		t.Fatal("a page without a cover keeps the interface accent")
	}
}

func TestRecapArtistPageUploadsItsThumbnails(t *testing.T) {
	m := recapFixtureAt(t, 120, 36, false)
	m = pressKey(m, " ")
	slots := m.artSlots()
	if len(slots) != 5 {
		t.Fatalf("the artists page places five thumbnails, got %d", len(slots))
	}
}
