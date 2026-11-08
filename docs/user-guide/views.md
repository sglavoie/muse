# Views and browsing

[Back to the overview](../../README.md) · [User guide](../user-guide.md) · Next: [Playback](playback.md)

**Eight primary views**, switched with `1`–`8`:

| View | What it does |
| --- | --- |
| **Library** (`1`) | The whole collection, with sub-modes for All / Recently Added / History. Filter with prefixes (`a:` artist, `t:` title, `l:` album, `g:` genre, `aa:` Album Artist). |
| **Playlists** (`2`) | Static playlists plus smart playlists driven by a rule editor (AND/OR over field/operator/value rules). `/` filters playlist names; open playlists also support `a:`, `t:`, `l:`, `g:`, and `aa:` field prefixes. |
| **Queue** (`3`) | The live play queue: reorder, remove, yank/cut/paste rows, save the queue as a playlist. |
| **Browse** (`4`) | Hierarchical browsing by Artists ↔ Albums ↔ Genres, drilling down to tracks. Albums show as a grid of covers (`c` switches to a list). |
| **Favorites** (`5`) | Favorited tracks, albums, and artists, grouped by category. |
| **Tags** (`6`) | Free-form user tags with create / rename / delete / merge, undo, and a per-track tag picker. |
| **Mixes** (`7`) | Generated recommendation mixes — Rediscover, Hidden Gems, On Repeat, Deep Cuts — tuned by thresholds in `config.toml`. |
| **Insights** (`8`) | Listening statistics: sessions, momentum, discovery, and a data-health panel that surfaces colliding genre spellings, blank fields, and unverified durations. `Y` opens Year in review, a full-screen recap of a year of listening. |

Albums are grouped by title and **Album Artist**. When Album Artist is missing,
muse uses the containing folder, keeping untagged compilations together. Album
rows show Album Artist when present. Folder-backed albums show just the title
when unique in the collection, or the shortest distinguishing folder suffix
when multiple releases share that title. Filtering preserves those labels.
Drill-down headings retain these concise labels. Full folder paths remain
searchable and available in track details.
Untagged discs in separate folders remain separate; a shared Album Artist tag
groups them across folders. Existing title-only album favorites apply to every
matching release, and each release can then be unfavorited independently.

Browsing keeps your place: opening a collection and returning with `Esc`
restores its filter, sort, focused item, and scroll position. This applies to
Browse, Favorites, Tags, and Playlists. If that item was removed, focus moves
to a remaining row. Press `Esc` again to clear a restored filter.

On exit, muse saves your current view, open collection, filters, sort, and focused
row. The next launch restores that location, including the parent context that
`Esc` returns to. Tracks and collections are matched by identity; missing tracks
fall back to a nearby row and missing collections return to their parent. A
restored filter remains visible. Pressing a key takes control and cancels any
unfinished restoration. Open editors and bulk selections are not restored.

The navigation bar keeps all eight numbered shortcuts visible, abbreviating
view names in compact terminals. Filter badges appear on the divider below it
so tabs stay in place. Track tables put Title before Artist, followed by Album
and Duration when applicable; compact tables give song identity priority over
album detail. Widening a table preserves its title and artist widths; album,
source, and play-count columns appear only when they have useful space. Collection counts name their units, such as `3 tracks`.

Command palettes use the same compact layout at every size, showing six commands
at the minimum terminal size. With an empty search, enabled listening actions
(enqueue, play next, preview, and track information) come first, followed by other
commands for the current view.
Removal actions come after other enabled commands. Usage ranks commands within
these groups. Broad topic searches such as `queue` favor navigation and
inspection over removal; explicit names such as `clear queue` retain their
query ranking. Help and Quit use `F1` and `ctrl+c` footer shortcuts. Both remain searchable. Shortcut labels
appear only when that key runs the action in the current view. Resizing keeps
the focused command, and unavailable commands retain their explanations.

Compact footers prioritize the current task: Queue keeps `d remove`, selection
mode keeps `X clear`, and Library keeps `: commands`. Generic navigation hints
yield before these actions. `i` opens full track information from track lists.
When a table leaves enough spare rows, an area
of up to four lines above the footer labels the focused song's title, artist,
and album with **Focused**, independently of playback. The playback bar emphasizes the playing song's title;
modes and playlist source use quieter text. Long titles and artist/album details
wrap within this area, using spare space without reducing the number of visible tracks.

Track Info (`i`) opens stored library metadata immediately and loads file details
in the background. If a file cannot be inspected, the library details remain
available with an explanation in the File metadata section. Closing the panel
discards late results. Long editable metadata values use `‹` and `›` to show
which side has hidden text.

Settings and Auto-DJ share a fixed frame across their tabs, with scrolling
content and a pinned footer. `tab/shift+tab` moves between tabs. Insights charts
align names, bars, and values within each section, including while scrolling.
