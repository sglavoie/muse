package tui

import (
	"image/color"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sglavoie/muse/internal/artwork"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Frame intervals. Playback alone only needs the progress bar to glide and
// the playing indicator to move, which a few frames a second does; a
// transition gets a brief burst of smoother frames.
const (
	motionSteadyFrame     = 250 * time.Millisecond
	motionTransitionFrame = 50 * time.Millisecond
	motionPulseDuration   = 700 * time.Millisecond
	motionAccentDuration  = 900 * time.Millisecond
	motionOverlayDuration = 180 * time.Millisecond
	motionViewDuration    = 160 * time.Millisecond
	// motionViewFloor is how visible a newly selected view starts: switching
	// tabs eases the panel in without blanking it, so quick tabbing never
	// flashes an empty screen.
	motionViewFloor = 0.3
	// motionMaxGlide bounds how far the progress bar runs ahead of the last
	// position mpv reported, so a stalled player never looks like it plays.
	motionMaxGlide = 1.5
)

// motionState drives every interface animation from one frame loop that only
// runs while something is moving.
type motionState struct {
	enabled   bool
	scheduled bool
	frame     int

	sampledAt time.Time // when playback.elapsedSecs was last read

	pulseStart time.Time

	overlayOpenedAt time.Time // when the overlay on screen started fading in
	viewChangedAt   time.Time // when the active view was last switched

	// The accent animates from accentFrom to accentTo starting at
	// accentStart. accentTarget is the hex the transition settles on; ""
	// means the configured primary color.
	accentFrom, accentTo color.Color
	accentStart          time.Time
	accentTarget         string
	accentKnown          bool // accentTarget has been decided at least once
	// accentFromArt records whether the transition started from a cover's
	// accent, so the Now Playing background tint can ease out with it.
	accentFromArt bool

	// titleSince is when the Now Playing title last started from its
	// beginning: the marquee counts its steps from here.
	titleSince time.Time

	// coverFrom is the previous song's cover while Now Playing cross-fades
	// to the new one, starting at coverFadeStart; nil when not fading.
	coverFrom      *artwork.Art
	coverFadeStart time.Time
}

// motionFrameMsg advances every running animation by one frame. levels is
// the level meter reading taken for this frame, when one was due.
type motionFrameMsg struct {
	levels *levelReading
}

// WithMotion enables interface animations. It also marks the model as one
// that may animate at all, so turning Animations on in Settings takes effect
// without a restart; models built without it (tests, the fixture harness)
// never animate.
func WithMotion(enabled bool) Option {
	return func(m *Model) {
		m.motion.enabled = enabled
		m.motionAllowed = true
	}
}

func (m Model) pulseActive(now time.Time) bool {
	return !m.motion.pulseStart.IsZero() && now.Sub(m.motion.pulseStart) < motionPulseDuration
}

func (m Model) overlayRevealing(now time.Time) bool {
	return !m.motion.overlayOpenedAt.IsZero() && now.Sub(m.motion.overlayOpenedAt) < motionOverlayDuration
}

// overlayReveal is how far the opening overlay has faded in, from 0 to 1.
func (m Model) overlayReveal(now time.Time) float64 {
	if !m.motion.enabled || !m.overlayRevealing(now) {
		return 1
	}
	return easeOutCubic(now.Sub(m.motion.overlayOpenedAt).Seconds() / motionOverlayDuration.Seconds())
}

func (m Model) viewRevealing(now time.Time) bool {
	return !m.motion.viewChangedAt.IsZero() && now.Sub(m.motion.viewChangedAt) < motionViewDuration
}

// viewReveal is how far a newly selected view has faded in, from
// motionViewFloor to 1.
func (m Model) viewReveal(now time.Time) float64 {
	if !m.motion.enabled || !m.viewRevealing(now) {
		return 1
	}
	t := easeOutCubic(now.Sub(m.motion.viewChangedAt).Seconds() / motionViewDuration.Seconds())
	return motionViewFloor + (1-motionViewFloor)*t
}

// overlayFamily groups overlays that are one dialog, such as the Auto-DJ
// panel's tabs, so moving between them does not fade the dialog in again.
func overlayFamily(name string) string {
	family, _, _ := strings.Cut(name, "_")
	if family == "autodj" {
		return family
	}
	return name
}

func (m Model) accentAnimating(now time.Time) bool {
	return m.motion.accentTo != nil && now.Sub(m.motion.accentStart) < motionAccentDuration
}

// transitionActive reports whether any short transition is running, which
// asks for the smoother frame rate.
func (m Model) transitionActive(now time.Time) bool {
	return m.pulseActive(now) || m.accentAnimating(now) || m.overlayRevealing(now) ||
		m.viewRevealing(now) || m.coverFading(now) || m.trackCardSliding(now)
}

// scheduleMotion starts the frame loop when an animation needs it.
func (m Model) scheduleMotion() (Model, tea.Cmd) {
	if !m.motion.enabled || m.motion.scheduled {
		return m, nil
	}
	now := time.Now()
	interval := time.Duration(0)
	switch {
	case m.transitionActive(now):
		interval = motionTransitionFrame
	case m.levelsWanted():
		interval = levelPollInterval
	case m.playback.state == playback.StatePlaying:
		interval = motionSteadyFrame
	default:
		return m, nil
	}
	m.motion.scheduled = true
	m, read := m.levelRead(now, interval)
	return m, tea.Tick(interval, func(time.Time) tea.Msg {
		if read == nil {
			return motionFrameMsg{}
		}
		r := read()
		return motionFrameMsg{levels: &r}
	})
}

// handleMotionFrameMsg advances the animations and applies the level reading
// the frame carries.
func (m Model) handleMotionFrameMsg(msg motionFrameMsg) Model {
	m = m.handleMotionFrame()
	if msg.levels != nil {
		m = m.applyLevelReading(*msg.levels)
	}
	return m
}

func (m Model) handleMotionFrame() Model {
	m.motion.scheduled = false
	m.motion.frame++
	now := time.Now()
	if m.motion.accentTo != nil {
		m = m.stepAccent(now)
	}
	if m.motion.coverFrom != nil && !m.coverFading(now) && !m.playingArtPending() {
		m.motion.coverFrom = nil
	}
	return m
}

// noteMotionChanges records what an update changed that animations follow:
// a new track starts the row pulse, and any play/pause change restarts the
// progress glide from the position mpv last reported.
func (m Model) noteMotionChanges(prev Model) Model {
	if !m.motion.enabled {
		return m
	}
	if m.playback.state != prev.playback.state || m.playback.elapsedSecs != prev.playback.elapsedSecs {
		m.motion.sampledAt = time.Now()
	}
	if m.playback.currentHash != prev.playback.currentHash && m.playback.currentHash != "" {
		m.motion.pulseStart = time.Now()
	}
	// An overlay opening, or another one replacing it, fades in; one closing
	// returns to the view at once.
	if name := m.ActiveOverlayName(); name == "" {
		m.motion.overlayOpenedAt = time.Time{}
	} else if overlayFamily(name) != overlayFamily(prev.ActiveOverlayName()) {
		m.motion.overlayOpenedAt = time.Now()
	}
	// Switching views (tabs, number keys, or a dialog that jumps to one)
	// eases the new panel in.
	if m.ActiveView != prev.ActiveView {
		m.motion.viewChangedAt = time.Now()
	}
	return m.noteNowPlayingMotion(prev)
}

// displayElapsed is the playback position to draw: the last reading, carried
// forward by wall-clock time while playing so the progress bar glides between
// the once-a-second position reads.
func (m Model) displayElapsed(now time.Time) float64 {
	elapsed := m.playback.elapsedSecs
	if !m.motion.enabled || m.playback.state != playback.StatePlaying || m.motion.sampledAt.IsZero() || elapsed <= 0 {
		return elapsed
	}
	glide := math.Min(now.Sub(m.motion.sampledAt).Seconds(), motionMaxGlide)
	elapsed += math.Max(0, glide)
	if m.playback.totalSecs > 0 {
		elapsed = math.Min(elapsed, m.playback.totalSecs)
	}
	return elapsed
}

// pulseStyles fades the now-playing row from an accent wash back to its
// resting colors over motionPulseDuration after a track change.
func (m Model) pulseStyles(s styles.Styles, now time.Time) styles.Styles {
	t := now.Sub(m.motion.pulseStart).Seconds() / motionPulseDuration.Seconds()
	strength := 0.55 * (1 - easeOutCubic(t))
	wash := func(st lipgloss.Style) lipgloss.Style {
		bg := st.GetBackground()
		if _, unset := bg.(lipgloss.NoColor); unset {
			bg = s.Background
		}
		return st.Background(styles.Mix(bg, s.Primary, strength))
	}
	s.NowPlayingRow = wash(s.NowPlayingRow)
	s.NowPlayingRowFavorite = wash(s.NowPlayingRowFavorite)
	s.NowPlayingRowFavArtist = wash(s.NowPlayingRowFavArtist)
	s.NowPlayingRowFavAlbum = wash(s.NowPlayingRowFavAlbum)
	s.NowPlayingTitle = wash(s.NowPlayingTitle)
	return s
}

func easeOutCubic(t float64) float64 {
	t = math.Min(1, math.Max(0, t))
	u := 1 - t
	return 1 - u*u*u
}

func easeInOutCubic(t float64) float64 {
	t = math.Min(1, math.Max(0, t))
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := -2*t + 2
	return 1 - u*u*u/2
}

// eqBars returns the animated three-bar level meter shown while playing when
// the music's own levels cannot be measured.
func eqBars(frame int) string {
	levels := []rune("▁▂▃▄▅▆▇")
	out := make([]rune, 3)
	for i := range out {
		f := float64(frame)
		v := 0.5 + 0.3*math.Sin(f*0.9+float64(i)*2.1) + 0.2*math.Sin(f*1.7+float64(i)*0.7)
		idx := int(math.Round(v * float64(len(levels)-1)))
		out[i] = levels[max(0, min(len(levels)-1, idx))]
	}
	return string(out)
}

// refreshArtworkAccent points the interface accent at the playing track's
// cover color, or back at the configured primary color when there is none.
func (m Model) refreshArtworkAccent() Model {
	target, decided := m.artworkAccentTarget()
	if !decided {
		return m
	}
	if m.motion.accentKnown && target == m.motion.accentTarget {
		return m
	}
	first := !m.motion.accentKnown
	m.motion.accentFromArt = m.motion.accentKnown && m.motion.accentTarget != ""
	m.motion.accentKnown = true
	m.motion.accentTarget = target
	if !m.motion.enabled || first {
		m.accentOverride = target
		return m.applyTheme()
	}
	m.motion.accentFrom = m.styles.Primary
	m.motion.accentTo = m.accentTargetColor(target)
	m.motion.accentStart = time.Now()
	return m
}

// artworkAccentTarget returns the hex the accent should settle on ("" for the
// configured primary color). decided is false while the playing track's cover
// is still being looked at: the current tint stays until then, so a track
// change does not flash through the default color.
func (m Model) artworkAccentTarget() (target string, decided bool) {
	if !m.cfg.ArtworkAccent || m.playback.state == playback.StateIdle || m.playback.state == playback.StateStopped {
		return "", true
	}
	t, ok := m.playingTrack()
	if !ok {
		return "", true
	}
	art, done := m.artFor(t)
	switch {
	case !done:
		return "", false
	case art != nil && art.HasAccent:
		return styles.Hex(art.Accent), true
	default:
		return "", true
	}
}

// accentTargetColor resolves a target hex ("" = configured primary) to the
// fill color the theme would use for it.
func (m Model) accentTargetColor(target string) color.Color {
	_, primary := m.themeState()
	if target != "" {
		primary = target
	}
	return styles.NewTheme(m.styles.HasDark, primary).Primary
}

func (m Model) stepAccent(now time.Time) Model {
	t := now.Sub(m.motion.accentStart).Seconds() / motionAccentDuration.Seconds()
	if t >= 1 {
		m.motion.accentFrom, m.motion.accentTo = nil, nil
		m.accentOverride = m.motion.accentTarget
		return m.applyTheme()
	}
	m.accentOverride = styles.Hex(styles.Mix(m.motion.accentFrom, m.motion.accentTo, easeInOutCubic(t)))
	return m.applyTheme()
}
