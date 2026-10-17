package library

import (
	"path/filepath"
)

// AlbumDisplayLabels shortens folder-backed release names for collection lists.
// Pass the unfiltered collection so labels do not change while searching. Keys,
// matching and AlbumLabel (the full label used in details/search) stay intact.
func AlbumDisplayLabels(keys []string) map[string]string {
	labels := make(map[string]string, len(keys))
	groups := make(map[string][]string)
	for _, key := range keys {
		if _, exists := labels[key]; exists {
			continue
		}
		labels[key] = AlbumLabel(key)
		groups[AlbumTitle(key)] = append(groups[AlbumTitle(key)], key)
	}
	for _, group := range groups {
		shortenAlbumGroup(group, labels)
	}
	return labels
}

func shortenAlbumGroup(group []string, labels map[string]string) {
	counts := albumSuffixCounts(group)
	for _, key := range group {
		a, ok := ParseAlbumKey(key)
		if !ok || a.Kind != "folder" {
			continue
		}
		title := a.Title
		if title == "" {
			title = "(unknown album)"
		}
		labels[key] = title
		if len(group) == 1 {
			continue
		}
		qualifier := "folder: " + a.Owner
		for _, suffix := range folderSuffixes(a.Owner) {
			if counts[suffix] == 1 {
				qualifier = suffix
				break
			}
		}
		labels[key] = title + " — " + qualifier
	}
}

func folderSuffixes(path string) []string {
	path = filepath.Clean(path)
	suffix := filepath.Base(path)
	out := []string{suffix}
	for parent := filepath.Dir(path); parent != "." && parent != string(filepath.Separator); parent = filepath.Dir(parent) {
		suffix = filepath.Join(filepath.Base(parent), suffix)
		out = append(out, suffix)
	}
	if out[len(out)-1] != path {
		out = append(out, path)
	}
	return out
}

func albumSuffixCounts(group []string) map[string]int {
	// Count suffixes once per release, avoiding pairwise comparisons even for
	// large collections of identically named albums.
	counts := make(map[string]int)
	for _, key := range group {
		a, ok := ParseAlbumKey(key)
		if !ok {
			continue
		}
		if a.Kind == "folder" {
			for _, suffix := range folderSuffixes(a.Owner) {
				counts[suffix]++
			}
		} else {
			counts[a.Owner]++
		}
	}
	return counts
}
