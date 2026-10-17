package tui

import "github.com/sglavoie/muse/internal/db"

func (m Model) remapSmartPlaylistTag(sourceID int64, destination db.TagRow) Model {
	for i, playlist := range m.playlists.Playlists {
		if !playlist.IsSmart {
			continue
		}
		query, err := db.ParseSmartPlaylistQuery(playlist.Query)
		if err != nil {
			continue
		}
		query.Root = db.RemapSmartRuleTag(query.Root, sourceID, destination)
		m.playlists.Playlists[i].Query, _ = db.MarshalSmartPlaylistQuery(query)
	}
	if m.playlists.SmartQuery != nil {
		query := *m.playlists.SmartQuery
		query.Root = db.RemapSmartRuleTag(query.Root, sourceID, destination)
		m.playlists.SmartQuery = &query
	}
	if m.overlay.smartRuleEditor != nil {
		editor := m.overlay.smartRuleEditor.RemapTag(sourceID, destination).WithTags(m.tags)
		m.overlay.smartRuleEditor = &editor
	}
	return m.preservePlaylistsSort()
}
