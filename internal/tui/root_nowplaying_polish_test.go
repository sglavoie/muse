package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// playbackFixtureAt is the playback fixture at a terminal size, with its
// covers loaded.
func playbackFixtureAt(t *testing.T, width, height int) Model {
	t.Helper()
	m := newPlaybackFixture()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = m.syncArtwork()
	return m
}

// changeTrack reports that the player moved to queue row idx by itself.
func changeTrack(m Model, idx int) Model {
	e := m.queue.Entries[idx]
	next, _ := m.Update(playback.TrackChangedMsg{Artist: e.Artist, Title: e.Title, Hash: e.Hash, Index: idx, QueueID: e.ID})
	return next.(Model)
}

func TestNowPlayingHidesTheStatusBar(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 36}, {60, 15}} {
		m := pressKey(playbackFixtureAt(t, size[0], size[1]), "I")
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		if len(lines) != size[1] {
			t.Fatalf("%dx%d: %d lines", size[0], size[1], len(lines))
		}
		plain := strings.Join(lines, "\n")
		if strings.Contains(plain, "(2 of 3)") {
			t.Fatalf("%dx%d: the status bar should give its rows to Now Playing:\n%s", size[0], size[1], plain)
		}
		if !strings.Contains(lines[len(lines)-1], "esc close") {
			t.Fatalf("%dx%d: the key hint belongs on the last row, got %q", size[0], size[1], lines[len(lines)-1])
		}
	}
	m := pressKey(pressKey(playbackFixtureAt(t, 80, 24), "I"), "esc")
	if !strings.Contains(ansi.Strip(m.View().Content), "(2 of 3)") {
		t.Fatal("closing Now Playing brings the status bar back")
	}
}

func TestNowPlayingUpNextWrapsUnderRepeatAll(t *testing.T) {
	m := playbackFixtureAt(t, 80, 24) // playing row 2 of 3, Repeat all, Shuffle
	ahead, _ := m.nowPlayingAhead()
	if len(ahead) != 1 || ahead[0].Title != "Track D1" {
		t.Fatalf("under shuffle the next pass is unknown, so only the rest of this one shows: %+v", ahead)
	}
	m.playback.shuffle = false
	ahead, _ = m.nowPlayingAhead()
	if len(ahead) != 2 || ahead[0].Title != "Track D1" || ahead[1].Title != "Track A1" {
		t.Fatalf("Repeat all in order wraps to the start, skipping the playing song: %+v", ahead)
	}
	m.playback.repeatMode = playback.RepeatOff
	if ahead, _ = m.nowPlayingAhead(); len(ahead) != 1 {
		t.Fatalf("without repeat the queue just ends: %+v", ahead)
	}
	now := m.decorateNowPlaying(*pressKey(m, "I").overlay.nowPlaying, *m.styles, time.Now())
	if len(now.UpNext) != 1 || now.UpNext[0].Title != "Track D1" || now.UpNext[0].Artist != "Charlie" {
		t.Fatalf("up next lists title and artist separately: %+v", now.UpNext)
	}
}

func TestNowPlayingBackgroundTakesTheCoverTint(t *testing.T) {
	m := playbackFixtureAt(t, 100, 30)
	if m.motion.accentTarget == "" {
		t.Skip("synthetic cover has no accent")
	}
	tint := m.nowPlayingTint(time.Now())
	if tint == m.styles.Background {
		t.Fatal("a cover accent should tint the Now Playing background")
	}
	if !tintReadable(*m.styles, tint) || styles.Contrast(m.styles.TextPrimary.GetForeground(), tint) < 7 {
		t.Fatal("the tint must keep text readable")
	}
	old := m.cfg
	m.cfg.ArtworkAccent = false
	m = m.applyPolishSettings(old)
	if m.nowPlayingTint(time.Now()) != m.styles.Background {
		t.Fatal("with Cover tint off the background stays the canvas")
	}
}

