package player

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// levelMeterLabel names the level meter's entry in mpv's af chain; its
// measurements are read back from af-metadata/<label>.
const levelMeterLabel = "muselevel"

// LevelBandCount is how many frequency bands the level meter measures.
const LevelBandCount = 10

// levelBands are the meter's octave bands, lowest first: a band-pass filter
// centred on each frequency, an octave wide. The top band is a high-pass
// above 10 kHz instead (hz is its rough centre). FFmpeg's biquads pass audio
// through unfiltered when asked for a frequency at or above half the sample
// rate, so every band stays below 11.025 kHz, the limit for 22.05 kHz audio;
// audio sampled lower still plays, though its top bars then read too tall.
var levelBands = [LevelBandCount]struct {
	hz     int
	filter string // biquad applied to the band's channel
}{
	{31, "bandpass=f=31:width_type=o:w=1"},
	{63, "bandpass=f=63:width_type=o:w=1"},
	{125, "bandpass=f=125:width_type=o:w=1"},
	{250, "bandpass=f=250:width_type=o:w=1"},
	{500, "bandpass=f=500:width_type=o:w=1"},
	{1000, "bandpass=f=1000:width_type=o:w=1"},
	{2000, "bandpass=f=2000:width_type=o:w=1"},
	{4000, "bandpass=f=4000:width_type=o:w=1"},
	{8000, "bandpass=f=8000:width_type=o:w=1"},
	{14000, "highpass=f=10000"},
}

// LevelBandHz returns the centre frequency of each band, lowest first.
func LevelBandHz() [LevelBandCount]int {
	var out [LevelBandCount]int
	for i, b := range levelBands {
		out[i] = b.hz
	}
	return out
}

// levelBandChannels name the channels that carry the bands, after FL and FR.
// Named channels let each band's biquad pick its own with c=.
var levelBandChannels = [LevelBandCount]string{"FC", "LFE", "BL", "BR", "FLC", "FRC", "BC", "SL", "SR", "TC"}

// levelMeterFilter measures the playing audio, per channel and per frequency
// band, and attaches the measurements to each frame as metadata. One linear
// chain does it, because FFmpeg drops frame metadata wherever streams are
// split and merged again:
//
//  1. the audio is made stereo (mono is copied to both sides; anything wider
//     is downmixed, as a stereo output device would) and widened with one
//     extra channel per band, each holding the mono mix;
//  2. each extra channel is filtered down to its band;
//  3. aspectralstats measures every channel's mean spectral magnitude over
//     each window of levelWindowSamples: channels 1-2 are the left and right
//     levels, 3 onward the bands, lowest first;
//  4. the band channels are dropped again, which keeps the metadata.
//
// aspectralstats rather than astats, which reports true RMS levels but costs
// several times as much per channel; with twelve channels that came to about
// five percent of a core, where this chain stays near one percent at 44.1
// kHz. Left and right leave the filter untouched.
var levelMeterFilter = buildLevelMeterFilter()

func buildLevelMeterFilter() string {
	layout := "FL+FR+" + strings.Join(levelBandChannels[:], "+")
	widen := []string{"pan=" + layout, "c0=c0", "c1=c1"}
	filters := make([]string, 0, LevelBandCount)
	for i, band := range levelBands {
		widen = append(widen, fmt.Sprintf("c%d=0.5*c0+0.5*c1", i+2))
		filters = append(filters, band.filter+":c="+levelBandChannels[i])
	}
	return "@" + levelMeterLabel + ":lavfi=[aformat=channel_layouts=stereo," +
		strings.Join(widen, "|") + "," + strings.Join(filters, ",") +
		fmt.Sprintf(",aspectralstats=win_size=%d:overlap=0:measure=mean,", levelWindowSamples) +
		"pan=stereo|c0=c0|c1=c1]"
}

// levelWindowSamples is the analysis window: about 23 ms at 44.1 kHz, so
// each reading describes the moment.
const levelWindowSamples = 1024

// spectralToRMSDB converts a channel's mean spectral magnitude, in dB, to
// its approximate RMS level in dBFS. It is measured against astats on real
// music, where it holds within a few dB across genres. It is not exact for
// every sound: a lone sine tone, whose energy sits in a few bins rather than
// spread across the spectrum, reads about 10 dB low.
const spectralToRMSDB = 46.0

// propLevelMeter is the property that holds the meter's latest measurements.
const propLevelMeter = "af-metadata/" + levelMeterLabel

