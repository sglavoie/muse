package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// newDenseFixture exercises crowded states together: long multilingual names,
// scrolling collections, repeated queue entries, hidden selections, playback,
// and a long notification. All data and mutations stay inside the fixture.
func newDenseFixture() Model {
	tracks := denseLibraryTracks()
	entries := denseQueueEntries(tracks)
	playlists, playlistTracks, tags := denseCollections(tracks)
	favorites := newUITestFavorites(map[string][]string{
		"artist": {tracks[0].Artist, tracks[10].Artist},
		"album":  {library.AlbumKey(tracks[0])},
		"track":  {tracks[0].Hash, tracks[1].Hash},
	})
	current := entries[1]
	opts := append(baseFixtureOptions(),
		WithLibraryTracks(tracks), WithQueueEntries(entries),
		WithPlaylists(playlists), WithPlaylistTrackReader(&uitestPlaylistTracks{byID: playlistTracks}),
		WithTagReader(tags), WithTagWriter(tags),
		WithFavoriteReader(favorites), WithFavoriteWriter(favorites),
		WithInsightsReader(&uitestInsights{tracks: tracks}),
		WithCurrentTrackHash(current.Hash), WithCurrentIndex(1),
		WithPlaybackState(playback.StatePlaying), WithRepeatMode(playback.RepeatAll),
		WithShuffle(true), WithVolume(65),
		WithStatusBarTrack(current.Artist, current.Title, current.SourcePlaylist),
	)
	m := New(context.Background(), opts...)
	m.playback.elapsedSecs = 83
	m.playback.totalSecs = float64(current.DurationMs) / 1000
	m.statusBar.Album = current.Album
	m.library.SelectedHashes = map[string]struct{}{tracks[0].Hash: {}, tracks[1].Hash: {}}
	// Keep the notice stable throughout interactive review and scenario waits.
	m.statusBar.PushMsg("Added 24 tracks from Evening journeys — selections outside this filter are included", time.Hour, statusbar.NotifNormal)
	return m
}

func denseLibraryTracks() []library.Track {
	artists := []string{"Glass Harbor and the Northern Lights Ensemble", "Émilie Ñoño Straße Åström", "混雑した都市の夜を走るアーティスト", "The Paper Satellites"}
	titles := []string{"Night Signals Beyond the Last Station — Live at the Observatory", "Coast Road Through the Sleeping City — Extended Version", "夜明けの街を歩く🎵 — A Journey Home", "Paper Lanterns in the Rain — Piano and Strings"}
	tracks := make([]library.Track, 240)
	for i := range tracks {
		tracks[i] = library.Track{
			Hash: fmt.Sprintf("dense-%03d", i), Path: fmt.Sprintf("/fixture/music/release-%02d/track-%03d.flac", i/10, i),
			Title:  fmt.Sprintf("%s %03d", titles[i%len(titles)], i),
			Artist: artists[(i/10)%len(artists)], AlbumArtist: artists[(i/10)%len(artists)],
			Album: fmt.Sprintf("Journeys After Midnight — Expanded Edition %02d", i/10),
			Genre: "Ambient", TrackNumber: i%10 + 1, DiscNumber: i%2 + 1,
			Duration: float64(180 + i*3), CreatedAt: 1700000000 + int64(i*86400),
		}
	}
	return tracks
}

func denseQueueEntries(tracks []library.Track) []queue.Entry {
	entries := make([]queue.Entry, 48)
	for i := range entries {
		t := tracks[i%24]
		entries[i] = queue.Entry{
			ID: int64(i + 1), Position: int64(i+1) * 100, Hash: t.Hash, Path: t.Path,
			Title: t.Title, Artist: t.Artist, Album: t.Album,
			Display: t.Artist + " — " + t.Title, DurationMs: int64(t.Duration * 1000),
			SourcePlaylist: "Evening journeys — long routes home", Selected: i == 0 || i == 25,
		}
	}
	return entries
}

func denseCollections(tracks []library.Track) ([]db.PlaylistRow, map[int64][]db.PlaylistTrackRow, *uitestTags) {
	playlists := make([]db.PlaylistRow, 24)
	rows := make(map[int64][]db.PlaylistTrackRow)
	tags := &uitestTags{counts: map[int64]int{}, trackTags: map[string][]db.TagRow{}, tracksByTag: map[int64][]db.PlaylistTrackRow{}, hashesByTag: map[int64][]string{}}
	for i := range playlists {
		id := int64(i + 1)
		playlists[i] = db.PlaylistRow{ID: id, Name: fmt.Sprintf("Evening journeys %02d — music for the long route home", i), TrackCount: 10}
		tag := db.TagRow{ID: id, Name: fmt.Sprintf("Late-night listening %02d — 静かな夜", i), Color: "#88c0d0"}
		tags.tags = append(tags.tags, tag)
		for j, t := range tracks[i*10 : (i+1)*10] {
			row := db.PlaylistTrackRow{ID: int64(i*10 + j + 1), Position: int64(j + 1), Hash: t.Hash, Path: t.Path, Title: t.Title, Artist: t.Artist, Album: t.Album, AlbumArtist: t.AlbumArtist, Genre: t.Genre, DurationMs: int64(t.Duration * 1000)}
			rows[id] = append(rows[id], row)
			tags.tracksByTag[id] = append(tags.tracksByTag[id], row)
			tags.hashesByTag[id] = append(tags.hashesByTag[id], t.Hash)
			tags.trackTags[t.Hash] = []db.TagRow{tag}
		}
		tags.counts[id] = len(rows[id])
	}
	return playlists, rows, tags
}
