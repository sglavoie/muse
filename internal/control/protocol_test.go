package control

import (
	"encoding/json"
	"testing"
)

// The wire names are the contract with muse.app's Swift client: a rename here
// silently breaks the host, so each one is pinned.

func marshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestProtocol_PinnedNames(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"request", Request{ID: 3, Op: OpTogglePlay}, `{"id":3,"op":"toggle-play"}`},
		{"reply minimal", Reply{ID: 3, OK: true}, `{"reply":3,"ok":true}`},
		{
			"reply with state",
			Reply{ID: 4, OK: true, Message: "m", State: &State{Playback: PlaybackStopped}},
			`{"reply":4,"ok":true,"message":"m","state":{"playback":"stopped","elapsed":0}}`,
		},
		{
			"hello",
			Event{Event: EventHello, Hello: &Hello{PID: 9, Version: "v1.2.3", Protocol: ProtocolVersion}},
			`{"event":"hello","hello":{"pid":9,"version":"v1.2.3","protocol":1}}`,
		},
		{
			"state",
			Event{Event: EventState, State: &State{
				Playback: PlaybackPlaying,
				Track:    &Track{ID: "h", QueueID: 7, Title: "T", Artist: "A", Album: "B", Duration: 181.5},
				Elapsed:  12.25,
			}},
			`{"event":"state","state":{"playback":"playing","track":{"id":"h","queueId":7,"title":"T","artist":"A","album":"B","duration":181.5},"elapsed":12.25}}`,
		},
		{
			"notify",
			Event{Event: EventNotify, Notify: &Notification{Title: "T", Subtitle: "A", Body: "B"}},
			`{"event":"notify","notify":{"title":"T","subtitle":"A","body":"B"}}`,
		},
		{"quitting", Event{Event: EventQuitting}, `{"event":"quitting"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := marshal(t, tt.v); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestProtocol_OpAndEventNames(t *testing.T) {
	names := map[string]string{
		OpTogglePlay:    "toggle-play",
		OpNext:          "next",
		OpPrevious:      "previous",
		OpVolumeUp:      "volume-up",
		OpVolumeDown:    "volume-down",
		OpState:         "state",
		EventHello:      "hello",
		EventNotify:     "notify",
		EventQuitting:   "quitting",
		PlaybackPlaying: "playing",
		PlaybackPaused:  "paused",
	}
	for got, want := range names {
		if got != want {
			t.Errorf("name = %q, want %q", got, want)
		}
	}
}
