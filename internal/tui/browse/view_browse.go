package browse

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
	Mode    BrowseMode
	Artists ArtistsView
	Albums  AlbumsView
	Genres  GenresView
}

func (v *BrowseView) SetSize(w, h int) {
	v.Artists.setSize(w, h)
	v.Albums.setSize(w, h)
	v.Genres.setSize(w, h)
}

func (v BrowseView) Render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	switch v.Mode {
	case BrowseModeAlbums:
		return v.Albums.render(s, favHashes, favArtists, favAlbums, nowPlayingHash, nowPlayingIcon)
	case BrowseModeGenres:
		return v.Genres.render(s, favHashes, favArtists, favAlbums, nowPlayingHash, nowPlayingIcon)
	default:
		return v.Artists.render(s, favHashes, favArtists, favAlbums, nowPlayingHash, nowPlayingIcon)
	}
}
