package tui

import (
	"context"
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/artwork"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// The Browse Albums cover grid keeps its own small thumbnails rather than
// the full covers in artworkState.cache: a screen of tiles holds dozens of
// covers at once, far more than the decoded-cover cache is sized for.
const (
	// gridThumbSide is the thumbnail edge in pixels: enough for the largest
	// tile as a Kitty image (16×8 cells) without holding full covers.
	gridThumbSide = 144
	// gridThumbEntries bounds the thumbnails kept in memory (about 80 KB
	// each). Visible tiles are never evicted, whatever this says.
	gridThumbEntries = 256
	// gridMaxInflight caps the cover lookups running at once, so a page of
	// albums that were never looked at does not start dozens of extractors.
	gridMaxInflight = 4
)

// albumGridArt is shared by every copy of the Model, like artworkState.
type albumGridArt struct {
	thumbs  map[string]*image.RGBA // art key → thumbnail; nil value means "no art"
	order   []string               // insertion order, for eviction
	pending map[string]bool
	// blocks caches half-block drawings per slot so a frame never resamples
	// a thumbnail; it is filled during Update, never from View.
	blocks map[kittySlot]string
}

// gridArtLoadedMsg delivers a finished tile lookup; thumb is nil when the
// album has no cover.
type gridArtLoadedMsg struct {
	key   string
	thumb *image.RGBA
}

// withAlbumGrid enables the cover grid when art is on. It runs once in New,
// after the options have decided whether there is art.
func (m Model) withAlbumGrid() Model {
	m.browse.Albums.Artwork = m.art != nil
	if m.art == nil {
		m.albumGrid = nil
		return m
	}
	m.albumGrid = &albumGridArt{
		thumbs:  map[string]*image.RGBA{},
		pending: map[string]bool{},
		blocks:  map[kittySlot]string{},
	}
	return m
}

// albumGridActive reports whether Browse is showing the albums cover grid.
func (m Model) albumGridActive() bool {
	return m.ActiveView == viewBrowse && m.browse.Mode == browse.BrowseModeAlbums && m.browse.Albums.GridActive()
}

// visibleGridSlots returns the art key and Kitty slot of every tile on
// screen, or nothing when the grid is hidden.
func (m Model) visibleGridSlots() []kittySlot {
	if m.albumGrid == nil || m.hasActiveOverlay() || !m.albumGridActive() {
		return nil
	}
	albums, g, _ := m.browse.Albums.VisibleTiles()
	slots := make([]kittySlot, 0, len(albums))
	for _, album := range albums {
		if rel, ok := m.browse.Albums.Releases[album]; ok {
			_, key := artRequestFor(rel.Track)
			slots = append(slots, kittySlot{key: key, cols: g.ArtCols, rows: g.ArtRows})
		}
	}
	return slots
}

// syncAlbumGrid runs after every update: it starts lookups for the visible
// tiles that have none yet, prepares their drawings, and uploads Kitty
// images the next frame shows. Tiles off screen are never looked up.
func (m Model) syncAlbumGrid() tea.Cmd {
	slots := m.visibleGridSlots()
	if len(slots) == 0 {
		return nil
	}
	cmds := m.requestGridThumbs()
	m.prepareGridBlocks(slots)
	cmds = append(cmds, m.placeAlbumGrid(slots))
	return batchNonNil(cmds...)
}

// requestGridThumbs starts lookups for visible tiles, at most
// gridMaxInflight at a time. Instant loaders answer synchronously.
func (m Model) requestGridThumbs() []tea.Cmd {
	ga := m.albumGrid
	albums, _, _ := m.browse.Albums.VisibleTiles()
	ctx := m.appCtx
	if ctx == nil {
		ctx = context.Background()
	}
	var cmds []tea.Cmd
	for _, album := range albums {
		rel, ok := m.browse.Albums.Releases[album]
		if !ok {
			continue
		}
		req, key := artRequestFor(rel.Track)
		if _, done := ga.thumbs[key]; done || ga.pending[key] {
			continue
		}
		loader := m.art.loader
		if m.art.instant {
			m.handleGridArtLoaded(loadGridThumb(ctx, loader, req, key))
			continue
		}
		if len(ga.pending) >= gridMaxInflight {
			break
		}
		ga.pending[key] = true
		cmds = append(cmds, func() tea.Msg { return loadGridThumb(ctx, loader, req, key) })
	}
	return cmds
}

func loadGridThumb(ctx context.Context, loader artwork.Loader, req artwork.Request, key string) gridArtLoadedMsg {
	art, err := loader.Load(ctx, req)
	if err != nil || art == nil {
		return gridArtLoadedMsg{key: key}
	}
	return gridArtLoadedMsg{key: key, thumb: artwork.Resize(art.Image, gridThumbSide, gridThumbSide)}
}

// handleGridArtLoaded stores a thumbnail and evicts the oldest ones that
// are not on screen once more than gridThumbEntries are held.
func (m Model) handleGridArtLoaded(msg gridArtLoadedMsg) Model {
	ga := m.albumGrid
	if ga == nil {
		return m
	}
	delete(ga.pending, msg.key)
	if _, exists := ga.thumbs[msg.key]; !exists {
		ga.order = append(ga.order, msg.key)
	}
	ga.thumbs[msg.key] = msg.thumb
	if len(ga.order) <= gridThumbEntries {
		return m
	}
	visible := map[string]bool{}
	for _, slot := range m.visibleGridSlots() {
		visible[slot.key] = true
	}
	kept := ga.order[:0]
	excess := len(ga.order) - gridThumbEntries
	for _, key := range ga.order {
		if excess > 0 && !visible[key] && key != msg.key {
			ga.evict(key)
			excess--
			continue
		}
		kept = append(kept, key)
	}
	ga.order = kept
	return m
}

func (ga *albumGridArt) evict(key string) {
	delete(ga.thumbs, key)
	for slot := range ga.blocks {
		if slot.key == key {
			delete(ga.blocks, slot)
		}
	}
}

// prepareGridBlocks draws the half-block covers the next frame shows.
// Kitty mode needs them too, for the frame before an upload lands.
func (m Model) prepareGridBlocks(slots []kittySlot) {
	ga := m.albumGrid
	for _, slot := range slots {
		if _, ok := ga.blocks[slot]; ok {
			continue
		}
		if thumb := ga.thumbs[slot.key]; thumb != nil {
			ga.blocks[slot] = artwork.Blocks(thumb, slot.cols, slot.rows)
		}
	}
}

// placeAlbumGrid uploads the visible tiles' covers the terminal does not
// hold yet. Uploaded images stay put while their ids last, so scrolling back
// to a page draws it from the terminal's copy without uploading again. An id
// is never recycled while any slot on screen uses it.
func (m Model) placeAlbumGrid(slots []kittySlot) tea.Cmd {
	if m.art.mode != artKitty {
		return nil
	}
	k := m.art.kitty
	live := map[int]bool{}
	for _, slot := range append(m.artSlots(), slots...) {
		if id, ok := k.ids[slot]; ok {
			live[id] = true
		}
	}
	var seqs []string
	for _, slot := range slots {
		thumb := m.albumGrid.thumbs[slot.key]
		if _, ok := k.ids[slot]; ok || thumb == nil {
			continue
		}
		id, ok := k.allocateAvoiding(slot, live)
		if !ok {
			break
		}
		live[id] = true
		seq, err := artwork.KittyTransmit(id, thumb, slot.cols, slot.rows)
		if err != nil {
			delete(k.ids, slot)
			delete(k.byID, id)
			continue
		}
		seqs = append(seqs, seq)
	}
	if len(seqs) == 0 {
		return nil
	}
	return tea.Raw(strings.Join(seqs, ""))
}

// allocateAvoiding is allocate, skipping ids that a slot on screen uses. It
// reports false when every id is live.
func (k *kittyPlacements) allocateAvoiding(slot kittySlot, live map[int]bool) (int, bool) {
	const first = 16
	for range artwork.MaxKittyID - first + 1 {
		if k.nextID < first || k.nextID > artwork.MaxKittyID {
			k.nextID = first
		}
		if !live[k.nextID] {
			return k.allocate(slot), true
		}
		k.nextID++
	}
	return 0, false
}

// albumTileCover draws album's cover for its grid tile, or returns "" while
// the cover loads or when there is none. It only reads state Update
// prepared, so drawing a frame never decodes or resamples an image.
func (m Model) albumTileCover(album string, cols, rows int) string {
	ga := m.albumGrid
	if ga == nil {
		return ""
	}
	rel, ok := m.browse.Albums.Releases[album]
	if !ok {
		return ""
	}
	_, key := artRequestFor(rel.Track)
	thumb := ga.thumbs[key]
	if thumb == nil {
		return ""
	}
	slot := kittySlot{key: key, cols: cols, rows: rows}
	if m.art.mode == artKitty {
		if id, ok := m.art.kitty.ids[slot]; ok {
			return artwork.KittyPlaceholders(id, cols, rows)
		}
	}
	if s, ok := ga.blocks[slot]; ok {
		return s
	}
	return artwork.Blocks(thumb, cols, rows)
}

// handleAlbumGridAction handles the grid's sideways moves and the grid/list
// toggle. Sideways moves only reach here while the grid shows: resolveKey
// gives h/l and the arrows their global meaning otherwise.
func (m Model) handleAlbumGridAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	switch action {
	case keys.ActionCursorLeft, keys.ActionCursorRight:
		if m.albumGridActive() {
			m.browse.Albums = m.browse.Albums.HandleAction(action)
		}
		return m, nil, true
	case keys.ActionAlbumLayoutToggle:
		m.browse.Albums = m.browse.Albums.ToggleLayout()
		return m, notify(m.albumLayoutNotice()), true
	}
	return m, nil, false
}

// albumLayoutNotice says what the toggle switched to, and why the grid
// does not show when it cannot.
func (m Model) albumLayoutNotice() string {
	switch {
	case m.browse.Albums.Layout == albtui.LayoutList:
		return "Albums: list"
	case m.art == nil:
		return "Albums: cover grid needs artwork (Settings → Display)"
	case m.browse.Albums.Level == albtui.AlbumLevel && !m.browse.Albums.GridActive():
		return "Albums: cover grid needs a larger window"
	default:
		return "Albums: cover grid"
	}
}

func notify(text string) tea.Cmd {
	return func() tea.Msg { return statusbar.TransientNotifyMsg(text) }
}
