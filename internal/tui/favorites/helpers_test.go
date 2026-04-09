package favorites

import "github.com/sglavoie/muse/internal/library"

func sampleTracks(n int) []library.Track {
	out := make([]library.Track, n)
	for i := range out {
		out[i] = library.Track{
			Hash:   "h" + string(rune('a'+i)),
			Title:  "t" + string(rune('0'+i)),
			Artist: "a" + string(rune('0'+i)),
			Album:  "al" + string(rune('0'+i)),
		}
	}
	return out
}