func TestNowPlayingTintEasesWithTheAccent(t *testing.T) {
	m := playbackFixtureAt(t, 100, 30)
	m.motion.enabled = true
	m.motion.accentTarget = "#e06c75"
	m.motion.accentFromArt = false
	m.motion.accentTo = m.styles.Primary
	m.motion.accentStart = time.Now().Add(-motionAccentDuration / 2)
	if w := m.coverTintWeight(time.Now()); w <= 0.2 || w >= 0.8 {
		t.Fatalf("halfway into a cover accent the tint is partway in, got %.2f", w)
	}
	if w := m.coverTintWeight(time.Now().Add(motionAccentDuration)); w != 1 {
		t.Fatalf("the tint settles fully in, got %.2f", w)
	}
	m.motion.accentTarget, m.motion.accentFromArt = "", true
	if w := m.coverTintWeight(time.Now()); w <= 0.2 || w >= 0.8 {
		t.Fatalf("easing back to the primary color fades the tint out, got %.2f", w)
	}
}

func TestNowPlayingMarqueeFollowsTheSteadyFrame(t *testing.T) {
	m := playbackFixtureAt(t, 100, 30)
	m.motion.enabled = true
	next, _ := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	m = next.(Model)
	if m.motion.titleSince.IsZero() {
		t.Fatal("opening Now Playing starts the marquee clock")
	}
	clock := m.motion.titleSince.Add(5 * motionSteadyFrame)
	now := m.decorateNowPlaying(*m.overlay.nowPlaying, *m.styles, clock)
	if !now.Marquee || now.TitleScroll != 5 {
		t.Fatalf("marquee %v at step %d, want step 5", now.Marquee, now.TitleScroll)
	}
	if m.transitionActive(clock) {
		t.Fatal("the marquee alone must not ask for the fast frame rate")
	}
	m.playback.state = playback.StatePaused
	if m.decorateNowPlaying(*m.overlay.nowPlaying, *m.styles, clock).Marquee {
		t.Fatal("a paused song's title stays still")
	}
	m.motion.enabled = false
	m.playback.state = playback.StatePlaying
	if m.decorateNowPlaying(*m.overlay.nowPlaying, *m.styles, clock).Marquee {
		t.Fatal("with animations off the title is truncated instead")
	}
}

func TestNowPlayingCrossFadesCoversOnTrackChange(t *testing.T) {
	m := playbackFixtureAt(t, 100, 30)
	m.motion.enabled = true
	m = pressKey(m, "I")
	cols, rows := m.nowPlayingArtSize()
	before := ansi.Strip(m.decorateNowPlaying(*m.overlay.nowPlaying, *m.styles, time.Now()).Cover)
	m = changeTrack(m, 2)
	if m.motion.coverFrom == nil {
		t.Fatal("a track change in Now Playing starts a cover cross-fade")
	}
	if !m.transitionActive(time.Now()) {
		t.Fatal("the cross-fade runs on the transition frame rate")
	}
	track, _ := m.playingTrack()
	newArt, _ := m.artFor(track)
	mid := m.nowPlayingCover(track, *m.styles, cols, rows, m.motion.coverFadeStart.Add(motionAccentDuration/2))
	end := m.nowPlayingCover(track, *m.styles, cols, rows, m.motion.coverFadeStart.Add(motionAccentDuration))
	if mid == end || mid == before {
		t.Fatal("halfway through, the cover is a blend of both")
	}
	if newArt == nil || end != m.renderArt(track, cols, rows) {
		t.Fatal("the fade ends on the new cover")
	}
	m = m.handleMotionFrame()
	m.motion.coverFadeStart = time.Now().Add(-2 * motionAccentDuration)
	m = m.handleMotionFrame()
	if m.motion.coverFrom != nil {
		t.Fatal("a finished fade lets go of the old cover")
	}
}

func TestNowPlayingKittyCoversSwapWithoutFading(t *testing.T) {
	m := playbackFixtureAt(t, 100, 30)
	m.art.mode = artKitty
	m.motion.enabled = true
	m = changeTrack(pressKey(m, "I"), 2)
	if m.motion.coverFrom != nil {
		t.Fatal("Kitty covers swap: blending would upload an image every frame")
	}
}

