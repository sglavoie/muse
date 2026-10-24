package library

import "encoding/json"

// ExifTool's spelling varies by container: Vorbis uses Albumartist and ID3
// uses Band. Prefer the explicit album-artist tag when both are present.
func exifAlbumArtist(raw map[string]json.RawMessage) string {
	return firstExifString(raw, "AlbumArtist", "Albumartist", "Band")
}
