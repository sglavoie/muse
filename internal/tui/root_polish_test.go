package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/artwork"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestResolveArtMode(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	cases := []struct {
		name string
		mode string
		vars map[string]string
		want artMode
	}{
		{"ghostty", config.ArtworkAuto, map[string]string{"TERM_PROGRAM": "ghostty"}, artKitty},
		{"kitty", config.ArtworkAuto, map[string]string{"KITTY_WINDOW_ID": "1"}, artKitty},
		{"xterm-ghostty", config.ArtworkAuto, map[string]string{"TERM": "xterm-ghostty"}, artKitty},
		{"plain terminal", config.ArtworkAuto, map[string]string{"TERM": "xterm-256color"}, artBlocks},
		{"tmux hides graphics", config.ArtworkAuto, map[string]string{"TERM_PROGRAM": "ghostty", "TMUX": "/tmp/x"}, artBlocks},
		{"forced kitty", config.ArtworkKitty, nil, artKitty},
		{"forced blocks", config.ArtworkBlocks, map[string]string{"TERM_PROGRAM": "ghostty"}, artBlocks},
		{"off", config.ArtworkOff, nil, artOff},
	}
	for _, tc := range cases {
		if got := resolveArtMode(tc.mode, env(tc.vars)); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestFocusedCardShowsCoverAndDetails(t *testing.T) {
	m := newRichFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.syncArtwork()
	track, ok := m.focusedTrackForInspection()
	if !ok {
		t.Fatal("rich fixture has a focused track")
	}
	if art, done := m.artFor(track); !done || art == nil {
		t.Fatal("synthetic cover was not loaded for the focused track")
	}
	card := m.renderFocusedIdentity(6)
	lines := strings.Split(card, "\n")
	if len(lines) < focusedCardArtRows {
		t.Fatalf("card has %d lines, want at least %d", len(lines), focusedCardArtRows)
	}
	plain := ansi.Strip(card)
	for _, want := range []string{"▀", "Focused · " + track.Title, track.Artist, "played"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("card missing %q:\n%s", want, plain)
		}
	}
	for _, line := range lines {
		if w := ansi.StringWidth(line); w > m.termWidth {
			t.Fatalf("card line overflows %d: %d", m.termWidth, w)
		}
	}
}

func TestFocusedCardWithoutArtKeepsItsLayout(t *testing.T) {
	m := New(context.Background(),
		WithLibraryTracks([]library.Track{{Path: "/m/a.flac", Hash: "h1", Title: "Song", Artist: "Artist", Album: "Album"}}),
		WithArtwork(failingLoader{}, config.ArtworkBlocks))
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 90, Height: 24})
	card := ansi.Strip(m.renderFocusedIdentity(5))
	if !strings.Contains(card, "♪") || !strings.Contains(card, "Focused · Song") {
		t.Fatalf("pending or missing art should show a placeholder tile:\n%s", card)
	}
}

type failingLoader struct{}

func (failingLoader) Load(context.Context, artwork.Request) (*artwork.Art, error) {
	return nil, errors.New("no")
}

func TestLibrarySummaryDescribesTheCollection(t *testing.T) {
	now := time.Now()
	m := New(context.Background(), WithLibraryTracks([]library.Track{
		{Path: "/a", Hash: "a", Title: "A", Duration: 1800, PlayCount: 3, CreatedAt: now.Add(-48 * time.Hour).Unix()},
		{Path: "/b", Hash: "b", Title: "B", Duration: 1800, CreatedAt: now.Unix()},
	}))
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	got := ansi.Strip(m.renderLibrarySummary())
	for _, want := range []string{"2 tracks", "1h", "1 never played", "newest added today"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary %q lacks %q", got, want)
		}
	}
	m.library.Filter.Text = "a"
	if m.renderLibrarySummary() != "" {
		t.Fatal("a filtered library must not show whole-library numbers")
	}
}

func TestRelativeAgo(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{
		10 * time.Second:     "just now",
		5 * time.Minute:      "5 min ago",
		3 * time.Hour:        "3 h ago",
		30 * time.Hour:       "yesterday",
		5 * 24 * time.Hour:   "5 days ago",
		21 * 24 * time.Hour:  "3 weeks ago",
		90 * 24 * time.Hour:  "3 months ago",
		800 * 24 * time.Hour: "2 years ago",
	}
	for d, want := range cases {
		if got := relativeAgo(now.Add(-d), now); got != want {
			t.Errorf("%v ago: got %q, want %q", d, got, want)
		}
	}
}

