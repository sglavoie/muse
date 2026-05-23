package audio

import "strings"

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

// Extensions is the set of supported audio file extensions (lowercase, with dot).
var Extensions = cloneExtensions()

func cloneExtensions() map[string]struct{} {
	out := make(map[string]struct{}, len(extensions))
	for ext := range extensions {
		out[ext] = struct{}{}
	}
	return out
}

// IsSupportedExtension reports whether ext is a supported audio file extension.
// ext may be supplied with or without a leading dot and is matched case-insensitively.
func IsSupportedExtension(ext string) bool {
	if ext == "" {
		return false
	}
	ext = strings.ToLower(ext)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	_, ok := extensions[ext]
	return ok
}
