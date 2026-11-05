package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestNowPlayingOverlay_BasicContent(t *testing.T) {
	n := NowPlayingOverlay{
		Artist:  "Test Artist",
		Title:   "Test Song",
		Elapsed: 83,
		Total:   296,
		State:   playback.StatePlaying,
	}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))

	if !strings.Contains(out, "Now Playing") {
		t.Error("expected 'Now Playing' header")
	}
	if !strings.Contains(out, "Test Artist") {
		t.Error("expected artist in output")
	}
	if !strings.Contains(out, "Test Song") {
		t.Error("expected title in output")
	}
	if !strings.Contains(out, "1:23") || !strings.Contains(out, "4:56") {
		t.Error("expected formatted elapsed/total times")
	}
}

func TestNowPlayingOverlay_AlbumShown(t *testing.T) {
	n := NowPlayingOverlay{
		Artist: "A",
		Title:  "T",
		Album:  "Great Album",
		State:  playback.StatePlaying,
	}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "Great Album") {
		t.Error("expected album in output")
	}
}

func TestNowPlayingOverlay_AlbumAbsentWhenEmpty(t *testing.T) {
	n := NowPlayingOverlay{Artist: "The Artist", Title: "T", State: playback.StatePlaying}
	lines := strings.Split(ansi.Strip(n.Render(80, 24, newTestStyles())), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "The Artist" {
			continue
		}
		// Without an album the artist is followed by the spacer before the
		// progress line, not by an empty album row.
		if i+2 >= len(lines) || strings.TrimSpace(lines[i+1]) != "" || !strings.Contains(lines[i+2], "0:00") {
			t.Fatalf("unexpected layout after artist:\n%s", strings.Join(lines, "\n"))
		}
		return
	}
	t.Fatal("artist line not found")
}

func TestNowPlayingOverlay_FullScreenLayout(t *testing.T) {
	cover := strings.TrimSuffix(strings.Repeat(strings.Repeat("▀", 20)+"\n", 10), "\n")
	n := NowPlayingOverlay{
		Artist: "Artist", Title: "Song", Album: "Record", State: playback.StatePlaying,
		Elapsed: 30, Total: 120,
		Cover: cover, CoverCols: 20, CoverRows: 10,
		Details: "2:00 · played 3 times", Modes: "Shuffle",
		Progress: func(width int) string { return strings.Repeat("=", width) },
		UpNext:   []UpNextSong{{Title: "Next One", Artist: "Someone"}, {Title: "Next Two"}},
		Reason:   "Because you love it",
	}
	for _, size := range [][2]int{{100, 28}, {72, 16}, {60, 12}} {
		out := n.Render(size[0], size[1], newTestStyles())
		plain := ansi.Strip(out)
		lines := strings.Split(plain, "\n")
		if len(lines) > size[1] {
			t.Fatalf("%dx%d: %d lines overflow", size[0], size[1], len(lines))
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > size[0] {
				t.Fatalf("%dx%d: line %q is %d wide", size[0], size[1], line, w)
			}
		}
		for _, want := range []string{"Song", "Artist", "0:30", "2:00", "Playing", "esc close"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("%dx%d: missing %q in\n%s", size[0], size[1], want, plain)
			}
		}
	}
	plain := ansi.Strip(n.Render(100, 28, newTestStyles()))
	for _, want := range []string{"▀▀▀▀", "Up next", "1  Next One", "Why Auto-DJ chose it", "Because you love it", "Shuffle", "===="} {
		if !strings.Contains(plain, want) {
			t.Fatalf("roomy layout missing %q:\n%s", want, plain)
		}
	}
	// On a short screen the least important sections give way first.
	short := ansi.Strip(n.Render(100, 14, newTestStyles()))
	if strings.Contains(short, "Why Auto-DJ") && !strings.Contains(short, "Up next") {
		t.Fatalf("reason kept while up next was dropped:\n%s", short)
	}
}

func TestNowPlayingOverlay_PlayingIcon(t *testing.T) {
	n := NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StatePlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "▶") {
		t.Error("expected ▶ icon for playing state")
	}
}

func TestNowPlayingOverlay_PausedIcon(t *testing.T) {
	n := NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StatePaused}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "⏸") {
		t.Error("expected ⏸ icon for paused state")
	}
}

func TestNowPlayingOverlay_StoppedIcon(t *testing.T) {
	n := NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StateStopped}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "■") {
		t.Error("expected ■ icon for stopped state")
	}
	if strings.Contains(out, "▶") {
		t.Error("stopped state should not show ▶ icon")
	}
}

