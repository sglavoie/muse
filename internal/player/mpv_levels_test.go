package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// levelTestManager connects a Manager to a fake mpv whose replies come from
// handler, recording every command it receives.
func levelTestManager(t *testing.T, name string, handler responseHandler) (*Manager, func() []ipcTestCommand) {
	t.Helper()
	sock := shortSockPath(t, name)
	var mu sync.Mutex
	var received []ipcTestCommand
	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		recordCommand(&mu, &received, req.Command)
		return handler(req)
	})
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, audioFilters: make(map[string]string)}
	t.Cleanup(func() { _ = m.Close() })
	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()
	return m, func() []ipcTestCommand {
		mu.Lock()
		defer mu.Unlock()
		return append([]ipcTestCommand(nil), received...)
	}
}

// lastAF returns the last value the manager set af to.
func lastAF(t *testing.T, cmds []ipcTestCommand) string {
	t.Helper()
	for i := len(cmds) - 1; i >= 0; i-- {
		if cmds[i].stringIs(0, cmdSetProperty) && cmds[i].stringIs(1, propAF) {
			v, _ := cmds[i].stringAt(2)
			return v
		}
	}
	t.Fatalf("af was never set: %v", cmds)
	return ""
}

func TestAudioFilterChainKeepsMeterLast(t *testing.T) {
	cases := []struct {
		filters map[string]string
		meter   bool
		want    string
	}{
		{nil, false, ""},
		{nil, true, levelMeterFilter},
		{map[string]string{"loudnorm": "loudnorm"}, false, "lavfi=[loudnorm]"},
		{map[string]string{"loudnorm": "loudnorm", "equalizer": "eq"}, true, "lavfi=[eq,loudnorm]," + levelMeterFilter},
	}
	for _, c := range cases {
		if got := audioFilterChain(c.filters, c.meter); got != c.want {
			t.Errorf("audioFilterChain(%v, %v) = %q, want %q", c.filters, c.meter, got, c.want)
		}
	}
}

func TestSetLevelMeterSurvivesOtherFilterChanges(t *testing.T) {
	m, received := levelTestManager(t, "lvl-chain", successHandler)
	if err := m.SetLevelMeter(true); err != nil {
		t.Fatalf("SetLevelMeter(true): %v", err)
	}
	if got := lastAF(t, received()); got != levelMeterFilter {
		t.Fatalf("meter alone: af = %q", got)
	}
	// Normalization and the equalizer rebuild the chain; the meter stays.
	if err := m.SetNormalize(true); err != nil {
		t.Fatalf("SetNormalize: %v", err)
	}
	if got := lastAF(t, received()); got != "lavfi=[loudnorm],"+levelMeterFilter {
		t.Fatalf("after normalize: af = %q", got)
	}
	if err := m.SetEqualizer(false, [10]int{}); err != nil {
		t.Fatalf("SetEqualizer: %v", err)
	}
	if got := lastAF(t, received()); got != "lavfi=[loudnorm],"+levelMeterFilter {
		t.Fatalf("after equalizer off: af = %q", got)
	}
	// Turning the meter off leaves the other filters exactly as they were.
	if err := m.SetLevelMeter(false); err != nil {
		t.Fatalf("SetLevelMeter(false): %v", err)
	}
	if got := lastAF(t, received()); got != "lavfi=[loudnorm]" {
		t.Fatalf("meter off: af = %q", got)
	}
	n := len(received())
	if err := m.SetLevelMeter(false); err != nil || len(received()) != n {
		t.Fatalf("a repeated SetLevelMeter(false) should send nothing (err %v)", err)
	}
}

func TestSetLevelMeterRevertsOnFailureAndNeverQueues(t *testing.T) {
	m, _ := levelTestManager(t, "lvl-fail", errorHandler("error running command"))
	if err := m.SetLevelMeter(true); err == nil {
		t.Fatal("a rejected af change should fail")
	}
	m.mu.RLock()
	on := m.levelMeter
	m.mu.RUnlock()
	if on {
		t.Fatal("a failed SetLevelMeter must not leave the meter marked on")
	}

	idle := &Manager{audioFilters: map[string]string{}, cmdQueue: []func() error{}}
	if err := idle.SetLevelMeter(true); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("before mpv connects the meter is refused, not queued: %v", err)
	}
	if len(idle.cmdQueue) != 0 || idle.levelMeter {
		t.Fatal("nothing may be queued for replay")
	}
}

