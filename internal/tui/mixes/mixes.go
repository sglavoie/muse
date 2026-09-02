package mixes

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

// MixesView manages the two-level Mixes panel: a list of recommendation mixes
// at level 0, and the track list for the selected mix at level 1.
type MixesView struct {
	Mixes             []recommendation.MixInfo
	SelectedMix       int
	Level             int // 0=list, 1=tracks
	ActiveMix         db.MixID
	TrackView         libtui.View // reuses sort/filter/select/enqueue logic
	CachedMixes       map[db.MixID][]library.Track
	PlaysSinceRefresh int
	InsufficientData  bool
	width, Height     int
}

func (v *MixesView) SetSize(w, h int) { v.width = w; v.Height = h }

// clampCursor ensures the cursor is within the valid range for the current level.
func (v *MixesView) clampCursor() {
	if v.Level == 0 {
		n := len(v.Mixes)
		if n == 0 {
			v.SelectedMix = 0
			return
		}
		if v.SelectedMix < 0 {
			v.SelectedMix = 0
		} else if v.SelectedMix >= n {
			v.SelectedMix = n - 1
		}
	}
}

// FocusedMix returns a pointer to the currently focused mix, or nil if the
// list is empty or the view is at track level.
func (v *MixesView) FocusedMix() *recommendation.MixInfo {
	if v.Level != 0 || len(v.Mixes) == 0 {
		return nil
	}
	return &v.Mixes[v.SelectedMix]
}

// FocusedTrack returns the focused track at level 1, or nil when not at track level.
// It delegates to the embedded trackView which accounts for filter and sort state.
func (v *MixesView) FocusedTrack() *library.Track {
	if v.Level != 1 {
		return nil
	}
	track, ok := v.TrackView.Panel().FocusedTrack()
	if !ok {
		return nil
	}
	return &track
}

// OpenMix transitions to level 1 and loads cached tracks for the given mix.
// The caller is responsible for populating cachedMixes before calling OpenMix.
func (v *MixesView) OpenMix(id db.MixID) {
	v.Level = 1
	v.ActiveMix = id
	tracks := v.CachedMixes[id]
	v.TrackView = libtui.View{
		SourceTracks: tracks,
		Fuzzy:        library.SahilmFuzzySearcher{},
		Width:        v.width,
		Height:       v.Height,
	}
}

// Back returns from the track list to the mix list.
func (v *MixesView) Back() {
	v.Level = 0
	v.TrackView = libtui.View{}
}

// HandleCursorAction moves the cursor at list level 0 or delegates to the
// embedded trackView at level 1.
func (v MixesView) HandleCursorAction(a keys.Action) MixesView {
	if v.Level == 1 {
		v.TrackView = v.TrackView.HandleAction(a)
		return v
	}
	n := len(v.Mixes)
	if n == 0 {
		return v
	}
	half := max(v.Height/2, 1)
	switch a {
	case keys.ActionCursorUp:
		v.SelectedMix--
	case keys.ActionCursorDown:
		v.SelectedMix++
	case keys.ActionHalfPageUp:
		v.SelectedMix -= half
	case keys.ActionHalfPageDown:
		v.SelectedMix += half
	case keys.ActionFullPageUp:
		v.SelectedMix -= v.Height
	case keys.ActionFullPageDown:
		v.SelectedMix += v.Height
	case keys.ActionGoToTop:
		v.SelectedMix = 0
	case keys.ActionGoToBottom:
		v.SelectedMix = n - 1
	}
	v.clampCursor()
	return v
}

// Render returns a string representation of the Mixes view.
// At level 0, it shows the list of mixes with name, description, and count.
// At level 1, it delegates to the embedded trackView.
func (v MixesView) Render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	if v.Level == 1 {
		if len(v.TrackView.SourceTracks) == 0 {
			for _, mix := range v.Mixes {
				if mix.ID == v.ActiveMix && mix.Progress != "" {
					return s.Dim.Render(mix.Progress)
				}
			}
			return s.Dim.Render("No tracks match yet — keep listening!")
		}
		return v.TrackView.Render(s, favHashes, favArtists, favAlbums, nowPlayingHash, nowPlayingIcon)
	}

	if v.InsufficientData {
		if len(v.Mixes) > 0 && v.Mixes[0].Progress != "" {
			var sb strings.Builder
			sb.WriteString("Personalized Mixes\n\n")
			for _, mix := range v.Mixes {
				fmt.Fprintf(&sb, "  %-14s  %s\n", mix.Name, mix.Progress)
			}
			return s.Dim.Render(strings.TrimRight(sb.String(), "\n"))
		}
		msg := "Personalized Mixes\n\nPlay more tracks to unlock personalized mixes.\nMixes are generated from your listening history\n(minimum 20 plays)."
		return s.Dim.Render(msg)
	}

	if len(v.Mixes) == 0 {
		return s.RenderEmptyCard(v.width, v.Height, "No mixes yet", []styles.EmptyAction{
			{Key: "1", Label: "go to library"},
			{Key: "?", Label: "help"},
		})
	}

	var lines []string
	for i, mix := range v.Mixes {
		focused := i == v.SelectedMix
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

func renderMixListRow(cursor string, mix recommendation.MixInfo, width int) string {
	name, rest := splitMixListRow(cursor, mix, width)
	if width <= 0 {
		return cursor + name + rest
	}
	return text.Truncate(cursor+name+rest, width)
}

func splitMixListRow(cursor string, mix recommendation.MixInfo, width int) (name, rest string) {
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