func TestNowPlayingOverlay_UnknownTrack(t *testing.T) {
	n := NowPlayingOverlay{State: playback.StatePlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "(unknown title)") || !strings.Contains(out, "(unknown artist)") {
		t.Error("expected '(unknown title)' and '(unknown artist)' for empty artist/title")
	}
}

func TestNowPlayingOverlay_DismissHint(t *testing.T) {
	n := NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StatePlaying}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "esc close") {
		t.Error("expected dismiss hint")
	}
}

// TestNowPlayingOverlay_IdleStateDoesNotShowPlayingIcon verifies the bug fix
// where PlaybackIdle (the zero value of PlaybackState) used to fall through to
// the default switch arm and erroneously render the playing icon. Idle should
// render the stopped icon so the overlay never suggests playback is active when
// it isn't.
func TestNowPlayingOverlay_IdleStateDoesNotShowPlayingIcon(t *testing.T) {
	n := NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StateIdle}
	out := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if strings.Contains(out, "▶") {
		t.Error("idle state must not render the playing icon")
	}
	if !strings.Contains(out, "■") {
		t.Error("idle state should fall back to the stopped icon")
	}
}

func TestNowPlayingUpNextReadsTitleFirst(t *testing.T) {
	s := newTestStyles()
	song := UpNextSong{Title: "Short", Artist: "Glass Harbor and the Northern Lights Ensemble"}
	got := ansi.Strip(upNextLine(0, song, 30, s))
	if !strings.HasPrefix(got, "1  Short · Glass") || !strings.HasSuffix(got, "…") || ansi.StringWidth(got) != 30 {
		t.Fatalf("artist should follow the title and give way first: %q", got)
	}
	long := UpNextSong{Title: "A Title Long Enough To Fill The Row", Artist: "Someone"}
	got = ansi.Strip(upNextLine(1, long, 30, s))
	if strings.Contains(got, "Someone") || !strings.HasPrefix(got, "2  A Title") || ansi.StringWidth(got) > 30 {
		t.Fatalf("a long title keeps the row to itself: %q", got)
	}
	bare := ansi.Strip(upNextLine(2, UpNextSong{Title: "file.flac"}, 30, s))
	if bare != "3  file.flac" {
		t.Fatalf("a row without an artist shows its title alone: %q", bare)
	}
}

func TestNowPlayingLongTitleStaysOnOneLine(t *testing.T) {
	title := "An Extraordinarily Long Song Title That Cannot Fit Beside The Cover"
	n := NowPlayingOverlay{Artist: "A", Title: title, State: playback.StatePlaying}
	plain := ansi.Strip(n.Render(60, 15, newTestStyles()))
	if strings.Contains(plain, "The Cover") || !strings.Contains(plain, "An Extraordinarily Long Song Title That Cannot Fit Be…") {
		t.Fatalf("without the marquee a long title is truncated on one line:\n%s", plain)
	}
	n.Marquee = true
	n.TitleScroll = marqueePauseSteps + 3
	scrolled := ansi.Strip(n.Render(60, 15, newTestStyles()))
	if !strings.Contains(scrolled, "Extraordinarily") || strings.Contains(scrolled, "An Extraordinarily") {
		t.Fatalf("the marquee should have moved three cells:\n%s", scrolled)
	}
}

func TestMarqueeWindowPausesScrollsAndLoops(t *testing.T) {
	title := "abcdefghij" // 10 cells in a 4-cell window
	for step := range marqueePauseSteps + 1 {
		if got := marqueeWindow(title, 4, step); got != "abcd" {
			t.Fatalf("step %d: the start holds during the pause, got %q", step, got)
		}
	}
	if got := marqueeWindow(title, 4, marqueePauseSteps+2); got != "cdef" {
		t.Fatalf("scrolls one cell a step, got %q", got)
	}
	if got := marqueeWindow(title, 4, marqueePauseSteps+8); got != "ij  " {
		t.Fatalf("a gap follows the end, got %q", got)
	}
	period := marqueePauseSteps + len(title) + marqueeGap
	if got := marqueeWindow(title, 4, period); got != "abcd" {
		t.Fatalf("loops back to the start, got %q", got)
	}
	if got := marqueeWindow("短い", 10, 50); got != "短い" {
		t.Fatalf("a title that fits never scrolls, got %q", got)
	}
	for step := range 40 {
		if w := ansi.StringWidth(marqueeWindow("夜明けの街を歩く", 5, step)); w != 5 {
			t.Fatalf("step %d: window is %d cells wide", step, w)
		}
	}
}