func TestTrackCardAnnouncesAutomaticChanges(t *testing.T) {
	m := changeTrack(playbackFixtureAt(t, 80, 24), 2)
	if m.trackCard.hash != m.queue.Entries[2].Hash {
		t.Fatal("a song the player moved to by itself gets a card")
	}
	plain := ansi.Strip(m.View().Content)
	for _, want := range []string{"♪ Now playing", "Track D1", "Charlie — Delta", "╭"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("card lacks %q:\n%s", want, plain)
		}
	}
	m = pressKey(m, "j")
	if strings.Contains(ansi.Strip(m.View().Content), "♪ Now playing") {
		t.Fatal("any key dismisses the card")
	}

	m = changeTrack(playbackFixtureAt(t, 80, 24), 2)
	m = m.handleTrackCardExpired(trackCardExpiredMsg{gen: m.trackCard.gen - 1})
	if m.trackCard.hash == "" {
		t.Fatal("a stale expiry must not remove a newer card")
	}
	m = m.handleTrackCardExpired(trackCardExpiredMsg{gen: m.trackCard.gen})
	if m.trackCard.hash != "" {
		t.Fatal("the card goes away when its time is up")
	}
}

func TestTrackCardStaysAwayWhenNotWanted(t *testing.T) {
	m := playbackFixtureAt(t, 80, 24)
	m.playback.currentHash = ""
	if m = changeTrack(m, 2); m.trackCard.hash != "" {
		t.Fatal("the first song (or a restored one) gets no card")
	}
	m = playbackFixtureAt(t, 80, 24)
	if m = changeTrack(m, 1); m.trackCard.hash != "" {
		t.Fatal("re-announcing the playing song (a confirmed selection) gets no card")
	}
	m = pressKey(playbackFixtureAt(t, 80, 24), "I")
	if m = changeTrack(m, 2); m.trackCard.hash != "" {
		t.Fatal("Now Playing already shows the change")
	}
	m = playbackFixtureAt(t, 80, 24)
	m.cfg.TrackChangeCard = false
	if m = changeTrack(m, 2); m.trackCard.hash != "" {
		t.Fatal("track_change_card = false turns the card off")
	}
}

func TestTrackCardNeverCoversTheFocusedRowOrFooter(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 36}, {60, 15}} {
		m := newDenseFixture()
		m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = changeTrack(m, 2)
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%dx%d: card changed the line count to %d", size[0], size[1], len(lines))
		}
		cardRows := 0
		for i, line := range lines {
			plain := ansi.Strip(line)
			if w := ansi.StringWidth(plain); w > size[0] {
				t.Fatalf("%dx%d: row %d is %d wide", size[0], size[1], i, w)
			}
			if !strings.Contains(plain, "│") && !strings.Contains(plain, "╭") && !strings.Contains(plain, "╰") {
				continue
			}
			cardRows++
			if strings.Contains(line, m.focusRowMarker()) {
				t.Fatalf("%dx%d: the card covers the focused row %d", size[0], size[1], i)
			}
			if i >= len(lines)-3 {
				t.Fatalf("%dx%d: the card reaches the footer at row %d", size[0], size[1], i)
			}
		}
		if cardRows != trackCardRows {
			t.Fatalf("%dx%d: card drew %d rows:\n%s", size[0], size[1], cardRows, ansi.Strip(m.View().Content))
		}
	}
}

func TestTrackCardSlidesInOnlyWithMotion(t *testing.T) {
	m := changeTrack(playbackFixtureAt(t, 80, 24), 2)
	if m.trackCardSliding(m.trackCard.shownAt.Add(trackCardSlide)) {
		t.Fatal("the slide is brief")
	}
	full := m.placeTrackCard(strings.Repeat("\n", 20), 2, m.trackCard.shownAt.Add(trackCardSlide/4))
	m.motion.enabled = true
	partial := m.placeTrackCard(strings.Repeat("\n", 20), 2, m.trackCard.shownAt.Add(trackCardSlide/4))
	if strings.Count(ansi.Strip(full), "─") <= strings.Count(ansi.Strip(partial), "─") {
		t.Fatal("with animations the card slides in; without them it appears whole")
	}
}
