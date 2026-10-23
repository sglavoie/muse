package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/albums"
	"github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/genres"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/tags"
)

// Compare the range to the actual rendered records, not to another copy of
// the renderer's window calculation. Bucket headings must not count as records.
func TestPositionRangesMatchRenderedRecords(t *testing.T) {
	var names []string
	var tracks []library.Track
	var entries []queue.Entry
	var lists []db.PlaylistRow
	var rows []db.PlaylistTrackRow
	var labels []db.TagRow
	for i := range 30 {
		name := fmt.Sprintf("Item %02d", i+1)
		names = append(names, name)
		tracks = append(tracks, library.Track{Hash: name, Title: name, Artist: "Artist", Album: "Album", TrackNumber: i + 1})
		entries = append(entries, queue.Entry{ID: int64(i + 1), Hash: name, Title: name, Artist: "Artist", Album: "Album"})
		lists = append(lists, db.PlaylistRow{ID: int64(i + 1), Name: name})
		rows = append(rows, db.PlaylistTrackRow{ID: int64(i + 1), Hash: name, Title: name, Artist: "Artist"})
		labels = append(labels, db.TagRow{ID: int64(i + 1), Name: name})
	}
	s := styles.NewStyles(true)
	renderers := map[string]func(int, int, int) string{
		"library": func(w, h, o int) string {
			return (libtui.Panel{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, OverrideTracks: tracks}).Render()
		},
		"bucketed library": func(w, h, o int) string {
			return (libtui.Panel{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, OverrideTracks: tracks, BucketLabel: func(t library.Track) string { return fmt.Sprintf("Group %d", t.TrackNumber/3) }}).Render()
		},
		"queue": func(w, h, o int) string {
			return (queue.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Entries: entries}).Render(s, -1, playback.StateStopped, nil, nil, nil)
		},
		"bucketed queue": func(w, h, o int) string {
			return (queue.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Entries: entries, SortState: sorting.SortState{Field: sorting.SortArtist}}).Render(s, -1, playback.StateStopped, nil, nil, nil)
		},
		"artists": func(w, h, o int) string {
			return (artists.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Artists: names}).Render()
		},
		"artist albums": func(w, h, o int) string {
			return (artists.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Level: artists.AlbumLevel, Albums: names}).Render()
		},
		"artist tracks": func(w, h, o int) string {
			return (artists.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Level: artists.TrackLevel, Tracks: tracks}).Render()
		},
		"albums": func(w, h, o int) string {
			return (albums.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Albums: names}).Render()
		},
		"album tracks": func(w, h, o int) string {
			return (albums.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Level: albums.TrackLevel, Tracks: tracks}).Render()
		},
		"genres": func(w, h, o int) string {
			return (genres.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Genres: names}).Render()
		},
		"genre artists": func(w, h, o int) string {
			return (genres.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Level: genres.ArtistLevel, Artists: names}).Render()
		},
		"genre tracks": func(w, h, o int) string {
			return (genres.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Level: genres.TrackLevel, Tracks: tracks}).Render()
		},
		"favorite artists": func(w, h, o int) string {
			return (favorites.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Level: favorites.ListLevel, Artists: names}).Render()
		},
		"favorite albums": func(w, h, o int) string {
			return (favorites.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Level: favorites.ListLevel, Category: favorites.CategoryAlbums, Albums: names}).Render()
		},
		"favorite tracks": func(w, h, o int) string {
			return (favorites.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Level: favorites.ListLevel, Category: favorites.CategoryTracks, Tracks: tracks}).Render()
		},
		"favorite drilldown": func(w, h, o int) string {
			return (favorites.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Level: favorites.TrackLevel, DrilldownName: "Collection", DrilldownTracks: tracks}).Render()
		},
		"tags": func(w, h, o int) string {
			return (tags.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Tags: labels}).Render()
		},
		"tag tracks": func(w, h, o int) string {
			return (tags.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Level: tags.TrackLevel, Tracks: rows}).Render()
		},
		"playlists": func(w, h, o int) string {
			return (playlists.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Playlists: lists}).Render()
		},
		"playlist tracks": func(w, h, o int) string {
			return (playlists.View{Width: w, Height: h, Cursor: o, ScrollOffset: o, Styles: s, Level: playlists.TrackLevel, Tracks: rows}).Render()
		},
	}
	rangeRE := regexp.MustCompile(`(\d+)–(\d+) of 30`)
	itemRE := regexp.MustCompile(`Item (\d+)`)
	for name, render := range renderers {
		t.Run(name, func(t *testing.T) {
			for _, size := range [][2]int{{60, 9}, {100, 18}} {
				for _, offset := range []int{0, 10, 29} {
					out := ansi.Strip(render(size[0], size[1], offset))
					if !strings.Contains(out, "▎") {
						t.Fatalf("focused row is hidden:\n%s", out)
					}
					position := rangeRE.FindStringSubmatch(out)
					items := itemRE.FindAllStringSubmatch(out, -1)
					if len(position) != 3 || len(items) == 0 {
						t.Fatalf("missing range or records:\n%s", out)
					}
					first, _ := strconv.Atoi(position[1])
					last, _ := strconv.Atoi(position[2])
					actualFirst, _ := strconv.Atoi(items[0][1])
					actualLast, _ := strconv.Atoi(items[len(items)-1][1])
					if first != actualFirst || last != actualLast || last-first+1 != len(items) {
						t.Fatalf("range disagrees with records:\n%s", out)
					}
					for _, line := range strings.Split(out, "\n") {
						if ansi.StringWidth(line) > size[0] {
							t.Fatal(out)
						}
					}
				}
			}
		})
	}
}