// Levels is one reading of the level meter. RMS is each channel's
// approximate RMS level in dBFS (0 is full scale), in channel order. Bands
// is each frequency band's mean spectral magnitude in dB, lowest band
// first: a relative scale, on which wider (higher) bands read a little
// louder, roughly offsetting music's natural fall toward the treble.
// Silence reads as negative infinity.
type Levels struct {
	RMS   []float64
	Bands []float64
}

// Channels is how many channels the reading describes.
func (l Levels) Channels() int { return len(l.RMS) }

// SetLevelMeter adds the level meter's analysis filter to mpv's audio filters
// or removes it. It applies only to a connected mpv, never queued for replay
// at connection: a filter mpv cannot build fails the file that loads after
// it, so the caller turns the meter on while a song plays (where mpv keeps
// playing if the filter is missing) and checks that readings arrive before
// relying on it. Once on, the meter is part of the chain that the restart
// supervisor restores.
func (m *Manager) SetLevelMeter(enabled bool) error {
	m.mu.Lock()
	if m.client == nil || m.activating {
		m.mu.Unlock()
		return ErrNotConnected
	}
	if m.levelMeter == enabled {
		m.mu.Unlock()
		return nil
	}
	m.levelMeter = enabled
	m.mu.Unlock()
	if err := m.rebuildAudioFilters(); err != nil {
		m.mu.Lock()
		m.levelMeter = !enabled
		m.mu.Unlock()
		return err
	}
	return nil
}

// Levels returns the level meter's latest reading. ok is false when there is
// none to give: the meter is off, nothing has played through it yet, or mpv
// is between files. Readings are not logged, even at debug level, because
// the interface asks for them several times a second.
func (m *Manager) Levels() (Levels, bool, error) {
	m.mu.RLock()
	c, on := m.client, m.levelMeter
	m.mu.RUnlock()
	if c == nil {
		return Levels{}, false, ErrNotConnected
	}
	if !on {
		return Levels{}, false, nil
	}
	resp, err := c.sendQuiet(cmdGetProperty, propLevelMeter)
	if errors.Is(err, ErrPlaybackFailed) {
		// "property unavailable": no frame has passed through the filter.
		return Levels{}, false, nil
	}
	if err != nil {
		return Levels{}, false, err
	}
	if resp.isNullData() {
		return Levels{}, false, nil
	}
	return parseLevelMetadata(resp.Data)
}

// parseLevelMetadata reads aspectralstats' frame metadata, which mpv
// reports as an object of strings keyed
// "lavfi.aspectralstats.<channel>.mean" with channels numbered from 1.
// Channels past left and right are the bands. A value that is not a positive
// number reads as silence.
func parseLevelMetadata(data json.RawMessage) (Levels, bool, error) {
	var fields map[string]string
	if err := json.Unmarshal(data, &fields); err != nil {
		return Levels{}, false, fmt.Errorf("level meter: unexpected metadata %s: %w", data, err)
	}
	var levels []float64
	for key, raw := range fields {
		channel, ok := spectralMeanKey(key)
		if !ok {
			continue
		}
		db := math.Inf(-1)
		if value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && value > 0 && !math.IsInf(value, 0) {
			db = 20 * math.Log10(value)
		}
		levels = setChannel(levels, channel, db)
	}
	var out Levels
	if len(levels) > 2 {
		out.Bands = levels[2:]
		levels = levels[:2]
	}
	for _, db := range levels {
		out.RMS = append(out.RMS, db+spectralToRMSDB)
	}
	return out, out.Channels() > 0, nil
}

// levelChannelLimit bounds the channel numbers accepted from the metadata,
// so a malformed key cannot ask for an enormous slice.
const levelChannelLimit = 64

// spectralMeanKey reads the zero-based channel from
// "lavfi.aspectralstats.2.mean" (here 1); other keys are not levels.
func spectralMeanKey(key string) (channel int, ok bool) {
	rest, found := strings.CutPrefix(key, "lavfi.aspectralstats.")
	if !found {
		return 0, false
	}
	num, found := strings.CutSuffix(rest, ".mean")
	if !found {
		return 0, false
	}
	n, err := strconv.Atoi(num)
	if err != nil || n < 1 || n > levelChannelLimit {
		return 0, false
	}
	return n - 1, true
}

// setChannel stores value at index channel, growing values with silence as
// needed.
func setChannel(values []float64, channel int, value float64) []float64 {
	for len(values) <= channel {
		values = append(values, math.Inf(-1))
	}
	values[channel] = value
	return values
}
