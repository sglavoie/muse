package tui

// This file builds deterministic, fixture-backed Models for the hidden
// `muse tui-test` layout harness. Each fixture constructs a real tui.Model via
// New(context.Background(), ...) wired only to the in-memory fakes defined in
// uitest_fakes.go. configPath and dataDir are left empty so side-effectful
// palette commands (open config/database directory, save config) are absent or
// no-op for fixture runs.

import (
	"context"
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// Built-in fixture names.
const (
	FixtureEmpty    = "empty"
	FixtureRich     = "rich"
	FixtureNarrow   = "narrow"
	FixturePlayback = "playback"
	FixtureHealth   = "health"
)

// FixtureNames returns the built-in fixture names in a stable order.
func FixtureNames() []string {
	return []string{FixtureEmpty, FixtureRich, FixtureNarrow, FixturePlayback, FixtureHealth}
}

// BuildFixture constructs a fixture-backed Model by name. The returned model is
// not yet sized; the caller is expected to apply an initial tea.WindowSizeMsg
// before rendering. It never touches real config, databases, music files, mpv,
// Finder, or clipboard. An error is returned for an unknown fixture name.
func BuildFixture(name string) (Model, error) {
	var m Model
	switch name {
	case FixtureEmpty:
		m = New(context.Background(), baseFixtureOptions()...)
	case FixtureRich:
		m = newRichFixture()
	case FixtureNarrow:
		m = newNarrowFixture()
	case FixturePlayback:
		m = newPlaybackFixture()
	case FixtureHealth:
		m = newHealthFixture()
	default:
		return Model{}, fmt.Errorf("unknown fixture %q (known: %s)", name, strings.Join(FixtureNames(), ", "))
	}
	// Every fixture-backed model runs inside the harness, so key- and
	// text-driven command-palette selections must honor the same fixture-safe
	// allowlist that guards direct command actions.
	m.fixtureSafeCommands = true
	return m, nil
}

// baseFixtureOptions returns the dependencies shared by every fixture: a no-op
// player plus in-memory queue and dismissal stores. These keep playback and
// queue-mutation keys safe to exercise without any real side effects.
func baseFixtureOptions() []Option {
	queue := &uitestQueueStore{}
	dismissals := &uitestDismissals{hashes: map[string]struct{}{}}
	return []Option{
		WithPlayer(uitestPlayer{}),
		WithQueueWriter(queue),
		WithQueueDequeuer(queue),
		WithQueueReorderer(queue),
		WithBatchEnqueuer(queue),
		WithDismissalReader(dismissals),
		WithDismissalWriter(dismissals),
		// A reader with no tracks so the empty fixture exercises the Insights
		// empty state rather than the "no reader wired" fallback. Fixtures with
		// a library override this with a track-backed reader.
		WithInsightsReader(&uitestInsights{}),
	}
}

// newRichFixture builds a Model populated across every primary view: a small
// deterministic library, queue, playlists, favorites, tags, and recommendation
// data sufficient to render the Mixes view.
func newRichFixture() Model {
	tracks := richLibraryTracks()
	favorites := newUITestFavorites(map[string][]string{
		"artist": {"Alice", "Bob"},
		"album":  {"Alpha"},
		"track":  {tracks[0].Hash},
	})
	tags := richTags()
	mixTracks := richMixTracks()

	opts := append(baseFixtureOptions(),
		WithLibraryTracks(tracks),
		WithQueueEntries(richQueueEntries()),
		WithPlaylists(richPlaylists()),
		WithPlaylistTrackReader(&uitestPlaylistTracks{byID: richPlaylistTracks()}),
		WithFavoriteReader(favorites),
		WithFavoriteWriter(favorites),
		WithTagReader(tags),
		WithTagWriter(tags),
		WithRecommendationReader(&uitestRecs{
			sufficient: true,
			tracks:     mixTracks,
			progress:   map[db.MixID]db.MixProgress{},
		}),
		WithCacheReader(&uitestCache{tracks: mixTracks}),
		WithInsightsReader(&uitestInsights{tracks: tracks}),
	)
	return New(context.Background(), opts...)
}

// newNarrowFixture builds a Model whose data intentionally stresses truncation
// and width arithmetic: very long titles/artists/albums/paths and wide Unicode
// (CJK) text.
func newNarrowFixture() Model {
	tracks := narrowLibraryTracks()
	favorites := newUITestFavorites(map[string][]string{
		"artist": {"アーティスト・非常に長い名前でレイアウトを試験する"},
		"album":  {"とても長いアルバム名 — 幅計算のストレステスト"},
	})
	tags := narrowTags()

	opts := append(baseFixtureOptions(),
		WithLibraryTracks(tracks),
		WithQueueEntries(narrowQueueEntries()),
		WithPlaylists(narrowPlaylists()),
		WithFavoriteReader(favorites),
		WithFavoriteWriter(favorites),
		WithTagReader(tags),
		WithTagWriter(tags),
		WithInsightsReader(&uitestInsights{tracks: tracks}),
	)
	return New(context.Background(), opts...)
}

// newPlaybackFixture builds a Model with an active playback session: a
// populated queue, a current track, and repeat/shuffle/volume/status metadata.
func newPlaybackFixture() Model {
	entries := playbackQueueEntries()
	current := entries[1]
	tracks := playbackLibraryTracks()

	opts := append(baseFixtureOptions(),
		WithLibraryTracks(tracks),
		WithInsightsReader(&uitestInsights{tracks: tracks}),
		WithQueueEntries(entries),
		WithCurrentTrackHash(current.Hash),
		WithCurrentIndex(1),
		WithPlaybackState(playback.StatePlaying),
		WithRepeatMode(playback.RepeatAll),
		WithShuffle(true),
		WithVolume(65),
		WithStatusBarTrack(current.Artist, current.Title, "Queue"),
	)
	return New(context.Background(), opts...)
}

// newHealthFixture builds a Model whose library is deliberately dirty, so the
// Data Health panel has one of every finding to report and the genre merge has
// something to fold. It is a fixture of its own rather than a dirtied `rich`
// because `rich` is the baseline half the scenarios assert against.
func newHealthFixture() Model {
	tracks := healthLibraryTracks()
	reader := &uitestInsights{tracks: tracks}
	opts := append(baseFixtureOptions(),
		WithLibraryTracks(tracks),
		WithInsightsReader(reader),
		WithMetadataWriter(&uitestMetadata{ins: reader}),
	)
	return New(context.Background(), opts...)
}

// --- deterministic fixture data ---------------------------------------------

// richLibraryTracks is the shared library behind the rich fixture. Play counts
// and last-played offsets are set so Insights has something to measure: three
// heard tracks, three never played, and one heard so long ago it counts as
// dormant. Offsets are relative to the run so the time windows stay meaningful.
// The three unheard tracks are added in three different months so Discovery's
// backlog breaks down into more than one row.
func richLibraryTracks() []library.Track {
	ago := func(days int) int64 { return uitestNow().AddDate(0, 0, -days).Unix() }
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Genre: "Ambient", Duration: 183, Hash: "rh1", Path: "/music/alice/alpha/a1.flac", TrackNumber: 1, CreatedAt: ago(300), PlayCount: 12, LastPlayedAt: ago(2)},
		{Artist: "Alice", Album: "Alpha", Title: "Track A2", Genre: "Ambient", Duration: 245, Hash: "rh2", Path: "/music/alice/alpha/a2.flac", TrackNumber: 2, CreatedAt: ago(10)},
		{Artist: "Alice", Album: "Beta", Title: "Track B1", Genre: "Ambient", Duration: 300, Hash: "rh3", Path: "/music/alice/beta/b1.flac", TrackNumber: 1, CreatedAt: ago(70)},
		{Artist: "Bob", Album: "Gamma", Title: "Track G1", Genre: "Techno", Duration: 120, Hash: "rh4", Path: "/music/bob/gamma/g1.flac", TrackNumber: 1, CreatedAt: ago(300), PlayCount: 5, LastPlayedAt: ago(20)},
		{Artist: "Bob", Album: "Gamma", Title: "Track G2", Genre: "Techno", Duration: 60, Hash: "rh5", Path: "/music/bob/gamma/g2.flac", TrackNumber: 2, CreatedAt: ago(200)},
		{Artist: "Charlie", Album: "Delta", Title: "Track D1", Genre: "Jazz", Duration: 3661, Hash: "rh6", Path: "/music/charlie/delta/d1.flac", TrackNumber: 1, CreatedAt: ago(500), PlayCount: 2, LastPlayedAt: ago(400)},
	}
}

