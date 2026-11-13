package tui

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/statusbar"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// stubLevelPlayer is a player whose level meter returns reading (or no
// reading when ok is false) and records what the interface asked of it.
type stubLevelPlayer struct {
	stubPlayer
	reading   player.Levels
	ok        bool
	enableErr error
	enables   []bool
	reads     int
}

func (s *stubLevelPlayer) SetLevelMeter(enabled bool) error {
	s.enables = append(s.enables, enabled)
	return s.enableErr
}

func (s *stubLevelPlayer) Levels() (player.Levels, bool, error) {
	s.reads++
	return s.reading, s.ok, nil
}

func loudReading() player.Levels {
	return player.Levels{RMS: []float64{-9, -12},
		Bands: []float64{-66, -64, -66, -68, -70, -70, -71, -72, -74, -78}}
}

// levelModel is the playback fixture playing through src with animations on.
func levelModel(t *testing.T, src *stubLevelPlayer, width, height int) Model {
	t.Helper()
	m := newPlaybackFixture()
	m.player = src
	m.motion.enabled = true
	m.motionAllowed = true
	m.levels = levelState{}
	return m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: width, Height: height})
}

// runFrame schedules the next motion frame, waits for it, and delivers it.
func runFrame(t *testing.T, m Model) (Model, motionFrameMsg) {
	t.Helper()
	// Update already scheduled the frame after the last one; drop it and
	// schedule afresh so this frame is the one delivered.
	m.motion.scheduled = false
	m.levels.lastPoll = time.Time{}
	m, cmd := m.scheduleMotion()
	if cmd == nil {
		t.Fatal("no frame was scheduled")
	}
	msg, ok := cmd().(motionFrameMsg)
	if !ok {
		t.Fatal("the frame command did not produce a motion frame")
	}
	next, _ := m.Update(msg)
	return next.(Model), msg
}

func TestDbToLevelIsLinearInDecibels(t *testing.T) {
	cases := []struct{ db, want float64 }{
		{math.Inf(-1), 0},
		{math.NaN(), 0},
		{-90, 0},
		{levelFloorDB, 0},
		{levelFloorDB / 2, 0.5},
		{0, 1},
		{6, 1}, // inter-sample overs clamp
	}
	for _, c := range cases {
		if got := dbToLevel(c.db); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("dbToLevel(%v) = %v, want %v", c.db, got, c.want)
		}
	}
	prev := -1.0
	for db := -60.0; db <= 0; db += 1.5 {
		v := dbToLevel(db)
		if v < prev {
			t.Fatalf("mapping is not monotonic at %v dB", db)
		}
		prev = v
	}
}

func TestLevelTargetsMapBandsAndLoudness(t *testing.T) {
	reading := player.Levels{RMS: []float64{-20, -30},
		Bands: []float64{-120, -90, -85, -80, -75, -70, -79, -79, -79, -40}}
	got := levelTargets(reading)
	if got.bands[0] != 0 {
		t.Errorf("a band under the floor should rest: %v", got.bands)
	}
	if got.bands[9] != 1 {
		t.Errorf("a band over the ceiling should fill its bar: %v", got.bands)
	}
	for i := 2; i < 6; i++ {
		if got.bands[i] <= got.bands[i-1] {
			t.Errorf("louder bands should stand taller: %v", got.bands)
			break
		}
	}
	if got.bands[6] != got.bands[8] {
		t.Errorf("equal levels should stand level: %v", got.bands)
	}
	if want := (dbToLevel(-20+levelRMSLift) + dbToLevel(-30+levelRMSLift)) / 2; got.loudness != want {
		t.Errorf("loudness = %v, want the channels' average %v", got.loudness, want)
	}
	mono := levelTargets(player.Levels{RMS: []float64{-18}})
	if mono.loudness != dbToLevel(-18+levelRMSLift) {
		t.Errorf("mono counts once: %v", mono.loudness)
	}
	silent := levelTargets(player.Levels{RMS: []float64{math.Inf(-1)}, Bands: []float64{math.Inf(-1), math.NaN()}})
	if silent != (levelFrame{}) || meterRow(levelMeterFrame(silent.bands)) != "▁▁▁▁▁▁▁▁▁▁" {
		t.Errorf("silence should rest flat: %+v", silent)
	}
	if levelTargets(player.Levels{}) != (levelFrame{}) {
		t.Error("an empty reading has no level")
	}
}

