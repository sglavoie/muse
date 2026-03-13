package library

// ActivateFilter sets the filter bar to active, ready to accept text input.
// The existing FilterText is preserved so re-activation resumes the last query.
func (v View) ActivateFilter() View {
	v.FilterActive = true
	return v
}

// AppendFilterChar appends ch to the filter query string and resets the cursor
// to 0 since the displayed track list changes with each keystroke. It is a
// no-op when the filter bar is not active.
func (v View) AppendFilterChar(ch rune) View {
	if !v.FilterActive {
		return v
	}
	v.FilterText += string(ch)
	v.Cursor = 0
	return v
}

// DeactivateFilter clears the filter query, deactivates the filter bar, and
// resets the cursor to 0, restoring the full unfiltered track list.
func (v View) DeactivateFilter() View {
	v.FilterActive = false
	v.FilterText = ""
	v.Cursor = 0
	return v
}
