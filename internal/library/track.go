package library

// Track represents a single audio file in the library with its extracted metadata.
type Track struct {
	Path     string
	Hash     string
	Artist   string
	Title    string
	Album    string
	Duration float64 // seconds
}
