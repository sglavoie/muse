package autodj

import "github.com/sglavoie/muse/internal/radio"

// Mark semantic changes, ignoring filter order and case just like Dirty.
func (p TunePanel) rowDirty(r row) bool {
	switch r {
	case rowPreset:
		return p.draft.Preset() != p.applied.Preset()
	case rowDiscovery:
		return p.draft.DiscoveryPercent != p.applied.DiscoveryPercent
	case rowAnchor:
		return p.draft.Anchor != p.applied.Anchor
	}
	fr, ok := filterRows[r]
	if !ok {
		return false
	}
	baseline := p
	baseline.draft = p.applied
	return !fieldSubset(p.filterSet(fr.exclude), fr.field).Equal(fieldSubset(baseline.filterSet(fr.exclude), fr.field))
}

func fieldSubset(set radio.FilterSet, field filterField) radio.FilterSet {
	switch field {
	case fieldTags:
		return radio.FilterSet{TagIDs: set.TagIDs}
	case fieldGenres:
		return radio.FilterSet{Genres: set.Genres}
	default:
		return radio.FilterSet{Artists: set.Artists}
	}
}
