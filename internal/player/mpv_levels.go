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

// levelMeterFilter measures the peak and RMS level of every audio frame, per
// channel, and attaches them to the frame as metadata. reset=1 starts each
// frame afresh, so a reading describes the moment rather than the song so far,
// and limiting the measurements to the two it needs keeps the analysis to a
// fraction of a percent of one core. The filter passes audio through
// unchanged.
const levelMeterFilter = "@" + levelMeterLabel + ":lavfi=[astats=metadata=1:reset=1:" +
	"measure_perchannel=Peak_level+RMS_level:measure_overall=none]"

// propLevelMeter is the property that holds the meter's latest measurements.
const propLevelMeter = "af-metadata/" + levelMeterLabel

// Levels is one reading of the level meter: the peak and RMS level of the
// audio frame playing, in dBFS (0 is full scale), per channel in channel
// order. Silence reads as negative infinity.
type Levels struct {
	Peak []float64
	RMS  []float64
}

// Channels is how many channels the reading describes.
func (l Levels) Channels() int { return min(len(l.Peak), len(l.RMS)) }

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

// parseLevelMetadata reads astats' frame metadata, which mpv reports as an
// object of strings keyed "lavfi.astats.<channel>.<measure>" with channels
// numbered from 1. A value that is not a number (astats writes "-inf" for
// silence, which ParseFloat accepts) reads as silence.
func parseLevelMetadata(data json.RawMessage) (Levels, bool, error) {
	var fields map[string]string
	if err := json.Unmarshal(data, &fields); err != nil {
		return Levels{}, false, fmt.Errorf("level meter: unexpected metadata %s: %w", data, err)
	}
	var levels Levels
	for key, raw := range fields {
		channel, measure, ok := astatsKey(key)
		if !ok {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || math.IsNaN(value) {
			value = math.Inf(-1)
		}
		switch measure {
		case "Peak_level":
			levels.Peak = setChannel(levels.Peak, channel, value)
		case "RMS_level":
			levels.RMS = setChannel(levels.RMS, channel, value)
		}
	}
	return levels, levels.Channels() > 0, nil
}

// astatsChannelLimit bounds the channel numbers accepted from the metadata,
// so a malformed key cannot ask for an enormous slice.
const astatsChannelLimit = 64

// astatsKey splits "lavfi.astats.2.RMS_level" into channel index 1 (zero
// based) and "RMS_level".
func astatsKey(key string) (channel int, measure string, ok bool) {
	rest, found := strings.CutPrefix(key, "lavfi.astats.")
	if !found {
		return 0, "", false
	}
	num, measure, found := strings.Cut(rest, ".")
	if !found {
		return 0, "", false
	}
	n, err := strconv.Atoi(num)
	if err != nil || n < 1 || n > astatsChannelLimit {
		return 0, "", false
	}
	return n - 1, measure, true
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
