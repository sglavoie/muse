# Usage

[Back to the overview](../../README.md) · [User guide](../user-guide.md) · Previous: [Library management](library.md) · Next: [Compact windows and navigation](compact-windows.md)

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
to change panels. When nothing was played in the chosen window, the listening
panels fold into one line saying so, with the window and the `w`/`W` keys to
change it; Coverage and its lists stay, and Data Health stays when it has
findings. As soon as any listening panel has data, every panel is shown again.

**Year in review** (`Y` in Insights, or "Show year in review" in the command
palette) tells the story of a year of listening, one full-screen card at a
time, with `● ○ ○` marking your place. It opens on the current year, or on the
latest year you played anything in. The cards are: listening time, plays and
how many tracks, artists and albums you heard, compared with the year before;
your top five artists, each beside the cover of the album you played most;
your top five tracks; the album of the year with its cover; your listening
clock (plays by hour of the day and by weekday); your longest run of days
with music, over a calendar of the year; your biggest discovery, the most
played song you first heard that year; the song you played most in a single
day; your genre mix; and a summary that fits everything on one screen. Cards
with nothing to show are skipped.

Space, `→` or `l` turns the page and `←` or `h` goes back; `g` and `G` jump
to the first card and the summary. `[` and `]` move between the years you
have plays in and, after the newest, the last 12 months ("Show last 12 months
recap" in the palette opens there directly), staying on the same card. `S`
saves the period's 50 most played tracks as a playlist named "Muse recap:
2025" (or "Muse recap: 12 months to …"), and `esc` or `Y` closes the recap.
Days and hours are local time, the way the rest of Insights counts them, and
only counted plays take part: a skip never counts, and a play you stopped
early adds the time you actually listened. On the album, discovery and
repeat cards the background and accent follow the cover, as in Now Playing
(**Cover tint** off keeps the theme). Each card eases in and its numbers
count up; with `reduce_motion` the cards appear at once.

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
[`docs/karabiner.md`](../karabiner.md) for a macOS Karabiner-Elements setup. On
Linux, `playerctl --player=muse play-pause` (and `next`, `previous`,
`position 30`) reach muse through MPRIS as well.

Global flags: `muse --version`, `muse --help`, and `muse --control PATH` (see
[Hosting muse](../architecture.md#hosting-muse)).

## First run

Until muse has a music folder, the Library shows a welcome screen with the
muse name, a line about what it does, and how to begin. When your system's
music folder exists (`~/Music` on macOS; `$XDG_MUSIC_DIR`, or else `~/Music`,
on Linux) and is not configured yet, Enter adds it to `music_directories`,
saves the config and starts scanning. `ctrl+,` opens Settings to choose other
folders and `?` shows every key. If folders are configured but contain no
music, the Library keeps its guidance to check folders and exclusions.

While the first scan fills an empty library, the Library shows how many tracks
have been found, the folder being read, and covers of the latest albums as
they turn up. `esc` cancels the scan and Enter shows the tracks found so far.
When the scan finishes, the Library lists your music and the status bar says
how many tracks and albums were found.

## Core keybindings

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
| `left` / `right` | Seek ±5s (`shift` for ±30s); hold to speed up |
| `=` / `-` | Volume up / down |
| `1`–`8` | Switch to Library / Playlists / Queue / Browse / Favorites / Tags / Mixes / Insights |
| `tab` / `shift+tab` | Next / previous view |
| `ctrl+o` / `ctrl+i` | Jump back / forward through the places you visited, like Vim's jump list (`ctrl+i` needs a terminal that tells it apart from `tab`, such as kitty, Ghostty, WezTerm or foot) |
| `j` `k` / `down` `up` | Move cursor |
| `ctrl+d` / `ctrl+u` | Half-page down / up |
| `g` / `G` | Go to top / bottom |
| `h` `l` / `left` `right` | Move between covers in the Browse album grid |
| `c` | Switch Browse albums between the cover grid and the list |
| `/` | Filter the current view |
| `esc` | Clear the current text filter; otherwise go back |
| `enter` | Enqueue / open the focused row |
| `x` / `X` / `v` | Toggle bulk selection / clear selection / visual line mode |
| `r` / `z` | Cycle repeat mode / toggle shuffle |
| `ctrl+n` | Stop after this song (the next one waits, paused) |
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