// meterRow renders a meter alone, as plain text.
func meterRow(meter statusbar.Meter) string {
	sb := statusbar.StatusBar{PlaybackState: playback.StatePlaying, LevelMeter: &meter}
	line := ansi.Strip(sb.Render(0, styles.Styles{}))
	meterText, _, _ := strings.Cut(line, " ")
	return meterText
}

func TestSmoothLevelRisesFastAndFallsSteadily(t *testing.T) {
	up := smoothLevel(0, 1, 0.08)
	if up < 0.6 || up >= 1 {
		t.Fatalf("attack should cover most of a rise in one reading: %v", up)
	}
	down := smoothLevel(1, 0, 0.08)
	if want := 1 - levelDecayPerSec*0.08; math.Abs(down-want) > 1e-9 {
		t.Fatalf("decay = %v, want %v", down, want)
	}
	if got := smoothLevel(0.5, 0.45, 0.08); got != 0.45 {
		t.Fatalf("a small fall should land on the target, not overshoot: %v", got)
	}
	v := 1.0
	for range 10 {
		v = smoothLevel(v, 0, 0.08)
	}
	if v != 0 {
		t.Fatalf("silence should bring the bars to rest within a second: %v", v)
	}
}

func TestLevelHistoryRecordsOneColumnPerStep(t *testing.T) {
	var l levelState
	start := time.Unix(1000, 0)
	for i := range 40 {
		l = l.record(start.Add(time.Duration(i)*levelPollInterval), float64(i%4)/4)
	}
	// 40 readings 80 ms apart span 3.12 s: twelve closed 250 ms steps.
	if len(l.history) != 12 {
		t.Fatalf("history has %d columns, want 12", len(l.history))
	}
	for i, v := range l.history {
		if v < 0.5 {
			t.Fatalf("column %d = %v: each column keeps the loudest reading of its step", i, v)
		}
	}
	for i := range levelHistoryLen * 5 {
		l = l.record(start.Add(time.Duration(i)*levelHistoryStep), 0.5)
	}
	if len(l.history) != levelHistoryLen {
		t.Fatalf("history grew to %d, cap is %d", len(l.history), levelHistoryLen)
	}
}

func TestLevelReadingsDriveTheStatusBarMeter(t *testing.T) {
	src := &stubLevelPlayer{reading: loudReading(), ok: true}
	m := levelModel(t, src, 100, 30)
	if got := m.statusLevelMeter(); got == nil || meterRow(*got) != "▁▁▁▁▁▁▁▁▁▁" {
		t.Fatalf("before any reading the meter rests: %+v", got)
	}
	m, msg := runFrame(t, m)
	if msg.levels == nil || !msg.levels.enabled {
		t.Fatal("the first frame should turn the meter on and read it")
	}
	if len(src.enables) != 1 || !src.enables[0] || src.reads != 1 {
		t.Fatalf("enables %v reads %d", src.enables, src.reads)
	}
	if !m.levels.meterOn || m.levels.mode != levelsLive {
		t.Fatalf("a reading should make the meter live: %+v", m.levels)
	}
	m, _ = runFrame(t, m)
	if len(src.enables) != 1 {
		t.Fatalf("the filter is added once, not per frame: %v", src.enables)
	}
	plain := ansi.Strip(m.RenderStatusBar())
	if strings.Contains(plain, "▁▁▁▁▁▁▁▁▁▁") || strings.Contains(plain, "▶") {
		t.Fatalf("a loud reading should raise the bars: %q", plain)
	}
}

func TestLevelFramesRunAtTheMeterRate(t *testing.T) {
	src := &stubLevelPlayer{reading: loudReading(), ok: true}
	m := levelModel(t, src, 100, 30)
	start := time.Now()
	m, _ = runFrame(t, m)
	m, _ = runFrame(t, m)
	if elapsed := time.Since(start); elapsed < 2*levelPollInterval*9/10 || elapsed > 2*motionSteadyFrame {
		t.Fatalf("two level frames took %v", elapsed)
	}

	// Without levels to read, playback falls back to the steady frame.
	m.cfg.LevelMeter = config.LevelMeterOff
	m.motion.scheduled = false
	before := src.reads
	start = time.Now()
	_, cmd := m.scheduleMotion()
	msg := cmd().(motionFrameMsg)
	if time.Since(start) < motionSteadyFrame*9/10 || msg.levels != nil || src.reads != before {
		t.Fatal("with the meter off, frames are steady and read nothing")
	}
}

