package tui

import "github.com/sglavoie/muse/internal/db"

// stubTagReader returns canned tag data for tag-picker tests.
type stubTagReader struct {
	tags       []db.TagRow
	trackTags  map[string][]db.TagRow // hash -> tags
	tagCounts  map[int64]int
	tracksByID map[int64][]db.PlaylistTrackRow
	hashesByID map[int64][]string
}

func (s *stubTagReader) ReadTags() ([]db.TagRow, error) {
	return s.tags, nil
}
func (s *stubTagReader) ReadTagsForTrack(hash string) ([]db.TagRow, error) {
	return s.trackTags[hash], nil
}
func (s *stubTagReader) ReadTrackHashesByTag(tagID int64) ([]string, error) {
	return s.hashesByID[tagID], nil
}
func (s *stubTagReader) ReadTracksByTag(tagID int64) ([]db.PlaylistTrackRow, error) {
	return s.tracksByID[tagID], nil
}
func (s *stubTagReader) ReadTagCounts() (map[int64]int, error) {
	return s.tagCounts, nil
}

// stubTagWriter records tag mutation calls for assertion in tests.
type stubTagWriter struct {
	nextID        int64
	createCalls   []string
	addTrackCalls []struct {
		hash  string
		tagID int64
	}
	addBulkCalls []struct {
		hashes []string
		tagID  int64
	}
}

func (s *stubTagWriter) CreateTag(name, _ string) (int64, error) {
	s.createCalls = append(s.createCalls, name)
	s.nextID++
	return s.nextID, nil
}
func (s *stubTagWriter) RenameTag(_ int64, _ string) error      { return nil }
func (s *stubTagWriter) UpdateTagColor(_ int64, _ string) error { return nil }
func (s *stubTagWriter) DeleteTagSoft(_ int64) error            { return nil }
func (s *stubTagWriter) DeleteTagIfEmpty(_ int64) (bool, error) { return false, nil }
func (s *stubTagWriter) PurgeDeletedTags(_ int64) error         { return nil }
func (s *stubTagWriter) UndoDeleteTag(_ int64) error            { return nil }
func (s *stubTagWriter) AddTrackTag(hash string, tagID int64) error {
	s.addTrackCalls = append(s.addTrackCalls, struct {
		hash  string
		tagID int64
	}{hash, tagID})
	return nil
}
func (s *stubTagWriter) RemoveTrackTag(_ string, _ int64) error { return nil }
func (s *stubTagWriter) RemoveAllTrackTags(_ string) error      { return nil }
func (s *stubTagWriter) AddTrackTagBulk(hashes []string, tagID int64) error {
	s.addBulkCalls = append(s.addBulkCalls, struct {
		hashes []string
		tagID  int64
	}{hashes, tagID})
	return nil
}
func (s *stubTagWriter) RemoveTrackTagBulk(_ []string, _ int64) error { return nil }
func (s *stubTagWriter) MergeTags(_, _ int64) error                   { return nil }