// healthLibraryTracks carries one instance of every Data Health finding: three
// spellings of Ambient, a placeholder genre, a blank title, artist, album and
// genre, two durations that were never checked against the file, and one artist
// spelled with and without its article. Play counts sit inside the Deep Cuts
// band so the completion filter reads as inert too.
func healthLibraryTracks() []library.Track {
	ago := func(days int) int64 { return uitestNow().AddDate(0, 0, -days).Unix() }
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Drift", Genre: "Ambient", Duration: 210, Hash: "hh1", Path: "/music/alice/alpha/drift.flac", TrackNumber: 1, CreatedAt: ago(300), PlayCount: 4, LastPlayedAt: ago(3)},
		{Artist: "The Alice", Album: "Alpha", Title: "Drift Reprise", Genre: "ambient", Duration: 190, Hash: "hh2", Path: "/music/alice/alpha/drift-reprise.flac", TrackNumber: 2, CreatedAt: ago(280), PlayCount: 3, LastPlayedAt: ago(6)},
		{Artist: "Bob", Album: "Gamma", Title: "Pulse", Genre: "AMBIENT ", Duration: 240, Hash: "hh3", Path: "/music/bob/gamma/pulse.flac", TrackNumber: 1, CreatedAt: ago(120), PlayCount: 2, LastPlayedAt: ago(12)},
		{Artist: "Bob", Album: "", Title: "Bounce", Genre: "Unknown", Duration: 0, Hash: "hh4", Path: "/music/bob/bounce.flac", TrackNumber: 2, CreatedAt: ago(40)},
		{Artist: "", Album: "Delta", Title: "Orphan", Genre: "", Duration: 150, Hash: "hh5", Path: "/music/unsorted/orphan.flac", TrackNumber: 1, CreatedAt: ago(90), PlayCount: 2, LastPlayedAt: ago(9)},
		{Artist: "Charlie", Album: "Delta", Title: "", Genre: "Jazz", Duration: 0, Hash: "hh6", Path: "/music/charlie/delta/06.flac", TrackNumber: 6, CreatedAt: ago(15)},
	}
}

