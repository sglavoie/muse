package tui

import "github.com/sglavoie/muse/internal/tui/styles"

// BrowseMode identifies the active sub-mode of the Browse view.
type BrowseMode uint8

const (
	BrowseModeArtists BrowseMode = iota
	BrowseModeAlbums
	BrowseModeGenres
)

// BrowseView consolidates Artists, Albums, and Genres into a single view.
// Each sub-view retains its own cursor, scroll, and filter state independently.
type BrowseView struct {
	mode    BrowseMode
	artists ArtistsView
	albums  AlbumsView
	genres  GenresView
}

func (v *BrowseView) setSize(w, h int) {
	v.artists.setSize(w, h)
	v.albums.setSize(w, h)
	v.genres.setSize(w, h)
}

func (v BrowseView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	switch v.mode {
	case BrowseModeAlbums:
		return v.albums.render(s, favHashes, favArtists, favAlbums, nowPlayingHash, nowPlayingIcon)
	case BrowseModeGenres:
		return v.genres.render(s, favHashes, favArtists, favAlbums, nowPlayingHash, nowPlayingIcon)
	default:
		return v.artists.render(s, favHashes, favArtists, favAlbums, nowPlayingHash, nowPlayingIcon)
	}
}
