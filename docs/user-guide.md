# Using muse

[Back to the overview](../README.md) · [Setup and configuration](setup.md)

## Features

**Eight primary views**, switched with `1`–`8`:

| View | What it does |
| --- | --- |
| **Library** (`1`) | The whole collection, with sub-modes for All / Recently Added / History. Filter with prefixes (`a:` artist, `t:` title, `l:` album, `g:` genre, `aa:` Album Artist). |
| **Playlists** (`2`) | Static playlists plus smart playlists driven by a rule editor (AND/OR over field/operator/value rules). `/` filters playlist names; open playlists also support `a:`, `t:`, `l:`, `g:`, and `aa:` field prefixes. |
| **Queue** (`3`) | The live play queue: reorder, remove, yank/cut/paste rows, save the queue as a playlist. |
| **Browse** (`4`) | Hierarchical browsing by Artists ↔ Albums ↔ Genres, drilling down to tracks. |
| **Favorites** (`5`) | Favorited tracks, albums, and artists, grouped by category. |
| **Tags** (`6`) | Free-form user tags with create / rename / delete / merge, undo, and a per-track tag picker. |
| **Mixes** (`7`) | Generated recommendation mixes — Rediscover, Hidden Gems, On Repeat, Deep Cuts — tuned by thresholds in `config.toml`. |
| **Insights** (`8`) | Listening statistics: sessions, momentum, discovery, and a data-health panel that surfaces colliding genre spellings, blank fields, and unverified durations. |

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
album detail. Collection counts name their units, such as `3 tracks`.

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
When a table leaves enough space in a window at least 24 rows tall, an area
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

**Playback**

Now Playing, Track Info, Queue Summary, Playback Modes, Recent Messages, and
Help accept the usual playback keys: `Space`, `n`/`p`, seek arrows, and `=`/`-`
for volume. The overlay stays open. Text entry, including Help search, continues
to receive those characters normally.

The status bar names enabled modes: **Repeat all/one**, **Shuffle**,
**Stop after track**, and **Auto-DJ**. When none is enabled it says
**Sequential**. Compact combinations use `Rpt:all/one`, `Shuf`, `Stop`, and
`DJ`; enabled states remain distinguishable without color.


- **Auto-DJ** (`A`) — muse keeps the queue fed by scoring
  candidate tracks against what you have been listening to, avoiding recent
  history. It can be seeded from a mix, from an Insights row, or from whatever
  you have selected (`:` → _Start Auto-DJ from selection_).