func richMixTracks() []library.Track {
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Hash: "rh1", Path: "/music/alice/alpha/a1.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Track G1", Hash: "rh4", Path: "/music/bob/gamma/g1.flac"},
		{Artist: "Charlie", Album: "Delta", Title: "Track D1", Hash: "rh6", Path: "/music/charlie/delta/d1.flac"},
	}
}

func richQueueEntries() []QueueEntry {
	return []QueueEntry{
		{ID: 1, Hash: "rh1", Path: "/music/alice/alpha/a1.flac", Display: "Alice – Track A1", Artist: "Alice", Title: "Track A1", Album: "Alpha", DurationMs: 183000, Position: 100},
		{ID: 2, Hash: "rh4", Path: "/music/bob/gamma/g1.flac", Display: "Bob – Track G1", Artist: "Bob", Title: "Track G1", Album: "Gamma", DurationMs: 120000, Position: 200},
		{ID: 3, Hash: "rh6", Path: "/music/charlie/delta/d1.flac", Display: "Charlie – Track D1", Artist: "Charlie", Title: "Track D1", Album: "Delta", DurationMs: 3661000, Position: 300},
	}
}

func richPlaylists() []db.PlaylistRow {
	return []db.PlaylistRow{
		{ID: 1, Name: "Focus", TrackCount: 3},
		{ID: 2, Name: "Evening", TrackCount: 2},
	}
}