func TestLevelReadsAreThrottled(t *testing.T) {
	src := &stubLevelPlayer{reading: loudReading(), ok: true}
	m := levelModel(t, src, 100, 30)
	now := time.Now()
	m, first := m.levelRead(now, motionTransitionFrame)
	if first == nil {
		t.Fatal("the first frame should read")
	}
	_, second := m.levelRead(now.Add(motionTransitionFrame), motionTransitionFrame)
	if second != nil {
		t.Fatal("a transition frame 50 ms later must not read again: at most one read per poll interval")
	}
	_, third := m.levelRead(now.Add(levelPollInterval), motionTransitionFrame)
	if third == nil {
		t.Fatal("a frame a poll interval later should read")
	}
}

func TestLevelsAreOnlyReadWhenSomethingShowsThem(t *testing.T) {
	src := &stubLevelPlayer{reading: loudReading(), ok: true}
	base := levelModel(t, src, 100, 30)
	if !base.levelsWanted() {
		t.Fatal("playing with the status bar meter visible should read levels")
	}
	cases := map[string]func(Model) Model{
		"paused":         func(m Model) Model { m.playback.state = playback.StatePaused; return m },
		"stopped":        func(m Model) Model { m.playback.state = playback.StateStopped; return m },
		"animations off": func(m Model) Model { m.motion.enabled = false; return m },
		"setting off":    func(m Model) Model { m.cfg.LevelMeter = config.LevelMeterOff; return m },
		"fallen back":    func(m Model) Model { m.levels.mode = levelsSynthetic; return m },
		"no level source": func(m Model) Model {
			m.player = &stubPlayer{}
			return m
		},
	}
	for name, change := range cases {
		if change(base).levelsWanted() {
			t.Errorf("%s: levels should not be read", name)
		}
	}

	// Now Playing hides the status bar: levels are read only while the
	// loudness strip fits.
	big := pressKey(levelModel(t, src, 120, 36), "I")
	if !big.nowPlayingShown() || !big.levelsWanted() {
		t.Fatal("Now Playing with room for the strip should read levels")
	}
	small := pressKey(levelModel(t, src, 60, 15), "I")
	if !small.nowPlayingShown() || small.levelsWanted() {
		t.Fatal("a Now Playing too small for the strip shows no levels: nothing should be read")
	}
}

func TestPauseBringsTheMeterToRest(t *testing.T) {
	src := &stubLevelPlayer{reading: loudReading(), ok: true}
	m := levelModel(t, src, 100, 30)
	m, _ = runFrame(t, m)
	if m.levels.bands == ([player.LevelBandCount]float64{}) {
		t.Fatal("bars should be up while playing")
	}
	next, _ := m.Update(playback.PauseChangedMsg{Paused: true})
	m = next.(Model)
	if m.playback.state != playback.StatePaused {
		t.Skip("pause message did not pause the fixture")
	}
	if m.levels.bands != ([player.LevelBandCount]float64{}) || m.levels.loudness != 0 {
		t.Fatalf("paused bars should rest: %v", m.levels.bands)
	}
	if _, cmd := m.scheduleMotion(); cmd != nil {
		t.Fatal("paused with nothing animating: no frames, so no reads")
	}
}

func TestLevelMeterFallsBackWhenNoLevelsArrive(t *testing.T) {
	src := &stubLevelPlayer{ok: false}
	m := levelModel(t, src, 100, 30)
	for i := range levelFallbackMisses - 1 {
		m = m.applyLevelReading(levelReading{at: time.Now(), enabled: i == 0})
	}
	if m.levels.mode != levelsPending {
		t.Fatalf("fell back too early: %+v", m.levels)
	}
	m = m.applyLevelReading(levelReading{at: time.Now()})
	if m.levels.mode != levelsSynthetic {
		t.Fatalf("no readings for about a second should fall back: %+v", m.levels)
	}
	m.motion.frame = 3
	if got := m.statusLevelMeter(); got == nil || meterRow(*got) != meterRow(syntheticMeter(3)) {
		t.Fatalf("the fallback should show the animated meter %+v: %+v", syntheticMeter(3), got)
	}
	if m.levelsWanted() {
		t.Fatal("after falling back nothing reads levels")
	}
	m, cmd := m.syncLevels()
	if cmd == nil {
		t.Fatal("falling back should take the filter out of mpv's chain")
	}
	cmd()
	if len(src.enables) != 1 || src.enables[0] {
		t.Fatalf("want SetLevelMeter(false), got %v", src.enables)
	}
	if m.levels.meterOn {
		t.Fatal("the meter should be marked off")
	}
	if _, again := m.syncLevels(); again != nil {
		t.Fatal("the filter is removed once")
	}
}

