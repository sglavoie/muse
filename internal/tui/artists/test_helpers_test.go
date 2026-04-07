package artists

import "github.com/sglavoie/muse/internal/library"

func sampleTracks() []library.Track {
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Duration: 183},
		{Artist: "Alice", Album: "Alpha", Title: "Track A2", Duration: 245},
		{Artist: "Alice", Album: "Beta", Title: "Track B1", Duration: 300},
		{Artist: "Bob", Album: "Gamma", Title: "Track G1", Duration: 120},
		{Artist: "Bob", Album: "Gamma", Title: "Track G2", Duration: 60},
		{Artist: "Charlie", Album: "Delta", Title: "Track D1", Duration: 3661},
	}
}