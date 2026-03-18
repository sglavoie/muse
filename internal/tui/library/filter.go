package library

// ActivateFilter sets the filter bar to active, ready to accept text input.
// The existing Filter.Text is preserved so re-activation resumes the last query.
func (v View) ActivateFilter() View {
	v.Filter = v.Filter.Activate()
	return v
}

// AppendFilterChar appends ch to the filter query string and resets the cursor
// to 0 since the displayed track list changes with each keystroke. It is a
// no-op when the filter bar is not active.
func (v View) AppendFilterChar(ch rune) View {
	next, changed := v.Filter.AppendChar(ch)
	v.Filter = next
	if changed {
		v.Cursor = 0
	}
	return v
}

// BackspaceFilter removes the last rune from the filter query and resets the
// cursor to 0 when the text changes. It is a no-op when the filter bar is not
// active or the text is already empty.
func (v View) BackspaceFilter() View {
	next, changed := v.Filter.Backspace()
	v.Filter = next
	if changed {
		v.Cursor = 0
	}
	return v
}

// CommitFilter deactivates the filter bar input while keeping the current
// Filter.Text so the filtered results remain visible. Use this when the user
// confirms the query (Enter or /) and wants to navigate the filtered list.
func (v View) CommitFilter() View {
	v.Filter = v.Filter.Commit()
	return v
}

// DeactivateFilter clears the filter query, deactivates the filter bar, and
// resets the cursor to 0, restoring the full unfiltered track list.
func (v View) DeactivateFilter() View {
	v.Filter = v.Filter.Deactivate()
	v.Cursor = 0
	return v
}
