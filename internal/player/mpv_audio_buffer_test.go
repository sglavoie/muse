package player

import (
	"slices"
	"testing"
	"time"
)

// TestLaunchArgs_DeepensTheAudioBuffer guards the --audio-buffer flag: with
// mpv's default 0.2 s queue, a system-wide stall of half a second starves the
// audio device and the music stutters.
func TestLaunchArgs_DeepensTheAudioBuffer(t *testing.T) {
	args := launchArgs("/tmp/muse.sock", nil)
	if !slices.Contains(args, "--audio-buffer=1") {
		t.Errorf("launchArgs = %v, want --audio-buffer=1", args)
	}
}

func TestAudioBufferSecs(t *testing.T) {
	cases := []struct {
		name  string
		extra []string
		want  float64
	}{
		{"default", nil, defaultAudioBufferSecs},
		{"unrelated flags", []string{"--ao=coreaudio"}, defaultAudioBufferSecs},
		{"equals form", []string{"--audio-buffer=0.5"}, 0.5},
		{"separate value", []string{"--audio-buffer", "2"}, 2},
		{"last one wins", []string{"--audio-buffer=0.5", "--audio-buffer=3"}, 3},
		{"invalid ignored", []string{"--audio-buffer=lots"}, defaultAudioBufferSecs},
		{"negative ignored", []string{"--audio-buffer=-1"}, defaultAudioBufferSecs},
		{"dangling flag ignored", []string{"--audio-buffer"}, defaultAudioBufferSecs},
	}
	for _, tc := range cases {
		if got := audioBufferSecs(tc.extra); got != tc.want {
			t.Errorf("%s: audioBufferSecs(%v) = %v, want %v", tc.name, tc.extra, got, tc.want)
		}
	}
}

func TestLevelLeadFollowsTheAudioBuffer(t *testing.T) {
	if got, want := levelLead(nil), time.Second+levelDeviceLatency; got != want {
		t.Errorf("levelLead(nil) = %v, want %v", got, want)
	}
	if got, want := levelLead([]string{"--audio-buffer=0.2"}), 200*time.Millisecond+levelDeviceLatency; got != want {
		t.Errorf("levelLead(0.2 s) = %v, want %v", got, want)
	}
}
