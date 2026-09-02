package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recommendation"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// MixInfo is a type alias for recommendation.MixInfo, kept for backward
// compatibility so callers using tui.MixInfo continue to compile unchanged.
type MixInfo = recommendation.MixInfo

// MixesView manages the two-level Mixes panel: a list of recommendation mixes
// at level 0, and the track list for the selected mix at level 1.
type MixesView struct {
	mixes             []MixInfo
	selectedMix       int
	level             int // 0=list, 1=tracks
	activeMix         db.MixID
	trackView         libtui.View // reuses sort/filter/select/enqueue logic
	cachedMixes       map[db.MixID][]library.Track
	playsSinceRefresh int
	insufficientData  bool
	width, height     int
}

func (v *MixesView) SetSize(w, h int) { v.width = w; v.height = h }

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
	track, ok := v.trackView.Panel().FocusedTrack()
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
	v.trackView = libtui.View{
		SourceTracks: tracks,
		Fuzzy:        library.SahilmFuzzySearcher{},
		Width:        v.width,
		Height:       v.height,
	}
}

// back returns from the track list to the mix list.
func (v *MixesView) back() {
	v.level = 0
	v.trackView = libtui.View{}
}

// handleCursorAction moves the cursor at list level 0 or delegates to the
// embedded trackView at level 1.
func (v MixesView) handleCursorAction(a keys.Action) MixesView {
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
	case keys.ActionCursorUp:
		v.selectedMix--
	case keys.ActionCursorDown:
		v.selectedMix++
	case keys.ActionHalfPageUp:
		v.selectedMix -= half
	case keys.ActionHalfPageDown:
		v.selectedMix += half
	case keys.ActionFullPageUp:
		v.selectedMix -= v.height
	case keys.ActionFullPageDown:
		v.selectedMix += v.height
	case keys.ActionGoToTop:
		v.selectedMix = 0
	case keys.ActionGoToBottom:
		v.selectedMix = n - 1
	}
	v.clampCursor()
	return v
}

// render returns a string representation of the Mixes view.
// At level 0, it shows the list of mixes with name, description, and count.
// At level 1, it delegates to the embedded trackView.
func (v MixesView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	if v.level == 1 {
		if len(v.trackView.SourceTracks) == 0 {
			for _, mix := range v.mixes {
				if mix.ID == v.activeMix && mix.Progress != "" {
					return s.Dim.Render(mix.Progress)
				}
			}
			return s.Dim.Render("No tracks match yet — keep listening!")
		}
		return v.trackView.Render(s, favHashes, favArtists, favAlbums, nowPlayingHash, nowPlayingIcon)
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
		return s.RenderEmptyCard(v.width, v.height, "No mixes yet", []styles.EmptyAction{
			{Key: "1", Label: "go to library"},
			{Key: "?", Label: "help"},
		})
	}

	var lines []string
	for i, mix := range v.mixes {
		focused := i == v.selectedMix
		cursor := "  "
		if focused {
			cursor = "> "
		}

		row := renderMixListRow(cursor, mix, v.width)

		if focused {
			lines = append(lines, s.Accent.Render(row))
		} else {
			isOdd := i%2 == 1
			name, rest := splitMixListRow(cursor, mix, v.width)
			if isOdd {
				zb := lipgloss.NewStyle().Background(s.ZebraBg)
				lines = append(lines, zb.Render(cursor)+s.TextPrimary.Bold(true).Background(s.ZebraBg).Render(name)+s.TextTertiary.Background(s.ZebraBg).Render(rest))
			} else {
				lines = append(lines, cursor+s.TextPrimary.Bold(true).Render(name)+s.TextTertiary.Render(rest))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func renderMixListRow(cursor string, mix MixInfo, width int) string {
	name, rest := splitMixListRow(cursor, mix, width)
	if width <= 0 {
		return cursor + name + rest
	}
	return text.Truncate(cursor+name+rest, width)
}

func splitMixListRow(cursor string, mix MixInfo, width int) (name, rest string) {
	if width <= 0 {
		count := fmt.Sprintf("%d tracks", mix.TrackCount)
		return fmt.Sprintf("%-14s", mix.Name), fmt.Sprintf("  %-10s  %s", count, mix.Description)
	}

	cursorW := lipgloss.Width(cursor)
	gapCount := 2
	nameW := 14
	countW := 10
	available := width - cursorW - gapCount*2
	if available <= 0 {
		return "", ""
	}
	if available < nameW+countW {
		deficit := nameW + countW - available
		shrink := min(deficit, countW)
		countW -= shrink
		deficit -= shrink
		shrink = min(deficit, nameW)
		nameW -= shrink
	}

	descW := max(width-cursorW-nameW-gapCount-countW-gapCount, 0)
	count := fmt.Sprintf("%d tracks", mix.TrackCount)
	name = text.Pad(mix.Name, nameW)
	rest = strings.Repeat(" ", gapCount) + text.Pad(count, countW)
	if descW > 0 {
		rest += strings.Repeat(" ", gapCount) + text.Pad(mix.Description, descW)
	}
	return name, rest
}
