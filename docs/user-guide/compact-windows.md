# Compact windows and navigation

[Back to the overview](../../README.md) · [User guide](../user-guide.md) · Previous: [Usage](usage.md)

At narrow widths, track tables hide Album before shortening song identities.
Queue also hides Source, Favorites hides Plays, and History shows the date
without its time. Full detail returns when the window grows.

The command palette (`:`) opens with up to eight suggested actions, grouped as
Listening, This view, and Other actions. Type to search every command, including
actions outside the shortlist and unavailable actions with recovery hints.
It reduces padding in short windows and shows your position in its results. Up/Down moves one command; Page Up/Down moves a page.
Help and Quit remain in the footer and appear in search results when they match. A search with no results cannot execute a command.

View-specific help (`?`) begins with that view's actions. Empty-view footers
show recovery or creation controls instead of actions requiring a row.

## Error feedback

Failures take priority over routine confirmations. Long banners are shortened
to preserve the playing song and clock; their full text remains in
`:` → **Show recent messages**, where it wraps and can be scrolled or copied.
A `! errors · :` indicator remains after banners fade, until you open that
history. Errors arriving after you open it remain unread; reopen to inspect
them. A successful undo retry retires its old failure banner without deleting
the history.

## Dialog guidance and Data Health

Settings and Auto-DJ use a second hint line when space permits, keeping complete
key/action pairs together. Their tab headers identify Tab/Shift+Tab navigation.
Settings uses spare space below its fields to explain the focused value, its
units, and whether it previews live, applies on save, or affects the next launch.

Auto-DJ Preview leads with the proposed batch and keeps its track count,
discovery balance, and existing-queue count visible while scrolling. The footer
shows the visible line range. The existing queue and its explanations follow
the proposed tracks.

Data Health presents findings as repair rows with affected-track counts and
contextual actions. `m` merges genre variants after confirmation; `M` opens the
metadata editor for a finding's tracks; Enter opens the affected tracks for
inspection. Rows prefixed **Review:** are advisory, such as possible artist
duplicates or durations that have not been verified, rather than confirmed
metadata errors. At narrow widths the focused finding's action stays in the
footer.

Empty Library screens use the same recovery action in the card, footer, and
status bar: add a folder when unconfigured (see [First run](usage.md#first-run)), check folders and exclusions when
no music was found, or check folders after a scan error. During a scan, progress
and the cancel action remain visible even with an empty queue.

Auto-DJ previews distinguish a quota substitution from an undersized batch.
They report how many extra tracks from the other category were actually added;
a shortfall does not imply that the missing tracks were replaced.

## Draft clarity and list position

Auto-DJ marks unapplied controls with `*` and keeps the highest-priority warning
visible even at 60×15. Discovery is the target share of picks unheard in the last
90 days, including never-played tracks. Anchor can stay near the starting track
or follow the current track. Empty inclusion filters show `any`; exclusions show
`none`. Tune's scrolling hint identifies the visible settings range.

The metadata editor accepts `ctrl+s` or Enter to save. Edits update muse only;
audio files stay unchanged. When room permits, a long focused value appears as
a wrapped **Draft** beneath its input so its beginning remains visible.

Scrollable Library, Queue, Browse, Playlists, Favorites, and Tags lists show
ranges such as `41–58 of 2340`. Ranges count records, excluding group headings
and dividers, and disappear when every record fits. Filtering updates the range
to the matching records. Selection scope takes priority when space is tight.