func richPlaylistTracks() map[int64][]db.PlaylistTrackRow {
	return map[int64][]db.PlaylistTrackRow{
		1: {
			{ID: 11, Position: 1, Title: "Track A1", Artist: "Alice", Album: "Alpha", DurationMs: 183000, Hash: "rh1", Path: "/music/alice/alpha/a1.flac"},
			{ID: 12, Position: 2, Title: "Track B1", Artist: "Alice", Album: "Beta", DurationMs: 300000, Hash: "rh3", Path: "/music/alice/beta/b1.flac"},
			{ID: 13, Position: 3, Title: "Track D1", Artist: "Charlie", Album: "Delta", DurationMs: 3661000, Hash: "rh6", Path: "/music/charlie/delta/d1.flac"},
		},
		2: {
			{ID: 21, Position: 1, Title: "Track G1", Artist: "Bob", Album: "Gamma", DurationMs: 120000, Hash: "rh4", Path: "/music/bob/gamma/g1.flac"},
			{ID: 22, Position: 2, Title: "Track G2", Artist: "Bob", Album: "Gamma", DurationMs: 60000, Hash: "rh5", Path: "/music/bob/gamma/g2.flac"},
		},
	}
}

func richTags() *uitestTags {
	return &uitestTags{
		tags: []db.TagRow{
			{ID: 1, Name: "chill", Color: "#88c0d0"},
			{ID: 2, Name: "focus", Color: "#a3be8c"},
			{ID: 3, Name: "energetic", Color: "#bf616a"},
		},
		counts: map[int64]int{1: 3, 2: 2, 3: 1},
		trackTags: map[string][]db.TagRow{
			"rh1": {{ID: 1, Name: "chill", Color: "#88c0d0"}},
			"rh4": {{ID: 3, Name: "energetic", Color: "#bf616a"}},
		},
		tracksByTag: map[int64][]db.PlaylistTrackRow{
			1: {{ID: 11, Position: 1, Title: "Track A1", Artist: "Alice", Album: "Alpha", Hash: "rh1", Path: "/music/alice/alpha/a1.flac"}},
		},
		hashesByTag: map[int64][]string{1: {"rh1", "rh2", "rh3"}, 2: {"rh1", "rh6"}, 3: {"rh4"}},
	}
}

// narrowLibraryTracks stresses truncation and width arithmetic. It also carries
// a listening history: the Insights view only reaches its degraded, chart-free
// layout when there is something to degrade, so a fixture with no plays would
// render the empty card at 60x15 and prove nothing.
func narrowLibraryTracks() []library.Track {
	ago := func(days int) int64 { return uitestNow().AddDate(0, 0, -days).Unix() }
	return []library.Track{
		{
			Artist:       narrowLongArtist,
			Album:        "An Album Title That Simply Refuses to End No Matter What",
			Title:        "A Song With a Preposterously Long Title Designed to Overflow Any Reasonable Column Width",
			Genre:        "Experimental / Post-Everything / Long-Form Ambient Drone",
			Duration:     5000,
			Hash:         "nh1",
			Path:         "/very/deeply/nested/music/collection/that/keeps/going/and/going/artist/album/track-one.flac",
			CreatedAt:    ago(240),
			PlayCount:    9,
			LastPlayedAt: ago(1),
		},
		{
			Artist:       "混雑した都市の夜を走る非常に長い名前のアーティスト",
			Album:        "幅の広い文字で構成された長いアルバム名のテスト用データ",
			Title:        "全角文字と絵文字🎵🎶を含む長いタイトルで表示幅の計算を検証する",
			Genre:        "シティ・ポップ",
			Duration:     321,
			Hash:         "nh2",
			Path:         "/音楽/日本語のディレクトリ名/とても長いパス/曲.flac",
			CreatedAt:    ago(150),
			PlayCount:    4,
			LastPlayedAt: ago(45),
		},
		{
			Artist:    "Émilie Ñoño Straße Åström",
			Album:     "Café Naïve — Diacritics Everywhere",
			Title:     "Combining Marks and Accented Ünïcödé for Width Testing",
			Genre:     "Chanson",
			Duration:  200,
			Hash:      "nh3",
			Path:      "/musique/émilie/café-naïve/piste-3.flac",
			CreatedAt: ago(30),
		},
	}
}