func TestDisplayElapsedGlidesOnlyWhilePlaying(t *testing.T) {
	m := New(context.Background(), WithMotion(true))
	now := time.Now()
	m.playback.state = playback.StatePlaying
	m.playback.elapsedSecs, m.playback.totalSecs = 10, 100
	m.motion.sampledAt = now.Add(-500 * time.Millisecond)
	if got := m.displayElapsed(now); got < 10.49 || got > 10.51 {
		t.Fatalf("glide = %.2f, want 10.5", got)
	}
	m.motion.sampledAt = now.Add(-10 * time.Second)
	if got := m.displayElapsed(now); got != 10+motionMaxGlide {
		t.Fatalf("a stalled player glided to %.2f", got)
	}
	m.playback.state = playback.StatePaused
	if got := m.displayElapsed(now); got != 10 {
		t.Fatalf("paused position moved to %.2f", got)
	}
	m.motion.enabled = false
	m.playback.state = playback.StatePlaying
	if got := m.displayElapsed(now); got != 10 {
		t.Fatalf("reduced motion glided to %.2f", got)
	}
}

func TestMotionFrameLoopRunsOnlyWhenNeeded(t *testing.T) {
	m := New(context.Background())
	m.playback.state = playback.StatePlaying
	if _, cmd := m.scheduleMotion(); cmd != nil {
		t.Fatal("a model without motion must never animate")
	}
	m = New(context.Background(), WithMotion(true))
	if _, cmd := m.scheduleMotion(); cmd != nil {
		t.Fatal("nothing playing or transitioning: no frames")
	}
	m.playback.state = playback.StatePlaying
	m, cmd := m.scheduleMotion()
	if cmd == nil || !m.motion.scheduled {
		t.Fatal("playback should start the frame loop")
	}
	if _, again := m.scheduleMotion(); again != nil {
		t.Fatal("an already scheduled frame must not be doubled")
	}
	m = m.handleMotionFrame()
	if m.motion.scheduled || m.motion.frame != 1 {
		t.Fatalf("frame not consumed: %+v", m.motion)
	}
}

func TestSyntheticMeterAnimates(t *testing.T) {
	seen := map[string]bool{}
	for f := range 12 {
		meter := syntheticMeter(f)
		if len(meter.Bands) != player.LevelBandCount {
			t.Fatalf("frame %d: %d bars, want one per band", f, len(meter.Bands))
		}
		seen[meterRow(meter)] = true
	}
	if len(seen) < 4 {
		t.Fatalf("level meter barely moves: %v", seen)
	}
}

func TestStatusBarShowsLevelMeterWithMotion(t *testing.T) {
	m := newPlaybackFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(ansi.Strip(m.RenderStatusBar()), "▶") {
		t.Fatal("without motion the static ▶ stays")
	}
	m.motion.enabled = true
	plain := ansi.Strip(m.RenderStatusBar())
	// The fixture's levels are at rest: an empty track takes ▶'s place.
	if strings.Contains(plain, "▶") || m.statusLevelMeter() == nil {
		t.Fatalf("with motion the level meter replaces ▶: %q", plain)
	}
}

func TestCoverAccentTintsTheTheme(t *testing.T) {
	m := newPlaybackFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.syncArtwork()
	track, ok := m.playingTrack()
	if !ok {
		t.Fatal("playback fixture is playing")
	}
	art, _ := m.artFor(track)
	if art == nil || !art.HasAccent {
		t.Skip("synthetic cover has no accent")
	}
	if m.accentOverride == "" {
		t.Fatal("cover accent was not applied")
	}

	// Turning the tint off restores the configured primary color.
	old := m.cfg
	m.cfg.ArtworkAccent = false
	m = m.applyPolishSettings(old)
	if m.accentOverride != "" {
		t.Fatalf("tint off left override %q", m.accentOverride)
	}
}

func TestCoverAccentTransitionAnimates(t *testing.T) {
	m := newPlaybackFixture()
	m.motion.enabled = true
	m.motion.accentKnown = true // a theme is already showing
	m.motion.accentTarget = ""
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.syncArtwork()
	m = m.refreshArtworkAccent()
	if m.motion.accentTo == nil {
		t.Skip("synthetic cover has no accent")
	}
	start := m.styles.Primary
	m.motion.accentStart = time.Now().Add(-motionAccentDuration / 2)
	m = m.stepAccent(time.Now())
	if m.styles.Primary == start || m.accentOverride == "" {
		t.Fatal("halfway through, the accent should be between the two colors")
	}
	m = m.stepAccent(time.Now().Add(motionAccentDuration))
	if m.motion.accentTo != nil || m.accentOverride != m.motion.accentTarget {
		t.Fatal("the transition should settle on its target")
	}
}

func TestKittyPlacementUploadsOnceAndDrawsPlaceholders(t *testing.T) {
	m := newRichFixture()
	m.art.mode = artKitty
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.syncArtwork()
	cmd := m.placeArtwork()
	if cmd == nil {
		t.Fatal("a visible cover must be uploaded")
	}
	raw, ok := cmd().(tea.RawMsg)
	if !ok || !strings.Contains(raw.Msg.(string), "U=1") {
		t.Fatalf("expected a Kitty virtual placement, got %#v", cmd())
	}
	if m.placeArtwork() != nil {
		t.Fatal("an uploaded cover must not be sent again")
	}
	track, _ := m.focusedTrackForInspection()
	if out := m.renderArt(track, focusedCardArtCols, focusedCardArtRows); !strings.ContainsRune(out, '\U0010EEEE') {
		t.Fatal("placed covers must draw as Kitty placeholders")
	}
	if m.ArtworkCleanup() == "" {
		t.Fatal("uploaded covers must be freed on exit")
	}
}

