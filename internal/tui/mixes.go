package tui

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recommendation"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// MixInfo is a type alias for recommendation.MixInfo, kept for backward
// compatibility so callers using tui.MixInfo continue to compile unchanged.
type MixInfo = recommendation.MixInfo

// mixDefs defines the fixed set of recommendation mixes in display order.
var mixDefs = []MixInfo{
	{ID: db.MixRediscover, Name: "Rediscover", Description: "Tracks you loved but haven't played in 90+ days"},
	{ID: db.MixHiddenGems, Name: "Hidden Gems", Description: "Unplayed tracks by your favorite artists"},
	{ID: db.MixOnRepeat, Name: "On Repeat", Description: "Your most played tracks this week"},
	{ID: db.MixDeepCuts, Name: "Deep Cuts", Description: "Tracks you always listen to completion"},
	{ID: db.MixNeverPlayed, Name: "Never Played", Description: "Tracks you might like but haven't tried"},
}

// MixesView manages the two-level Mixes panel: a list of recommendation mixes
// at level 0, and the track list for the selected mix at level 1.
type MixesView struct {
	mixes             []MixInfo
	selectedMix       int
	level             int // 0=list, 1=tracks
	activeMix         db.MixID
	trackView         LibraryView // reuses sort/filter/select/enqueue logic
	cachedMixes       map[db.MixID][]library.Track
	playsSinceRefresh int
	insufficientData  bool
	width, height     int
}

func (v *MixesView) setSize(w, h int) { v.width = w; v.height = h }

// clampCursor ensures the cursor is within the valid range for the current level.
func (v *MixesView) clampCursor() {
	if v.level == 0 {
		n := len(v.mixes)
		if n == 0 {
			v.selectedMix = 0
			return
		}
		if v.selectedMix < 0 {
			v.selectedMix = 0
		} else if v.selectedMix >= n {
			v.selectedMix = n - 1
		}
	}
}

// focusedMix returns a pointer to the currently focused MixInfo, or nil if the
// list is empty or the view is at track level.
func (v *MixesView) focusedMix() *MixInfo {
	if v.level != 0 || len(v.mixes) == 0 {
		return nil
	}
	return &v.mixes[v.selectedMix]
}

// focusedTrack returns the focused track at level 1, or nil when not at track level.
// It delegates to the embedded trackView which accounts for filter and sort state.
func (v *MixesView) focusedTrack() *library.Track {
	if v.level != 1 {
		return nil
	}
	track, ok := v.trackView.view().FocusedTrack()
	if !ok {
		return nil
	}
	return &track
}

// openMix transitions to level 1 and loads cached tracks for the given mix.
// The caller is responsible for populating cachedMixes before calling openMix.
func (v *MixesView) openMix(id db.MixID) {
	v.level = 1
	v.activeMix = id
	tracks := v.cachedMixes[id]
	v.trackView = LibraryView{
		sourceTracks: tracks,
		fuzzy:        library.SahilmFuzzySearcher{},
		width:        v.width,
		height:       v.height,
	}
}

// back returns from the track list to the mix list.
func (v *MixesView) back() {
	v.level = 0
	v.trackView = LibraryView{}
}

// handleCursorAction moves the cursor at list level 0 or delegates to the
// embedded trackView at level 1.
func (v MixesView) handleCursorAction(a action) MixesView {
	if v.level == 1 {
		v.trackView = v.trackView.HandleAction(a)
		return v
	}
	n := len(v.mixes)
	if n == 0 {
		return v
	}
	half := max(v.height/2, 1)
	switch a {
	case actionCursorUp:
		v.selectedMix--
	case actionCursorDown:
		v.selectedMix++
	case actionHalfPageUp:
		v.selectedMix -= half
	case actionHalfPageDown:
		v.selectedMix += half
	case actionFullPageUp:
		v.selectedMix -= v.height
	case actionFullPageDown:
		v.selectedMix += v.height
	case actionGoToTop:
		v.selectedMix = 0
	case actionGoToBottom:
		v.selectedMix = n - 1
	}
	v.clampCursor()
	return v
}

// render returns a string representation of the Mixes view.
// At level 0, it shows the list of mixes with name, description, and count.
// At level 1, it delegates to the embedded trackView.
func (v MixesView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}) string {
	if v.level == 1 {
		if len(v.trackView.sourceTracks) == 0 {
			for _, mix := range v.mixes {
				if mix.ID == v.activeMix && mix.Progress != "" {
					return s.Dim.Render(mix.Progress)
				}
			}
			return s.Dim.Render("No tracks match yet — keep listening!")
		}
		return v.trackView.render(s, favHashes, favArtists, favAlbums)
	}

	if v.insufficientData {
		if len(v.mixes) > 0 && v.mixes[0].Progress != "" {
			var sb strings.Builder
			sb.WriteString("Personalized Mixes\n\n")
			for _, mix := range v.mixes {
				fmt.Fprintf(&sb, "  %-14s  %s\n", mix.Name, mix.Progress)
			}
			return s.Dim.Render(strings.TrimRight(sb.String(), "\n"))
		}
		msg := "Personalized Mixes\n\nPlay more tracks to unlock personalized mixes.\nMixes are generated from your listening history\n(minimum 20 plays)."
		return s.Dim.Render(msg)
	}

	if len(v.mixes) == 0 {
		return s.Dim.Render("Loading mixes…")
	}

	var lines []string
	for i, mix := range v.mixes {
		focused := i == v.selectedMix
		cursor := "  "
		if focused {
			cursor = "> "
		}

		count := fmt.Sprintf("%d tracks", mix.TrackCount)
		row := fmt.Sprintf("%s%-14s  %-10s  %s", cursor, mix.Name, count, mix.Description)

		if focused {
			lines = append(lines, s.Accent.Render(row))
		} else {
			name := mix.Name
			rest := strings.TrimPrefix(row, cursor+name)
			lines = append(lines, cursor+s.Bold.Render(name)+s.Dim.Render(rest))
		}
	}
	return strings.Join(lines, "\n")
}
