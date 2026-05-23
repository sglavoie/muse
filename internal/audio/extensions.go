package audio

import (
	"slices"
	"strings"
)

var extensions = map[string]struct{}{
	".mp3":  {},
	".flac": {},
	".ogg":  {},
	".oga":  {},
	".opus": {},
	".m4a":  {},
	".m4b":  {},
	".wav":  {},
	".wma":  {},
	".aac":  {},
	".aif":  {},
	".aifc": {},
	".aiff": {},
	".ape":  {},
	".mka":  {},
	".wv":   {},
}

// IsSupportedExtension reports whether ext is a supported audio file extension.
// ext may be supplied with or without a leading dot and is matched case-insensitively.
func IsSupportedExtension(ext string) bool {
	ext = normalizeExtension(ext)
	if ext == "" {
		return false
	}
	_, ok := extensions[ext]
	return ok
}

// SupportedExtensions returns the supported extensions in stable sorted order.
func SupportedExtensions() []string {
	out := make([]string, 0, len(extensions))
	for ext := range extensions {
		out = append(out, ext)
	}
	slices.Sort(out)
	return out
}

func normalizeExtension(ext string) string {
	ext = strings.TrimSpace(ext)
	if ext == "" {
		return ""
	}
	ext = strings.ToLower(ext)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}