- **Steerable Auto-DJ** (`ctrl+a`) — a panel for steering the session while it
  plays, in three tabs:
  - **Tune** — a preset (Familiar 20%, Balanced 40%, Explore 80%, or Custom for
    anything you tune in between), the discovery percentage itself in ten-point
    steps, an anchor mode (`Anchor` keeps scoring against the track you started
    from, `Follow` re-anchors on every track change), and
    include/exclude filters on tags, genres, and artists. `ctrl+s` starts the
    session, or applies the draft to a session already running — the queue is
    then rebuilt in place, ahead of the track you are listening to, without
    interrupting it.
  - **Preview** — the batch the current draft would add, with the reason each
    track was picked, what was ruled out, and any shortage (say, "only 1 of 3
    familiar tracks were available; discovery tracks filled the rest"). It is
    read-only: looking at a draft never changes what is playing or queued.
  - **Recipes** — named settings you can save, update, rename, and delete.
    A recipe stores settings only, not a queue or a seed: saving one changes
    nothing that is playing, and loading one edits the draft, which you then
    start. Settings the panel can no longer resolve — a filter naming a tag you
    have since deleted — are shown as unusable rather than silently widened.

  The DJ only ever touches rows it added. Tracks you queued yourself are left
  alone by a re-tune, and so is the one playing. Auto-DJ and the tuned
  settings survive a restart: muse restores the session, its seed, and its
  discovery balance, and keeps filling from where it left off.
- **Queue duration at a glance** — the Queue header shows time remaining in the
  current batch, including shuffle order. Repeat, Auto-DJ, and stop-after modes
  label it as queued time rather than an end time; `+` marks unknown durations.
  The duration yields to filtering and selection details in narrow windows.
- The queue summary (`q`) updates while open, including track changes and
  duration corrections. Both remaining and total time use `+` for unknown durations.
  Album counts distinguish releases by Album Artist or folder, as Browse does.
- **Copy now-playing info** — a palette command copies the playing or paused
  song while you browse elsewhere; its detail line identifies the song.
- Focused-track copy and reveal commands identify their song or file path in
  the palette detail line. Search synonyms include “locate file,” “restart song,”
  and “errors” (Recent Messages).
- **Browse now-playing…** — a palette command opens artist, album, and genre
  destinations for the playing or paused song, independently of the focused row.
  Its detail line identifies the song; selecting it keeps that target even if
  playback advances while the palette is open.
- **Seek to time…** in the command palette jumps to seconds (`90`), `m:ss`
  (`12:30`), or `h:mm:ss` (`1:02:03`) in the loaded track, preserving pause.
  The dialog shows the current position and track duration, updating while open;
  unknown durations are labeled explicitly. Invalid times stay open for correction;
  changing tracks cancels the target.
- **Gapless playback** via mpv prefetch (`gapless_audio`).
- **10-band equalizer** with built-in presets (Flat, Bass Boost, Treble Boost,
  Rock, Jazz, Classical, Pop) and user-defined custom presets, plus stereo
  widening and EBU R128 volume normalization.
- **Desktop notifications** on track change, and a `muse now-playing` subcommand
  you can bind to a global hotkey.
- **Remote control** of a running instance from another terminal:
  `muse toggle-play`, `muse next`, `muse previous`, `muse volume-up`,
  `muse volume-down`.

**Library management**

- **Metadata editor** (`M`) — fix title, artist, album, Album Artist, and genre on the
  focused track. Edits are stored in muse's database, so your audio files are
  never modified. `i` shows the library values first and labels differing original
  file tags, followed by audio and file details from ExifTool or ffprobe.
  Use `j`/`k`, arrow keys, or Page Up/Down to scroll; `g`/`G` jumps to the
  beginning/end. Long values wrap, and the title shows your position when
  more information is available.
- The metadata form places Album Artist beside Artist and shows the original
  filename when there is room. Duration accepts seconds (`90`), `m:ss` (`1:30`),
  or `h:mm:ss`, matching Seek to time; its format hint appears while focused.
  An empty duration remains unknown.
- Saving an unchanged metadata form leaves your previous undo available.
  Changed fields are marked with `*` in single-track and bulk forms, with a
  changed-field count below the editor. Restoring an original value removes its marker.
  In single-track forms, `ctrl+r` restores the focused field to its opening
  value without changing other edits.
  Editing other fields preserves the precise track duration. Bulk forms label
  differing values as “Multiple values — unchanged”; untouched fields retain
  each track's original value. In bulk forms, `ctrl+d` explicitly clears the
  focused field for every selected track; `ctrl+r` restores that field to
  unchanged. Clear operations show “Will clear for N tracks” before saving
  and support undo.
- Edited metadata forms, Settings, smart playlists, and Auto-DJ drafts ask before
  discarding changes on `Esc`. Smart-playlist rule edits are protected too.
  The default is **Keep editing**; choose **Discard changes** with Tab and Enter.
  Unchanged forms close immediately.
- Album Artist edits regroup albums immediately and support bulk editing, explicit
  clearing, and undo. Corrections survive rescans and restarts. TSV exports include
  `album_artist`; importing an older export without that column leaves it unchanged.
- Enter enqueues selected tracks in Library, open playlists, Browse track lists,
  Favorites track lists, Tags, Mixes, and Insights. With no selection it enqueues the
  focused track. Footers show the selected count, and the command palette offers
  the same action. Hidden selections remain included; repeated playlist entries
  remain separate. Enter still opens collections and plays the focused Queue row.
- Library enqueue, play-next, and replace-queue actions retain the text filter
  and focused row. Clear the filter explicitly with `Esc` when finished.
- Help keeps scrolling, search, scope, and close hints visible, with a scroll percentage.
- The palette’s **Clear selection** command describes the selected row count and
  how many are hidden, and clears only the current view’s selection.
- Track searches match base filenames consistently across views. Matching ignores
  case and Latin accents (`bjork` finds `Björk`), while displayed metadata retains
  its original spelling and highlights. Field prefixes still search only their
  named fields; directory names are excluded from filename matching.

- **Metadata export / import** — round-trip the visible view's rows through a
  TSV file in the data directory, for bulk cleanup outside the TUI. The palette
  shows the number of unique exportable tracks and whether it uses your selection
  or visible rows; at Browse collection level, its detail names the focused
  collection. Re-exporting keeps the previous file, including any external edits, as `metadata-export.tsv.bak`.
  Use the palette’s _Reveal previous metadata export backup_ command to locate it.
  To recover it, copy that backup over `metadata-export.tsv` before importing.
- The track tag picker accepts multiword names, with Enter to toggle or create
  and Space as normal text. A Create row remains available alongside partial
  matches unless the exact name already exists (ignoring case). Long lists scroll
  to keep the focused row visible; Page Up/Down moves a page and the heading
  shows the visible range.
- Bulk selection (`x` / `X`) and visual line selection (`v`) in every track list, including Mixes. In Mixes, `d` dismisses the focused
  recommendation and `D` restores the last dismissal in the current session.
  `x` / `X` always manage selection.
- Favorites selections survive filtering and sorting. Headings show selected and
  hidden counts; bulk actions include hidden selections. Navigating to another
  Favorites list clears the selection.
- Library and Queue selections show their total duration beside the count,
  including selected tracks hidden by filters. Queue counts each selected entry,
  including repeated songs. A separate hidden count shows how many
  selected tracks are outside the current list; scrolling alone does not change it.
  `+` marks unknown durations; the duration
  yields to other details in narrow windows.
- The palette's _Remove queue duplicates_ command previews how many repeated
  entries it will remove, keeping first occurrences. It is unavailable when
  the queue has no duplicates.
- Undo (`u`) retains the latest reversible action until another reversible
  action replaces it or you quit. Its banner fades, but the action stays available
  through `u` and the palette. Queue, playlist, and tag undo failures appear in
  Recent Messages and can be retried. Bulk playlist and tag-assignment restores
  are atomic. Undo banners have no countdown. Deleted rows needed for undo remain available
  until the action is released; interrupted sessions are cleaned up at startup.
- Enqueue confirmations distinguish “Queued next” from “Added to queue,” with
  the same undo shortcut for both.
- Clearing a text filter with `Esc` or the palette keeps the focused row in view; a search with no matches
  returns to your position before filtering. Library and Queue no-match hints
  distinguish `Esc` to clear a text filter from `ctrl+t` to clear a tag filter.
- While editing a filter, the footer shows `Enter` to apply, `Esc` to clear,
  and Up/Down to recall history. `/` is literal text while editing, so names
  such as `AC/DC` can be typed normally. Help search also treats `/` as literal
  text while typing; `Enter` applies the search and `Esc` clears it. Empty field
  prefixes such as `a:` leave tracks visible while you type; other populated terms still apply.
  When space allows, track-list filter editors also show
  how field prefixes were interpreted, such as `Artist: Miles Davis · Genre: Jazz`.
  Browse, Favorites, Tags, Queue, and open playlists support the same field
  prefixes, including `g:` / `genre:` and `aa:` / `albumartist:`, wherever
  individual tracks are listed. Album Artist filters match the stored Album Artist
  value, independently of the track artist; for example, `aa:Various Artists g:Jazz`.
  Collection-name filters continue to match literal text.
- **Recent filters** — while editing `/`, Up/Down recalls the last 20 distinct
  committed queries across views. Down past the newest restores your unfinished
  query. If saving history fails, Recent Messages reports it once until saving
  recovers; queries remain available for the current session. History survives
  restarts in `filter_history.json` in the data directory;
  cancelled queries are excluded, and startup does not apply a saved filter.
- The playlist destination picker supports `/` search, Page Up/Down, and
  `g`/`G` to jump to the first/last destination. Long lists scroll to fit the
  terminal. Search matches playlist names and keeps Play Queue and New playlist
  available. Enter applies the search, then confirms a destination; Esc clears
  the search before dismissing the picker.
  The heading shows the count and total duration of the tracks being added,
  including hidden selections; `+` marks unknown durations.
- Smart-playlist rules validate numeric values before accepting them. Duration
  accepts seconds, `m:ss`, or `h:mm:ss`; favorites offer Yes/No, and Tag rules
  offer a searchable picker of existing tags with equals/not-equals matching.
  Type to search tags (including spaces), use Up/Down to move, and Enter to
  choose. Tag rules keep a stable tag identity: renaming preserves membership, and merging
  follows the destination tag, including drafts saved after the merge. Existing
  rules are migrated automatically. Deleting a referenced tag reports a repair
  message; undo the deletion or edit the rule to choose a replacement. A new tag
  with the same name does not silently replace the deleted one.
  Failed saves keep the draft open for correction or retry; repeated Enter
  presses during a pending save do not create duplicate playlists.
- Open playlists show their total entry count and duration, including repeated
  songs. Filtering preserves selected entries, which have their own duration
  and hidden-selection count;
  `+` marks unknown durations. Playlist palette commands identify their target;
  rename and rule editing apply at list level, while **Refresh smart playlist**
  re-evaluates the open smart playlist.
- Playlist filters preserve individual rows when the same song appears more than
  once. `Esc` clears the track filter before returning to the playlist list, where
  the previous name filter is restored. Reordering and shuffle-in-place require
  an unfiltered track list. `E` still enqueues the entire playlist.
  Failed order saves restore the previous order and appear in Recent Messages;
  shuffle confirms success after saving. Further order changes wait until the
  current save finishes.
- **Direct sorting** — search the palette for _Sort by_ to choose a field supported
  by the current view. The selected field and direction appear in its detail line.
  Sort fields and directions survive restarts in `sort_preferences.json` in the
  data directory. Library modes and collection/track levels remember separate
  preferences; cursors and filters are not saved by this feature. Choose
  _Sort by default order_ to reset a view. The existing sort-cycle and
  direction shortcuts remain available.
- Completed library scans replace progress with “Scan complete — N tracks” and
  retain that result in Recent Messages. Manual rescans also report added and
  missing track counts compared with the library before the scan; moving a file
  with unchanged content does not count as an addition or a disappearance.
  Rescan results wait for database writes, and save failures appear as errors
  with a retry hint in Recent Messages. Canceled and incomplete scans keep their
  distinct messages.
- **Recent messages** — `:` → _Show recent messages_ reopens the last 50 action
  results and errors from this session. Long messages wrap; `j`/`k` scroll, with
  a line position indicator for longer histories. `y` copies the complete snapshot
  newest first, including timestamps, without display wrapping.
  Errors use distinct banner styling and retain an `[error]` label in recent
  messages and copied history. File-manager errors include the affected path.
  Progress updates are excluded, and past messages do not reactivate their shortcuts.

**Supported audio formats**: `.aac`, `.ac3`, `.aif`, `.aifc`, `.aiff`, `.ape`,
`.au`, `.caf`, `.dts`, `.flac`, `.m4a`, `.m4b`, `.m4r`, `.mka`, `.mp2`, `.mp3`,
`.mp4`, `.mpa`, `.oga`, `.ogg`, `.opus`, `.snd`, `.w64`, `.wav`, `.wma`, `.wv`.

`.mp4` is a container that can hold video. muse plays every file as audio only,
but a music directory holding video files will index them; use `excluded_paths`
in the config to keep them out.

## Usage

Collection lists and pickers use `▎` to mark the focused row, independently of
color. Footers distinguish `Enter open` at collection level from `Enter enqueue`
at track level; `enqueue N selected` identifies the selection used by Enter.
Track-row markers keep playback separate: `▎` means focused, `●` means selected,
and `◉` means focused and selected. The playing/paused icon stays beside these
markers, including on selected rows. Collection screens show their full name and
item count, including when navigation tabs are abbreviated.

Settings accepts `Tab` / `Shift+Tab` or `]` / `[` to switch tabs. Inline editors
keep those keys while editing. Settings and Auto-DJ retain save/start and exit
hints in narrow windows; Settings lists scroll to keep the focused row visible.
Changed Settings fields and tabs show `*`; the title counts unsaved changes.
Restoring opening values removes their markers. Directory/exclusion lists and
custom presets also contribute to the count.
Insights uses a compact panel selector in ordinary windows and shows its full
briefing when the terminal is tall enough. Coverage leads with the number and
duration of unheard tracks, followed by artists and albums to explore and tracks
to hear again; each row keeps its existing listening actions. Use backtick/tilde
to change panels.

Unavailable command-palette entries explain what
you need to select or open before using them. Focus a toggle command to see its
current setting in the palette’s detail line. Undo describes the change it will
reverse, even after its notification fades. It becomes unavailable after a
successful undo or when no reversible action has been recorded.

Queue commands describe their target in the palette's detail line. Enqueue, Play next (`N`), and Replace queue (`shift+enter`) all use the
selection when present, including hidden tracks and separate playlist occurrences;
otherwise they use the focused track. Palette details state the count and how
many are hidden. Play next keeps the selected display order. Replacement saves
atomically before switching playback; failure preserves the queue, selection,
and previous undo. Undo restores the replaced queue, including its original row
identities, order, and Auto-DJ provenance, and attempts to resume its playing or
paused track at the saved position. A failed database restore stays retryable.
If another queue write or playback transition is pending, replacement asks you
to retry once it settles. Commands needing a track are unavailable at
collection level or when there are no matching tracks.
The metadata command follows the same targets as `M`, including artist and album
renaming in Browse, bulk selections, and Data Health findings. Its detail line
identifies the track or shows the affected track count.
Palette search also accepts synonyms such as `refresh library`, `EQ`, and
`tune radio`; matches against displayed command names rank first. Seek and volume commands show
their configured step sizes in the detail line.

Run `muse` with no arguments to launch the TUI. The subcommands are:

| Command | Description |
| --- | --- |
| `muse check` | Validate config and prerequisites, then exit |
| `muse now-playing` | Notify the current track via a desktop notification |
| `muse toggle-play` | Toggle play/pause on the running instance |
| `muse next` | Skip to the next track |
| `muse previous` | Skip to the previous track |
| `muse volume-up` | Increase volume by `volume_increment` (default 5%) |
| `muse volume-down` | Decrease volume by `volume_increment` (default 5%) |

The five playback commands are relayed to an already-running muse over a Unix
control socket, so they are convenient to bind to global hotkeys — see
[`docs/karabiner.md`](karabiner.md) for a macOS Karabiner-Elements setup.

Global flags: `muse --version`, `muse --help`, and `muse --control PATH` (see
[Hosting muse](architecture.md#hosting-muse)).

### Core keybindings

Help search matches section headings as well as shortcuts and descriptions; matching
a heading shows all its controls.

Press `?` inside muse for the complete, searchable list — the table below is
just enough to get moving.

| Key | Action |
| --- | --- |
| `?` | Show help |
| `ctrl+c` | Quit |
| `space` | Play / pause |
| `s` / `n` / `p` | Stop / next track / previous track |
| `left` / `right` | Seek ±5s (`shift` for ±30s) |
| `=` / `-` | Volume up / down |
| `1`–`8` | Switch to Library / Playlists / Queue / Browse / Favorites / Tags / Mixes / Insights |
| `tab` / `shift+tab` | Next / previous view |
| `j` `k` / `down` `up` | Move cursor |
| `ctrl+d` / `ctrl+u` | Half-page down / up |
| `g` / `G` | Go to top / bottom |
| `/` | Filter the current view |
| `esc` | Clear the current text filter; otherwise go back |
| `enter` | Enqueue / open the focused row |
| `x` / `X` / `v` | Toggle bulk selection / clear selection / visual line mode |
| `r` / `z` | Cycle repeat mode / toggle shuffle |
| `A` | Toggle Auto-DJ |
| `ctrl+a` | Tune Auto-DJ — presets, discovery, filters, plus its Preview and Recipes tabs |
| `F` | Favorite the focused track, album, or artist |
| `t` / `ctrl+t` | Manage tags / filter views by tag |
| `M` / `i` | Edit metadata / show track info |
| `ctrl+r` | Rescan library |
| `ctrl+s` | Cycle sort field (`shift+up` / `shift+down` for direction) |
| `:` | Command palette |
| `ctrl+,` | Settings |
| `u` | Undo |

muse renders normal layout at terminal sizes of **60x15 and larger**; below
that it shows a "terminal window is too small" message instead.


## Compact windows and navigation

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

### Error feedback

Failures take priority over routine confirmations. Long banners are shortened
to preserve the playing song and clock; their full text remains in
`:` → **Show recent messages**, where it wraps and can be scrolled or copied.
A `! errors · :` indicator remains after banners fade, until you open that
history. Errors arriving after you open it remain unread; reopen to inspect
them. A successful undo retry retires its old failure banner without deleting
the history.

### Dialog guidance and Data Health

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
status bar: add a folder when unconfigured, check folders and exclusions when
no music was found, or check folders after a scan error. During a scan, progress
and the cancel action remain visible even with an empty queue.

Auto-DJ previews distinguish a quota substitution from an undersized batch.
They report how many extra tracks from the other category were actually added;
a shortfall does not imply that the missing tracks were replaced.

### Draft clarity and list position

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
