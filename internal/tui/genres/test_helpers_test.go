package genres

import "github.com/sglavoie/muse/internal/library"

func tracks(specs ...struct{ genre, artist, title string }) []library.Track {
	out := make([]library.Track, len(specs))
	for i, s := range specs {
		out[i] = library.Track{
			Hash:   s.title,
			Genre:  s.genre,
			Artist: s.artist,
			Title:  s.title,
		}
	}
	return out
}