func TestNowPlayingNoticeReplacesHint(t *testing.T) {
	n := NowPlayingOverlay{Artist: "A", Title: "T", State: playback.StatePlaying, Notice: "Shuffle: off"}
	plain := ansi.Strip(n.Render(80, 24, newTestStyles()))
	if !strings.Contains(plain, "Shuffle: off") || strings.Contains(plain, "esc close") {
		t.Fatalf("a notice should stand in for the hint:\n%s", plain)
	}
}

func TestNowPlayingCentersTheComposition(t *testing.T) {
	cover := strings.TrimSuffix(strings.Repeat(strings.Repeat("▀", 20)+"\n", 10), "\n")
	n := NowPlayingOverlay{
		Artist: "Artist", Title: "Song", State: playback.StatePlaying,
		Cover: cover, CoverCols: 20, CoverRows: 10,
	}
	lines := strings.Split(ansi.Strip(n.Render(140, 40, newTestStyles())), "\n")
	if len(lines) != 40 {
		t.Fatalf("got %d lines, want 40", len(lines))
	}
	first, last := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "▀") {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	// 38 body rows around a 10-row cover leave 14 above it.
	if first != 14 || last != 23 {
		t.Fatalf("cover rows %d–%d, want it centered vertically at 14–23", first, last)
	}
	// The cover, gap and capped text column are 80 cells wide, 30 from the left.
	if left := strings.Index(lines[first], "▀"); left != 30 {
		t.Fatalf("cover starts at column %d, want 30", left)
	}
}

// loudnessOverlay is a Now Playing view with a progress bar and a loudness
// strip that draws "~" across its width and records the width asked for.
func loudnessOverlay(widths *[]int) NowPlayingOverlay {
	return NowPlayingOverlay{
		Artist: "A", Title: "T", Album: "Al", Elapsed: 83, Total: 296, State: playback.StatePlaying,
		Progress: func(w int) string { return strings.Repeat("=", w) },
		Loudness: func(w int) string {
			*widths = append(*widths, w)
			return strings.Repeat("~", w)
		},
		UpNext: []UpNextSong{{Title: "One"}, {Title: "Two"}, {Title: "Three"}},
		Reason: "Because it fits",
	}
}

func TestNowPlayingOverlay_LoudnessSitsOverTheProgressBar(t *testing.T) {
	var widths []int
	n := loudnessOverlay(&widths)
	lines := strings.Split(ansi.Strip(n.Render(80, 30, newTestStyles())), "\n")
	for i, line := range lines {
		if !strings.Contains(line, "1:23 =") {
			continue
		}
		strip := lines[i-1]
		if strings.Index(strip, "~") != strings.Index(line, "=") || strings.Count(strip, "~") != strings.Count(line, "=") {
			t.Fatalf("the strip should cover exactly the bar:\n%s\n%s", strip, line)
		}
		if !n.ShowsLoudness(80, 30, newTestStyles()) {
			t.Fatal("ShowsLoudness disagrees with Render")
		}
		return
	}
	t.Fatalf("no progress line:\n%s", strings.Join(lines, "\n"))
}

func TestNowPlayingOverlay_LoudnessDropsOutFirst(t *testing.T) {
	var widths []int
	n := loudnessOverlay(&widths)
	st := newTestStyles()
	// Find the shortest screen that still shows every section with the strip.
	tall := 0
	for h := 10; h < 40; h++ {
		if n.ShowsLoudness(80, h, st) {
			tall = h
			break
		}
	}
	if tall == 0 {
		t.Fatal("the strip never fits")
	}
	full := ansi.Strip(n.Render(80, tall, st))
	short := ansi.Strip(n.Render(80, tall-1, st))
	if !strings.Contains(full, "~") || strings.Contains(short, "~") {
		t.Fatal("one row less should drop the strip")
	}
	if !strings.Contains(short, "Why Auto-DJ chose it") {
		t.Fatalf("the strip should drop out before any other section:\n%s", short)
	}
	without := n
	without.Loudness = nil
	if without.ShowsLoudness(80, 40, st) || strings.Contains(ansi.Strip(without.Render(80, 40, st)), "~") {
		t.Fatal("no Loudness, no strip")
	}
}
