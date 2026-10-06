package playlists

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/text"
)

func (v View) renderTrackHeading() string {
	title := "Playlists"
	if v.playlistName != "" {
		title += " — " + v.playlistName
	}
	if v.IsSmartPlaylist {
		title += "  (smart playlist — read-only)"
	}
	if v.Width > 0 {
		title = text.Truncate(title, v.Width)
	}
	return v.Styles.Heading.Render(title)
}

func (v View) renderTrackSummary() string {
	tracks := v.allTracks
	if tracks == nil {
		tracks = v.Tracks
	}
	var total, selected text.DurationTotal
	var selection strip.SelectionChunk
	for _, track := range tracks {
		total.Add(float64(track.DurationMs) / 1000)
		if _, ok := v.SelectedTrackIDs[track.ID]; ok {
			selection.Count++
			selected.Add(float64(track.DurationMs) / 1000)
		}
	}
	selection.Hidden = selection.Count
	for _, track := range v.Tracks {
		if _, ok := v.SelectedTrackIDs[track.ID]; ok {
			selection.Hidden--
		}
	}
	selection.Duration = selected.String()
	label := fmt.Sprintf("%d %s", len(tracks), text.Pluralize(len(tracks), "track", "tracks"))
	if len(v.Tracks) != len(tracks) {
		label = fmt.Sprintf("%d/%d tracks", len(v.Tracks), len(tracks))
	}
	return strip.Render(v.Width, []string{label}, strip.SortChunk{}, strip.FilterChunk{},
		strip.CountsChunk{}, &selection, total.String(), v.Styles)
}