func TestLevelMeterFallsBackWhenTheFilterIsRefused(t *testing.T) {
	src := &stubLevelPlayer{reading: loudReading(), ok: true, enableErr: errors.New("af rejected")}
	m := levelModel(t, src, 100, 30)
	for range levelFallbackMisses {
		m, _ = runFrame(t, m)
	}
	if m.levels.mode != levelsSynthetic || m.levels.meterOn || src.reads != 0 {
		t.Fatalf("a refused filter should fall back without reading: %+v reads %d", m.levels, src.reads)
	}
	if len(src.enables) != levelFallbackMisses {
		t.Fatalf("each frame should retry adding the filter until falling back: %v", src.enables)
	}
}

func TestLevelsOnceLiveSurviveGaps(t *testing.T) {
	src := &stubLevelPlayer{reading: loudReading(), ok: true}
	m := levelModel(t, src, 100, 30)
	m = m.applyLevelReading(levelReading{at: time.Now(), ok: true, levels: loudReading(), enabled: true})
	for range levelFallbackMisses * 2 {
		m = m.applyLevelReading(levelReading{at: time.Now()})
	}
	if m.levels.mode != levelsLive {
		t.Fatal("gaps between files after levels arrived must not fall back")
	}
}

func TestLevelMeterSettingOffRemovesTheFilter(t *testing.T) {
	src := &stubLevelPlayer{reading: loudReading(), ok: true}
	m := levelModel(t, src, 100, 30)
	m, _ = runFrame(t, m)
	m, _ = runFrame(t, m)
	if !m.levels.meterOn || len(m.levels.history) == 0 && m.levels.bands == ([player.LevelBandCount]float64{}) {
		t.Fatalf("meter should be live: %+v", m.levels)
	}
	old := m.cfg
	m.cfg.LevelMeter = config.LevelMeterOff
	m = m.applyPolishSettings(old)
	m, cmd := m.syncLevels()
	if cmd == nil {
		t.Fatal("level_meter off should remove the filter")
	}
	cmd()
	if last := src.enables[len(src.enables)-1]; last {
		t.Fatal("want SetLevelMeter(false)")
	}
	if m.levels.bands != ([player.LevelBandCount]float64{}) || m.levels.history != nil {
		t.Fatalf("off clears what was measured: %+v", m.levels)
	}
	if got := m.statusLevelMeter(); got == nil || meterRow(*got) != meterRow(syntheticMeter(m.motion.frame)) {
		t.Fatalf("off keeps the decorative meter: %+v", got)
	}

	// Back on: the meter starts over and asks for the filter again.
	old = m.cfg
	m.cfg.LevelMeter = config.LevelMeterAuto
	m = m.applyPolishSettings(old)
	_, msg := runFrame(t, m)
	if msg.levels == nil || !msg.levels.enabled {
		t.Fatal("turning the meter back on should add the filter again")
	}
}

func TestReduceMotionHidesTheMeterAndStrip(t *testing.T) {
	src := &stubLevelPlayer{reading: loudReading(), ok: true}
	m := levelModel(t, src, 120, 36)
	m.levels.history = syntheticLevelHistory(20)
	m.levels.meterOn = true
	old := m.cfg
	m.cfg.ReduceMotion = true
	m = m.applyPolishSettings(old)
	if m.statusLevelMeter() != nil {
		t.Fatal("reduce motion keeps the static ▶")
	}
	if m.levels.history != nil {
		t.Fatal("reduce motion clears the strip rather than freezing it")
	}
	if _, cmd := m.syncLevels(); cmd == nil {
		t.Fatal("with nothing to read them, the filter should come out")
	}
}

func TestLoudnessStripDrawsNewestOnTheRight(t *testing.T) {
	st := *newPlaybackFixture().styles
	strip := ansi.Strip(loudnessStrip([]float64{0, 1, 0.5}, 8, st, true))
	if strip != "▁▁▁▁▁▁█▅" {
		t.Fatalf("strip = %q", strip)
	}
	long := syntheticLevelHistory(50)
	if got := ansi.StringWidth(loudnessStrip(long, 20, st, true)); got != 20 {
		t.Fatalf("strip is %d cells, want 20", got)
	}
	want := ansi.Strip(loudnessStrip(long[30:], 20, st, true))
	if got := ansi.Strip(loudnessStrip(long, 20, st, false)); got != want {
		t.Fatalf("a long history shows its last width steps: %q vs %q", got, want)
	}
	if loudnessStrip(long, 0, st, true) != "" {
		t.Fatal("zero width draws nothing")
	}
}