func TestLevelsReadsSpectralMetadata(t *testing.T) {
	meta := `{"lavfi.aspectralstats.1.mean":"0.001","lavfi.aspectralstats.2.mean":"0"}`
	m, received := levelTestManager(t, "lvl-read", func(req ipcTestRequest) ipcTestResponse {
		if ipcTestCommand(req.Command).stringIs(1, propLevelMeter) {
			resp := successHandler(req)
			resp.Data = json.RawMessage(meta)
			return resp
		}
		return successHandler(req)
	})
	if _, ok, err := m.Levels(); ok || err != nil {
		t.Fatalf("meter off: want no reading and no error, got ok=%v err=%v", ok, err)
	}
	if err := m.SetLevelMeter(true); err != nil {
		t.Fatal(err)
	}
	levels, ok, err := m.Levels()
	if err != nil || !ok {
		t.Fatalf("Levels: ok=%v err=%v", ok, err)
	}
	if levels.Channels() != 2 || math.Abs(levels.RMS[0]-(-60+spectralToRMSDB)) > 1e-9 || !math.IsInf(levels.RMS[1], -1) {
		t.Fatalf("levels = %+v", levels)
	}
	reads := 0
	for _, c := range received() {
		if c.stringIs(0, cmdGetProperty) && c.stringIs(1, propLevelMeter) {
			reads++
		}
	}
	if reads != 1 {
		t.Fatalf("want one af-metadata read, got %d", reads)
	}
}

func TestLevelsUnavailableIsNoReading(t *testing.T) {
	m, _ := levelTestManager(t, "lvl-unavail", func(req ipcTestRequest) ipcTestResponse {
		if ipcTestCommand(req.Command).stringIs(0, cmdGetProperty) {
			return errorResponse(req, "property unavailable")
		}
		return successHandler(req)
	})
	if err := m.SetLevelMeter(true); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := m.Levels(); ok || err != nil {
		t.Fatalf("unavailable property: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}
	disconnected := &Manager{}
	if _, _, err := disconnected.Levels(); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("disconnected: %v", err)
	}
}

func TestParseLevelMetadata(t *testing.T) {
	cases := []struct {
		name     string
		data     string
		ok       bool
		channels int
	}{
		{"mono", `{"lavfi.aspectralstats.1.mean":"0.0002"}`, true, 1},
		{"ignores other keys", `{"lavfi.r128.M":"-20","lavfi.aspectralstats.1.mean":"0.0002","lavfi.aspectralstats.1.flux":"3","lavfi.astats.1.RMS_level":"-9"}`, true, 1},
		{"other measures only", `{"lavfi.aspectralstats.1.flux":"3"}`, false, 0},
		{"empty", `{}`, false, 0},
		{"absurd channel", `{"lavfi.aspectralstats.100000.mean":"0.1"}`, false, 0},
	}
	for _, c := range cases {
		levels, ok, err := parseLevelMetadata(json.RawMessage(c.data))
		if err != nil || ok != c.ok || levels.Channels() != c.channels {
			t.Errorf("%s: ok=%v channels=%d err=%v", c.name, ok, levels.Channels(), err)
		}
	}
	levels, _, _ := parseLevelMetadata(json.RawMessage(`{"lavfi.aspectralstats.1.mean":"nan","lavfi.aspectralstats.2.mean":"-1","lavfi.aspectralstats.3.mean":"garbage"}`))
	for i, v := range append(levels.RMS, levels.Bands...) {
		if !math.IsInf(v, -1) {
			t.Fatalf("value %d: unreadable or non-positive values should read as silence: %+v", i, levels)
		}
	}
	if _, _, err := parseLevelMetadata(json.RawMessage(`"not an object"`)); err == nil {
		t.Fatal("non-object metadata should be an error")
	}
}

func TestParseLevelMetadataSplitsOffTheBands(t *testing.T) {
	fields := map[string]string{}
	for ch := 1; ch <= 2+LevelBandCount; ch++ {
		// Channel ch reads -20*ch dB.
		fields[fmt.Sprintf("lavfi.aspectralstats.%d.mean", ch)] = strconv.FormatFloat(math.Pow(10, -float64(ch)), 'g', -1, 64)
	}
	data, _ := json.Marshal(fields)
	levels, ok, err := parseLevelMetadata(data)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-9 }
	if levels.Channels() != 2 || !near(levels.RMS[1], -40+spectralToRMSDB) {
		t.Fatalf("channels 1-2 are left and right, as RMS: %+v", levels)
	}
	if len(levels.Bands) != LevelBandCount || !near(levels.Bands[0], -60) || !near(levels.Bands[LevelBandCount-1], -20*(2+LevelBandCount)) {
		t.Fatalf("channels 3 onward are the bands, lowest first: %v", levels.Bands)
	}
}

func TestLevelMeterFilterGivesEachBandItsChannel(t *testing.T) {
	for i, band := range levelBands {
		want := band.filter + ":c=" + levelBandChannels[i]
		if !strings.Contains(levelMeterFilter, want) {
			t.Errorf("band %d Hz: filter lacks %q", band.hz, want)
		}
	}
	if strings.Count(levelMeterFilter, "=0.5*c0+0.5*c1") != LevelBandCount {
		t.Errorf("each band channel should carry the mono mix: %s", levelMeterFilter)
	}
	if !strings.HasSuffix(levelMeterFilter, ",aspectralstats=win_size=1024:overlap=0:measure=mean,pan=stereo|c0=c0|c1=c1]") {
		t.Errorf("the band channels must be dropped after they are measured: %s", levelMeterFilter)
	}
}