// narrowLongArtist is exactly 60 columns, the enforced minimum terminal width,
// so every view that shows an artist name has to truncate it somewhere.
const narrowLongArtist = "The Exceptionally Verbose Orchestra of Very Long Names & Co."

func narrowQueueEntries() []QueueEntry {
	return []QueueEntry{
		{ID: 1, Hash: "nh1", Path: "/very/deeply/nested/track-one.flac", Display: "The Exceptionally Verbose Orchestra – A Song With a Preposterously Long Title", Artist: narrowLongArtist, Title: "A Song With a Preposterously Long Title", DurationMs: 5000000, Position: 100},
		{ID: 2, Hash: "nh2", Path: "/音楽/曲.flac", Display: "混雑した都市の夜を走る非常に長い名前のアーティスト – 全角文字と絵文字🎵🎶を含む長いタイトル", Artist: "混雑した都市の夜を走る非常に長い名前のアーティスト", Title: "全角文字と絵文字🎵🎶を含む長いタイトル", DurationMs: 321000, Position: 200},
	}
}

func narrowPlaylists() []db.PlaylistRow {
	return []db.PlaylistRow{
		{ID: 1, Name: "A Playlist Name That Is Also Unreasonably Long For Layout Testing", TrackCount: 2},
		{ID: 2, Name: "日本語のプレイリスト名 — 幅計算テスト", TrackCount: 1},
	}
}

func narrowTags() *uitestTags {
	return &uitestTags{
		tags: []db.TagRow{
			{ID: 1, Name: "a-very-long-tag-name-for-width-testing", Color: "#88c0d0"},
			{ID: 2, Name: "全角タグ名テスト", Color: "#a3be8c"},
		},
		counts:      map[int64]int{1: 2, 2: 1},
		trackTags:   map[string][]db.TagRow{},
		tracksByTag: map[int64][]db.PlaylistTrackRow{},
		hashesByTag: map[int64][]string{},
	}
}

func playbackLibraryTracks() []library.Track {
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Duration: 183, Hash: "ph1", Path: "/music/alice/alpha/a1.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Track G1", Duration: 120, Hash: "ph2", Path: "/music/bob/gamma/g1.flac"},
		{Artist: "Charlie", Album: "Delta", Title: "Track D1", Duration: 240, Hash: "ph3", Path: "/music/charlie/delta/d1.flac"},
	}
}

func playbackQueueEntries() []QueueEntry {
	return []QueueEntry{
		{ID: 1, Hash: "ph1", Path: "/music/alice/alpha/a1.flac", Display: "Alice – Track A1", Artist: "Alice", Title: "Track A1", Album: "Alpha", DurationMs: 183000, Position: 100},
		{ID: 2, Hash: "ph2", Path: "/music/bob/gamma/g1.flac", Display: "Bob – Track G1", Artist: "Bob", Title: "Track G1", Album: "Gamma", DurationMs: 120000, Position: 200},
		{ID: 3, Hash: "ph3", Path: "/music/charlie/delta/d1.flac", Display: "Charlie – Track D1", Artist: "Charlie", Title: "Track D1", Album: "Delta", DurationMs: 240000, Position: 300},
	}
}