func TestNowPlayingShowsTheLoudnessStripAboveTheProgressBar(t *testing.T) {
	m := pressKey(playbackFixtureAt(t, 80, 24), "I")
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	progress := -1
	for i, line := range lines {
		if strings.Contains(line, "0:00") && strings.Count(line, "0:00") == 2 {
			progress = i
		}
	}
	if progress < 1 {
		t.Fatalf("no progress line:\n%s", strings.Join(lines, "\n"))
	}
	strip := lines[progress-1]
	if !strings.ContainsAny(strip, "▂▃▄▅▆▇█") {
		t.Fatalf("the line above the progress bar should be the loudness strip: %q", strip)
	}
	// The strip spans exactly the bar: from the cell after "0:00 " to the
	// cell before " 0:00".
	bar := lines[progress]
	first := strings.Index(bar, "0:00")
	barStart := ansi.StringWidth(bar[:first]) + len("0:00 ")
	barEnd := ansi.StringWidth(bar[:strings.LastIndex(bar, "0:00")]) - 1
	if ansi.Cut(strip, barStart-1, barStart) != " " || ansi.Cut(strip, barEnd, barEnd+1) != " " {
		t.Fatalf("the strip should not spill past the bar (cells %d-%d): %q", barStart, barEnd, strip)
	}
	if body := ansi.Cut(strip, barStart, barEnd); strings.Contains(body, " ") || ansi.StringWidth(body) != barEnd-barStart {
		t.Fatalf("the strip should fill the bar's cells %d-%d: %q", barStart, barEnd, body)
	}

	small := pressKey(playbackFixtureAt(t, 60, 15), "I")
	if strings.ContainsAny(ansi.Strip(small.View().Content), "▂▃▄▅▆▇█") {
		t.Fatal("the strip drops out first on a small screen")
	}

	off := playbackFixtureAt(t, 80, 24)
	off.cfg.LevelMeter = config.LevelMeterOff
	off = pressKey(off, "I")
	if strings.ContainsAny(ansi.Strip(off.View().Content), "▂▃▄▅▆▇█") {
		t.Fatal("level_meter off shows no strip")
	}
}

// TestLevelReadingsWaitForTheirLead guards the meter's sync with the music:
// mpv measures audio as it enters its output queue, a lead ahead of what is
// heard, so a reading must not move the bars until that lead has passed.
func TestLevelReadingsWaitForTheirLead(t *testing.T) {
	src := &stubLevelPlayer{}
	m := levelModel(t, src, 100, 30)
	lead := 500 * time.Millisecond
	loud := loudReading()
	loud.Lead = lead
	start := time.Now()

	m = m.applyLevelReading(levelReading{at: start, ok: true, levels: loud, enabled: true})
	if m.levels.bands != ([player.LevelBandCount]float64{}) || m.levels.loudness != 0 {
		t.Fatalf("a reading must not show before its lead has passed: %+v", m.levels.bands)
	}
	if m.levels.mode != levelsLive {
		t.Fatal("a held-back reading still proves the meter live")
	}

	m = m.applyLevelReading(levelReading{at: start.Add(lead - levelPollInterval), ok: true, levels: loud})
	if m.levels.loudness != 0 {
		t.Fatal("the meter moved before the first reading was heard")
	}

	m = m.applyLevelReading(levelReading{at: start.Add(lead), ok: true, levels: loud})
	if m.levels.loudness == 0 {
		t.Fatal("the first reading should show once its lead has passed")
	}
	if len(m.levels.pending) != 2 {
		t.Fatalf("readings not yet heard should stay pending: %d", len(m.levels.pending))
	}
}

func TestPausingDropsHeldBackReadings(t *testing.T) {
	src := &stubLevelPlayer{}
	m := levelModel(t, src, 100, 30)
	loud := loudReading()
	loud.Lead = time.Second
	m = m.applyLevelReading(levelReading{at: time.Now(), ok: true, levels: loud, enabled: true})
	m.playback.state = playback.StatePaused
	m, _ = m.syncLevels()
	if len(m.levels.pending) != 0 {
		t.Fatalf("paused, the held-back readings will never be heard: %d pending", len(m.levels.pending))
	}
}
