package lyrics

import (
	"context"
	"time"
)

// syntheticVerses are the words of the made-up song fixtures sing. The blank
// entry is an instrumental break.
var syntheticVerses = []string{
	"Streetlights hum a tune I almost know",
	"Every window holds a different glow",
	"We ride the last train out past the river bend",
	"Counting stations like a song that never ends",
	"",
	"Hold on, hold on, the night is wide awake",
	"Hold on, hold on, for every turn we take",
	"夜明けまで歌おう — sing until the morning light",
	"Paper lanterns drifting slowly out of sight",
	"Coast road calling through the sleeping town",
	"Radio static as the moon goes down",
	"We were young and loud and running out of time",
	"Every chorus sounded better on the line",
	"",
	"Hold on, hold on, the night is wide awake",
	"Hold on, hold on, for every turn we take",
	"And when the signal fades, I'll know the way back home",
	"Following the echo of a song we used to own",
}

// syntheticStep is the time between synthetic lines.
const syntheticStep = 8 * time.Second

// Synthetic returns the same made-up synced lyrics for every song, one line
// every eight seconds from the start, so fixtures and demos show lyrics
// without reading a file.
func Synthetic() *Lyrics {
	lines := make([]Line, len(syntheticVerses))
	for i, text := range syntheticVerses {
		lines[i] = Line{At: time.Duration(i) * syntheticStep, Text: text}
	}
	return &Lyrics{Lines: lines, Synced: true, Source: "synthetic"}
}

// SyntheticLoader serves [Synthetic] lyrics for every song.
type SyntheticLoader struct{}

// Load returns the synthetic lyrics.
func (SyntheticLoader) Load(context.Context, string) (*Lyrics, error) {
	return Synthetic(), nil
}

// Instant reports that synthetic lyrics cost nothing to make, so callers can
// load them synchronously.
func (SyntheticLoader) Instant() bool { return true }
