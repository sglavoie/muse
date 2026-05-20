package library

import tea "charm.land/bubbletea/v2"

// ActivateFilter sets the filter bar to active, ready to accept text input.
// The existing Filter.Text is preserved so re-activation resumes the last query.
func (v View) ActivateFilter() View {
	next, _ := v.Filter.Activate()
	v.Filter = next
	return v
}

// AppendFilterChar appends ch to the filter query string and resets the cursor
// to 0 since the displayed track list changes with each keystroke. It is a
// no-op when the filter bar is not active. ScrollOffset is also reset to 0
// so the cursor remains visible — otherwise a prior scroll position past the
// filtered result count would silently render "No matches" or hide the cursor.
func (v View) AppendFilterChar(ch rune) View {
	if !v.Filter.Active {
		return v
	}
	next, _ := v.Filter.Update(tea.KeyPressMsg{Text: string(ch)})
	v.Filter = next
	v.Cursor = 0
	v.ScrollOffset = 0
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
// ScrollOffset is also reset to 0 to keep the cursor visible — otherwise a
// stale scroll position from the filtered view could leave the cursor below
// the visible window after the list expands back.
func (v View) DeactivateFilter() View {
	v.Filter = v.Filter.Deactivate()
	v.Cursor = 0
	v.ScrollOffset = 0
	return v
}