func TestNowPlayingScreenShowsCoverAndUpNext(t *testing.T) {
	m := newPlaybackFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 110, Height: 32})
	m = pressKey(m, "I")
	m, _ = m.syncArtwork()
	if m.overlay.nowPlaying == nil {
		t.Fatal("I should open Now Playing")
	}
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"Now Playing", "▀▀▀▀", "Up next", "esc close"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("Now Playing lacks %q:\n%s", want, plain)
		}
	}
}

func TestOverlayOpeningFadesIn(t *testing.T) {
	m := newRichFixture()
	m.motion.enabled = true
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = next.(Model)
	if m.ActiveOverlayName() == "" || m.motion.overlayOpenedAt.IsZero() {
		t.Fatal("opening Help should start its fade-in")
	}
	if !m.motion.scheduled {
		t.Fatal("the fade-in needs animation frames")
	}
	if r := m.overlayReveal(m.motion.overlayOpenedAt); r != 0 {
		t.Fatalf("reveal at open = %.2f, want 0", r)
	}
	if r := m.overlayReveal(m.motion.overlayOpenedAt.Add(motionOverlayDuration)); r != 1 {
		t.Fatalf("reveal after the fade = %.2f, want 1", r)
	}
	m.motion.overlayOpenedAt = time.Now().Add(-motionOverlayDuration / 3)
	faded := m.View().Content
	m.motion.overlayOpenedAt = time.Now().Add(-time.Second)
	settled := m.View().Content
	if faded == settled {
		t.Fatal("mid-fade frame should differ from the settled one")
	}
	if ansi.Strip(faded) != ansi.Strip(settled) {
		t.Fatal("the fade must change colors only, never text")
	}

	// Closing returns to the view without a fade.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.ActiveOverlayName() == "" && !m.motion.overlayOpenedAt.IsZero() {
		t.Fatal("closing an overlay must clear its fade")
	}
}

func TestOverlayFamilyKeepsAutoDJTabsTogether(t *testing.T) {
	if overlayFamily("autodj_tune") != overlayFamily("autodj_preview") {
		t.Fatal("Auto-DJ tabs are one dialog")
	}
	if overlayFamily("help") == overlayFamily("now_playing") {
		t.Fatal("different overlays are different families")
	}
}

func TestRenderScreenFadedKeepsKittyPlaceholders(t *testing.T) {
	s := styles.NewTheme(true, "")
	content := "\x1b[38;2;255;255;255mtext\x1b[m\n\x1b[38;5;20m\U0010EEEE̅̅\x1b[m\nbar"
	out := s.RenderScreenFaded(content, 10, 3, 0, 0, 2)
	if strings.Contains(out, "38;2;255;255;255") {
		t.Fatal("at reveal 0 the faded rows must not keep their full color")
	}
	if !strings.Contains(out, "\U0010EEEE") || !strings.Contains(out, "38;5;20") {
		t.Fatalf("placeholder colors identify the image and must survive: %q", out)
	}
	if full := s.RenderScreenFaded(content, 10, 3, 1, 0, 2); full != s.RenderScreen(content, 10, 3) {
		t.Fatal("reveal 1 must match RenderScreen")
	}
}

func TestSwitchingViewsFadesOnlyThePanel(t *testing.T) {
	m := newRichFixture()
	m.motion.enabled = true
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(Model)
	if m.ActiveView == viewLibrary || m.motion.viewChangedAt.IsZero() {
		t.Fatal("switching views should start the panel fade")
	}
	if !m.motion.scheduled {
		t.Fatal("the fade needs animation frames")
	}
	if r := m.viewReveal(m.motion.viewChangedAt); r != motionViewFloor {
		t.Fatalf("reveal at switch = %.2f, want %.2f", r, motionViewFloor)
	}
	m.motion.viewChangedAt = time.Now().Add(-motionViewDuration / 3)
	faded := strings.Split(m.View().Content, "\n")
	m.motion.viewChangedAt = time.Now().Add(-time.Second)
	settled := strings.Split(m.View().Content, "\n")
	if len(faded) != len(settled) {
		t.Fatal("the fade must not change the layout")
	}
	header := strings.Count(m.renderTitleBar()+"\n"+m.renderTitleDivider(), "\n") + 1
	footer := strings.Count(m.RenderStatusBar(), "\n") + 1
	for i := range faded {
		if ansi.Strip(faded[i]) != ansi.Strip(settled[i]) {
			t.Fatalf("line %d changed text during the fade", i)
		}
		fixed := i < header || i >= len(faded)-footer
		if fixed && faded[i] != settled[i] {
			t.Fatalf("line %d (tab bar or status bar) must not fade", i)
		}
	}
	if strings.Join(faded, "\n") == strings.Join(settled, "\n") {
		t.Fatal("the panel should be mid-fade")
	}
}
