package tui

import (
	"log/slog"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/statusbar"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// levelSource is the part of the player that measures what is playing:
// player.Manager adds a light analysis filter to mpv and reads its levels
// back. Root checks for it via type assertion; without it the level meter
// keeps its decorative animation.
type levelSource interface {
	SetLevelMeter(enabled bool) error
	Levels() (player.Levels, bool, error)
}

// Level meter timing. Levels are read on the motion frame loop, which runs
// at levelPollInterval while something on screen shows them: about twelve
// readings a second, lively without redrawing more than the meter needs.
const (
	levelPollInterval = 80 * time.Millisecond
	// levelFallbackMisses is how many readings in a row may come back empty
	// before the first one arrives (about a second of playback) before the
	// meter gives up on this mpv and animates instead.
	levelFallbackMisses = 14
	// levelHistoryStep is how much time each column of the Now Playing
	// loudness strip covers; levelHistoryLen columns are kept, more than the
	// widest strip draws.
	levelHistoryStep = 250 * time.Millisecond
	levelHistoryLen  = 96
)

// The loudness strip maps dBFS linearly onto its height, which is already a
// perceptual scale: levelFloorDB and below rest flat, 0 dBFS fills it. RMS
// sits well under the peaks in real music, so it is lifted by levelRMSLift to
// use the same range.
const (
	levelFloorDB = -48.0
	levelRMSLift = 3.0
	// The spectrum bars map each band's level (player.Levels.Bands, a
	// relative dB scale) between bandFloorDB, which rests flat, and
	// bandCeilingDB, which fills a bar. Set from real music across genres: a
	// dense mix sits a little over halfway up, with room to bounce either way,
	// while quiet passages and thin arrangements stay low.
	bandFloorDB   = -96.0
	bandCeilingDB = -62.0
	// A rising level is followed most of the way at once; a falling one
	// sinks at levelDecayPerSec of the meter's height per second, so the
	// bars bounce with the music instead of flickering with every frame.
	levelAttack      = 0.7
	levelDecayPerSec = 1.8
)

// levelMode says where the meter's movement comes from.
type levelMode int

const (
	// levelsPending: the meter has been asked for but has not yet shown that
	// this mpv can deliver levels.
	levelsPending levelMode = iota
	// levelsLive: readings arrive; the meter follows the music.
	levelsLive
	// levelsSynthetic: no readings came, so the meter animates instead.
	levelsSynthetic
)

// levelState is the level meter: where its readings come from, the smoothed
// spectrum bars the status bar draws, the smoothed loudness, and the history
// behind the Now Playing loudness strip.
type levelState struct {
	mode    levelMode
	meterOn bool // the player has added its analysis filter
	misses  int  // empty readings while pending

	lastPoll time.Time // when the newest reading was scheduled
	lastAt   time.Time // when the newest applied reading was taken

	// pending holds readings taken but not yet heard: each is drawn once
	// lead (the player's buffering, from the newest real reading) has passed
	// since it was taken, so the bars move with the music rather than ahead
	// of it.
	pending []levelReading
	lead    time.Duration

	// bands are the spectrum bars, lowest frequency first; loudness is the
	// average of the channels' levels. 0 rests, 1 is full scale.
	bands    [player.LevelBandCount]float64
	loudness float64

	// history holds one loudness value per levelHistoryStep, oldest first;
	// bucket collects the loudest value of the step in progress.
	history     []float64
	bucket      float64
	bucketStart time.Time
}

// levelReading is one read of the player's level meter, taken on the motion
// frame's command goroutine so the update loop never waits on mpv.
type levelReading struct {
	at      time.Time
	levels  player.Levels
	ok      bool
	enabled bool // this read also turned the analysis filter on
	err     error
}

// levelSourceInUse returns the player's level meter when the music should
// drive the meter: the setting allows it, animations are on (with them off
// the meter is not drawn), and the meter has not fallen back.
func (m Model) levelSourceInUse() (levelSource, bool) {
	src, ok := m.player.(levelSource)
	if !ok || !m.motion.enabled || !m.realLevelsAllowed() {
		return nil, false
	}
	return src, true
}

// realLevelsAllowed reports whether measured levels may be drawn: the player
// can measure, level_meter is not off, and readings have not failed.
func (m Model) realLevelsAllowed() bool {
	_, ok := m.player.(levelSource)
	return ok && m.cfg.LevelMeter != config.LevelMeterOff && m.levels.mode != levelsSynthetic
}

// levelsWanted reports whether the next frame should read levels: music is
// playing and something on screen shows them. Now Playing hides the status
// bar, so there it depends on the loudness strip fitting.
func (m Model) levelsWanted() bool {
	if m.playback.state != playback.StatePlaying {
		return false
	}
	if _, ok := m.levelSourceInUse(); !ok {
		return false
	}
	if m.nowPlayingShown() {
		return m.nowPlayingLoudnessFits()
	}
	return true
}

// levelRead returns the read the frame scheduled now should carry, or nil
// when levels are not wanted or the last read is too recent: transition
// frames come faster than the meter needs.
func (m Model) levelRead(now time.Time, interval time.Duration) (Model, func() levelReading) {
	if !m.levelsWanted() {
		return m, nil
	}
	due := now.Add(interval)
	if due.Sub(m.levels.lastPoll) < levelPollInterval*9/10 {
		return m, nil
	}
	m.levels.lastPoll = due
	src, _ := m.levelSourceInUse()
	enable := !m.levels.meterOn
	return m, func() levelReading {
		if enable {
			if err := src.SetLevelMeter(true); err != nil {
				return levelReading{at: time.Now(), err: err}
			}
		}
		levels, ok, err := src.Levels()
		return levelReading{at: time.Now(), levels: levels, ok: ok, enabled: enable, err: err}
	}
}

// levelPendingLimit bounds the readings held back for their lead: a few
// seconds' worth at levelPollInterval, more than any sane audio buffer.
const levelPendingLimit = 64

// applyLevelReading takes in a reading and moves the meter toward every
// reading whose lead has passed. An empty reading lets the bars sink; enough
// of them before the first real one means this mpv cannot measure, and the
// meter falls back to its animation.
func (m Model) applyLevelReading(r levelReading) Model {
	if r.enabled {
		m.levels.meterOn = true
	}
	if r.ok {
		m.levels.mode = levelsLive
		m.levels.misses = 0
		m.levels.lead = r.levels.Lead
	} else if m.levels.mode == levelsPending {
		m.levels.misses++
		if m.levels.misses >= levelFallbackMisses {
			return m.fallBackToSyntheticLevels(r.err)
		}
	}
	m.levels.pending = append(m.levels.pending, r)
	if over := len(m.levels.pending) - levelPendingLimit; over > 0 {
		m.levels.pending = m.levels.pending[over:]
	}
	return m.drainLevels(r.at)
}

// drainLevels steps the meter through the pending readings that are heard by
// now, in order, each at the moment it is heard.
func (m Model) drainLevels(now time.Time) Model {
	heard := 0
	for _, p := range m.levels.pending {
		at := p.at.Add(m.levels.lead)
		if at.After(now) {
			break
		}
		heard++
		target := levelFrame{}
		if p.ok {
			target = levelTargets(p.levels)
		}
		m = m.stepLevels(at, target)
	}
	m.levels.pending = append(m.levels.pending[:0:0], m.levels.pending[heard:]...)
	return m
}

// fallBackToSyntheticLevels gives up on measured levels for this session.
// syncLevels then takes the filter back out of mpv's chain.
func (m Model) fallBackToSyntheticLevels(err error) Model {
	slog.Debug("level meter: no levels from mpv, animating instead", "readings", m.levels.misses, "err", err)
	meterOn := m.levels.meterOn
	m.levels = levelState{mode: levelsSynthetic, meterOn: meterOn}
	return m
}

// syncLevels keeps the meter in step with playback: the bars rest whenever
// nothing plays, and the analysis filter comes out of mpv's chain once
// nothing will read it (level_meter off, animations off, or fallen back).
func (m Model) syncLevels() (Model, tea.Cmd) {
	if m.playback.state != playback.StatePlaying {
		m.levels = m.levels.rest()
	}
	if !m.levels.meterOn {
		return m, nil
	}
	if _, inUse := m.levelSourceInUse(); inUse {
		return m, nil
	}
	m.levels.meterOn = false
	src, ok := m.player.(levelSource)
	if !ok {
		return m, nil
	}
	return m, func() tea.Msg {
		if err := src.SetLevelMeter(false); err != nil {
			slog.Debug("level meter: remove filter", "err", err)
		}
		return nil
	}
}

// rest drops the bars to the floor and closes the history step in
// progress, so playback resumes from a still meter.
func (l levelState) rest() levelState {
	l.bands, l.loudness = [player.LevelBandCount]float64{}, 0
	l.lastAt = time.Time{}
	l.pending = nil
	l.bucket, l.bucketStart = 0, time.Time{}
	return l
}

// levelFrame is what one reading asks of the meter: a height for each
// spectrum bar and the loudness, all 0..1.
type levelFrame struct {
	bands    [player.LevelBandCount]float64
	loudness float64
}

// stepLevels smooths the bars and loudness toward a reading's targets and
// records the loudness history.
func (m Model) stepLevels(at time.Time, targets levelFrame) Model {
	dt := levelPollInterval.Seconds()
	if !m.levels.lastAt.IsZero() {
		if d := at.Sub(m.levels.lastAt).Seconds(); d > 0 && d < 0.5 {
			dt = d
		}
	}
	m.levels.lastAt = at
	for i, target := range targets.bands {
		m.levels.bands[i] = smoothLevel(m.levels.bands[i], target, dt)
	}
	m.levels.loudness = smoothLevel(m.levels.loudness, targets.loudness, dt)
	m.levels = m.levels.record(at, m.levels.loudness)
	return m
}

// smoothLevel follows a rising target quickly and lets a falling one sink at
// a steady rate.
func smoothLevel(current, target, dt float64) float64 {
	if target >= current {
		return current + (target-current)*levelAttack
	}
	return math.Max(target, current-levelDecayPerSec*dt)
}

// record adds a loudness value to the history step in progress, closing it
// once it spans levelHistoryStep.
func (l levelState) record(at time.Time, loudness float64) levelState {
	if l.bucketStart.IsZero() {
		l.bucketStart = at
	}
	l.bucket = math.Max(l.bucket, loudness)
	if at.Sub(l.bucketStart) < levelHistoryStep {
		return l
	}
	l.history = append(l.history, l.bucket)
	if over := len(l.history) - levelHistoryLen; over > 0 {
		l.history = append(l.history[:0:0], l.history[over:]...)
	}
	// Steps stay levelHistoryStep apart however the readings fall, unless
	// a gap (a busy machine, a stall) left the step far behind.
	l.bucket, l.bucketStart = 0, l.bucketStart.Add(levelHistoryStep)
	if at.Sub(l.bucketStart) >= levelHistoryStep {
		l.bucketStart = at
	}
	return l
}

// levelTargets maps a reading onto the meter: each band's level sets its
// bar, and the channels' average RMS sets the loudness. Mono counts once.
func levelTargets(l player.Levels) levelFrame {
	var f levelFrame
	n := len(l.RMS)
	if n == 0 {
		return f
	}
	var sum float64
	for _, rms := range l.RMS {
		sum += dbToLevel(rms + levelRMSLift)
	}
	f.loudness = sum / float64(n)
	for i := range min(len(l.Bands), len(f.bands)) {
		f.bands[i] = dbRangeToLevel(l.Bands[i], bandFloorDB, bandCeilingDB)
	}
	return f
}

// dbToLevel maps dBFS onto 0..1 between levelFloorDB and full scale.
// Silence (negative infinity) and anything below the floor rest at 0.
func dbToLevel(db float64) float64 {
	return dbRangeToLevel(db, levelFloorDB, 0)
}

// dbRangeToLevel maps dB onto 0..1 between floor and ceiling. Silence
// (negative infinity), NaN and anything below the floor rest at 0.
func dbRangeToLevel(db, floor, ceiling float64) float64 {
	if math.IsNaN(db) || db <= floor {
		return 0
	}
	return math.Min(1, (db-floor)/(ceiling-floor))
}

// levelMeterFrame turns the smoothed bands into the status bar meter.
func levelMeterFrame(bands [player.LevelBandCount]float64) statusbar.Meter {
	return statusbar.Meter{Bands: bands[:]}
}

func levelGlyph(glyphs []rune, v float64) rune {
	idx := int(math.Round(math.Min(1, math.Max(0, v)) * float64(len(glyphs)-1)))
	return glyphs[idx]
}

// statusLevelMeter is what the status bar shows in place of ▶ while playing:
// the measured levels, or the animation when levels cannot be measured. It
// is nil with animations off, which keeps the static ▶.
func (m Model) statusLevelMeter() *statusbar.Meter {
	if !m.motion.enabled {
		return nil
	}
	meter := syntheticMeter(m.motion.frame)
	if m.realLevelsAllowed() {
		meter = levelMeterFrame(m.levels.bands)
	}
	return &meter
}

// stripGlyphs are the loudness strip's column heights.
var stripGlyphs = []rune("▁▂▃▄▅▆▇█")

// loudnessStripFade is how far toward the dim text color the oldest column
// of the loudness strip fades; newer columns are progressively brighter.
const loudnessStripFade = 0.65

// nowPlayingLoudness returns the Now Playing loudness strip renderer, or nil
// when there is nothing measured to show.
func (m Model) nowPlayingLoudness(st styles.Styles) func(int) string {
	if !m.realLevelsAllowed() || len(m.levels.history) == 0 {
		return nil
	}
	history := m.levels.history
	playing := m.playback.state == playback.StatePlaying
	return func(width int) string { return loudnessStrip(history, width, st, playing) }
}

// loudnessStrip draws the last width steps of loudness history, newest on
// the right, in the accent color fading into the past. Columns before the
// history began rest flat; with playback paused the whole strip dims.
func loudnessStrip(history []float64, width int, st styles.Styles, playing bool) string {
	if width <= 0 {
		return ""
	}
	accent := st.ProgressFilled.GetForeground()
	dim := st.Dim.GetForeground()
	pad := max(0, width-len(history))
	visible := history[max(0, len(history)-width):]
	var b strings.Builder
	b.WriteString(st.ProgressEmpty.Render(strings.Repeat(string(stripGlyphs[0]), pad)))
	for i, v := range visible {
		color := dim
		if playing {
			age := 1 - float64(pad+i+1)/float64(width)
			color = styles.Mix(accent, dim, loudnessStripFade*age)
		}
		b.WriteString(lipgloss.NewStyle().Foreground(color).Render(string(levelGlyph(stripGlyphs, v))))
	}
	return b.String()
}

// nowPlayingLoudnessFits reports whether the Now Playing view at this size
// has room for the loudness strip. It lays out the view's text without its
// cover, which is all the decision depends on.
func (m Model) nowPlayingLoudnessFits() bool {
	if m.overlay.nowPlaying == nil {
		return false
	}
	st := *m.styles
	np := m.decorateNowPlayingText(*m.overlay.nowPlaying, st, time.Now())
	np.Loudness = func(int) string { return " " }
	return np.ShowsLoudness(m.termWidth, m.termHeight, st)
}

// syntheticLevelHistory is a deterministic loudness history for fixtures,
// shaped like a song's phrases so the Now Playing strip has something
// plausible to show without mpv or animation.
func syntheticLevelHistory(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		f := float64(i)
		v := 0.58 + 0.22*math.Sin(f*0.21) + 0.12*math.Sin(f*0.83+1.3) + 0.06*math.Sin(f*2.9)
		out[i] = math.Min(1, math.Max(0, v))
	}
	return out
}
