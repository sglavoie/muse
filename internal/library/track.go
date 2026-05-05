package library

// Track represents a single audio file in the library with its extracted metadata.
type Track struct {
	Path         string
	Hash         string
	Artist       string
	Title        string
	Album        string
	Genre        string
	Duration     float64 // seconds
	CreatedAt    int64   // Unix timestamp when the track was first added to the library
	ModTime      int64   // Unix timestamp of last modification
	SizeBytes    int64   // file size in bytes
	PlayCount    int
	LastPlayedAt int64 // Unix timestamp of most recent play (0 if never played)
	TrackNumber  int   // 0 = unknown
	DiscNumber   int   // 0 = unknown
}
