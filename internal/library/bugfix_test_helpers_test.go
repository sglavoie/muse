package library

import "context"

// whitespaceFieldExtractor returns tracks whose metadata fields contain only
// whitespace, simulating files written by broken taggers that emit " " or
// "\t" instead of leaving the tag blank.
type whitespaceFieldExtractor struct{}

func (e *whitespaceFieldExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{
			Path:   f.Path,
			Hash:   f.Hash,
			Title:  "   ",
			Artist: "\t",
			Album:  "  ",
			Genre:  " ",
		}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*whitespaceFieldExtractor)(nil)
