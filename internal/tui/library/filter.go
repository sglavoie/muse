package library

// ActivateFilter sets the filter bar to active, ready to accept text input.
// The existing FilterText is preserved so re-activation resumes the last query.
func (v View) ActivateFilter() View {
	v.FilterActive = true
	return v
}

// AppendFilterChar appends ch to the filter query string. It is a no-op when
// the filter bar is not active.
func (v View) AppendFilterChar(ch rune) View {
	if !v.FilterActive {
		return v
	}
	v.FilterText += string(ch)
	return v
}
