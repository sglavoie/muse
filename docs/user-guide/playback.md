# Playback

[Back to the overview](../../README.md) · [User guide](../user-guide.md) · Previous: [Views and browsing](views.md) · Next: [Library management](library.md)

**Now Playing** (`I`) fills the screen: the album cover beside the song, a
wide progress bar, the active playback modes, how often you have played it,
its tags, the next three songs in play order, and, for an Auto-DJ pick, why it
was chosen. The cover and text sit together in the middle of the screen, on a
background darkened from the cover's accent color (only as far as text stays
easy to read; **Cover tint** off keeps the plain background). The status bar
steps aside while the view is open, and its notices take the key hint's place.
On short screens the reason and the up-next list give way first; narrow ones
drop the cover.

Up next lists each song title first with its artist dimmed after it; the
artist is shortened before the title. Under Repeat all with shuffle off, the
list wraps around to the start of the queue as playback will. With shuffle on
the next pass is reshuffled when it begins, so the list stops at the end of
the current one rather than guess. A title too long for one line scrolls
slowly after a two-second pause and loops; with animations off it is
shortened instead. When the song changes, the new cover fades in over the old
one in block mode; Kitty covers swap at once, because a fade would mean
uploading a new image for every frame.

When the song changes on its own or with `n`/`p` while you are in another
view, a small card with its cover, title and artist appears for four seconds,
in empty space above the footer when there is some, otherwise at the top or
bottom right of the list, never over the focused row. Any key dismisses it.
Songs you start with `Enter`, and the song restored at startup, get no card.
`track_change_card = false` (or **Track card** in Settings → Display) turns
it off.

**Lyrics.** When the playing song has lyrics, Now Playing shows them between
the listening details and up next. muse reads an `.lrc` file with the same
name as the song next to it (`Song.flac` → `Song.lrc`, in any letter case),
otherwise the lyrics stored in the file's tags (ID3 USLT, Vorbis `LYRICS` or
`UNSYNCEDLYRICS`, MP4 `©lyr`), read with ExifTool or ffprobe like the rest of
the metadata. The lookup happens in the background the first time you open
Now Playing for a song and is kept in memory, so the view never waits for it;
songs you never look at in Now Playing are never read. Synced lyrics follow
the song: the line being sung is bold in the accent color, lines already sung
are dimmed, and the ones coming up are a little dimmer the further away they
are. A long current line wraps to a second row; the others are shortened.
With animations on the highlight eases onto each new line and the window
glides after it; with animations off, or after a seek, it jumps straight to
the right line. An empty timed line is an instrumental break and shows a note
while it plays. Lyrics without timings show as plain text that `j`/`k` or the
arrow keys scroll.

The LRC reader accepts several time tags on one line (`[00:12.34][01:02.00]`),
times with no fraction or up to three digits after the point, `[offset:]` in
milliseconds (a positive offset shows lines earlier), per-word times
(`<00:12.34>`, which are ignored), and files in UTF-8 with or without a
byte-order mark, UTF-16, or Windows-1252/Latin-1. Tags such as `[ar:]` and
`[ti:]` are ignored.

`L` in Now Playing shows or hides lyrics and remembers the choice
(`lyrics = false` in the config, or **Lyrics** in Settings → Display). The key
hint mentions `L` only for songs that have lyrics; songs without any show no
trace of them. On short screens the lyrics keep a three-line window before
the details, up next and the Auto-DJ reason get room, since they are what you
opened the view for while a song has them; press `L` to give that room back.

<a id="album-art"></a>**Album art.** muse looks for an album's cover the first time it needs it: an
image named `cover`, `folder`, `front`, `album`, `albumart` or `artwork`
(`.jpg`, `.png` or `.gif`, any case) next to the music, otherwise the picture
embedded in the file, read with ExifTool or, when installed, ffmpeg. Covers are
cached in `~/Library/Caches/muse/artwork` on macOS and in
`$XDG_CACHE_HOME/muse/artwork` (default `~/.cache/muse/artwork`) elsewhere;
an album with no art is looked at again after a day. Delete the folder to
start over. With `artwork = "auto"`, Ghostty, kitty and muse.app show the real
image through the Kitty graphics protocol and every other terminal draws it
with colored half blocks; inside tmux, screen or Zellij muse always uses
blocks. `artwork = "kitty"`, `"blocks"` or `"off"` overrides the choice; it
applies when muse starts.

The interface takes its accent color (the active tab, focus marks, the
progress bar) from the playing song's cover and eases into each new one.
Covers without a vivid color keep `primary_color`. Turn this off with
`artwork_accent = false` or **Cover tint** in Settings → Display.

When a list leaves room above the footer, the focused song appears there with
its cover, artist and album, length, genre, play count, when you last played
it, and its tags. Below a Library list that does not fill the screen, one line
sums up the collection: its size, total length, how much is still unheard,
and when music last arrived.

Once the screen rests for a moment, muse quietly gets the next covers ready:
the albums a few rows above and below the focused one in the Library and
Queue, and the next two songs in the queue, so moving the cursor or changing
tracks shows the cover at once. It looks up two at a time and keeps only a
handful of covers that have not been shown yet.

Pause and resume fade the audio in and out over about a tenth of a second
(`fade_on_pause`). While music plays, the progress bar glides between position
updates, a small level meter replaces `▶` in the status bar, and the playing
row fades in after a track change; dialogs and overlays fade in over a fifth
of a second as they open, and switching views eases the new panel in below
the tab bar while the tabs and status bar stay put; the track card slides in
from the right. Track lists show titles brightest, artists tinted
toward the accent color, and albums in a quieter tone. `reduce_motion = true` (or **Animations**
in Settings → Display) turns these off and restores the static `▶`.

**Level meter.** The status bar's three bars follow the music itself: the
left one is the left channel's level, the middle one the loudest peak, and
the right one the right channel (a mono song moves both sides together).
Levels come from a light analysis filter added to mpv's audio filters after
normalization, the equalizer and stereo widening, so the meter shows what you
hear before the volume control; it leaves the audio untouched, and costs well
under one percent of a core. muse reads it about twelve times a second, only
while music plays and the meter or the strip is on screen; paused, stopped or
with animations off it reads nothing. Silence leaves the bars flat, and pausing
brings them to rest.

Full-screen Now Playing adds a loudness strip directly above the progress
bar: one column per quarter second, newest on the right, in the accent color
fading into the past, dimmed while paused. It needs a spare row and is the
first thing to go on a short screen.

If mpv cannot measure levels (an FFmpeg without the `astats` filter, for
example), nothing arrives within about a second and the meter quietly falls
back to its animation for the session. `level_meter = "off"` (or **Level
meter** in Settings → Display) never adds the filter: the status bar keeps the
animation and Now Playing has no strip.

**Album covers in Browse.** Browse → Albums lays the albums out as a grid of
covers, with each album's title and artist beneath its cover; the number of
covers per row follows the window width, and wide windows get larger covers.
`j`/`k` (or `↑`/`↓`) move a row, `h`/`l` (or `←`/`→`) move one cover along,
`Ctrl+D`/`Ctrl+U` and `Ctrl+F`/`Ctrl+B` page by rows, and `g`/`G` jump to the
first and last album. The focused cover's title is highlighted and underlined,
and everything that acts on a focused album in the list (Enter, `F`, `N`,
`Shift+Enter`, `/`, `t`) acts on it in the grid. Covers load as they scroll
into view, a few at a time; an album without art, or whose art is still
loading, shows its initials on a plain tile. Press `c` to switch between the
grid and the list; muse remembers the choice across restarts. The grid needs
cover art enabled and room for two covers side by side, otherwise Albums stays
a list; in the list `←`/`→` seek as everywhere else.

Now Playing, Track Info, Queue Summary, Playback Modes, Recent Messages, and
Help accept the usual playback keys: `Space`, `n`/`p`, seek arrows, and `=`/`-`
for volume. The overlay stays open. Text entry, including Help search, continues
to receive those characters normally.

The seek arrows preview where they land: the time and progress bar (in the
status bar and Now Playing) jump to the target at once, marked `→` or `←`, and
muse seeks a moment after the last press, or straight away when you press
another key. Holding `←`/`→` speeds up, from `seek_offset_short` (5 seconds)
a step to twice that after a second and six times after two and a half.

The status bar names enabled modes: **Repeat all/one**, **Shuffle**,
**Stop after track**, and **Auto-DJ**. When none is enabled it says
**Sequential**. Compact combinations use `Rpt:all/one`, `Shuf`, `Stop`, and
`DJ`; enabled states remain distinguishable without color.

**Stop after this song** (`ctrl+n`, or _Toggle stop after current_ in the
command palette) lets the song playing now finish and then stops, without a
fraction of the next one: muse withdraws the song mpv had preloaded for gapless
playback the moment you arm it. The next song is then loaded and left paused
at its start, so `space` (or a media key) carries on exactly where the queue
was. It works with shuffle, Auto-DJ and repeat: under Repeat one the same song
waits, and under Repeat all the last song waits on the first. At the very end
of a queue with nothing to wrap to, playback simply stops. The flag is spent
once it fires, and skipping to another song spends it too. Now Playing says
**Stops after this song** while it is armed.

**Sleep timer** (`:` → _Sleep timer…_) pauses playback later. Choose 15, 30,
45, 60 or 90 minutes, **End of this song**, **End of queue**, or **Off**. A
countdown shows in the status bar and in Now Playing (`☾ 23m`, then seconds
for the last minute); it keeps running across views. When the time is up the
volume fades out over about fifteen seconds, playback pauses, and the volume
goes back to where it was, so the next play is at your normal level. Pressing
play — `space`, a media key or the headphone button — during the fade
cancels the timer and keeps the music on. **End of this song** uses stop after
this song; **End of queue** stops when the queue runs out and holds Auto-DJ
refills and the Repeat all wrap until then (under Repeat one it waits for the
end of the song instead). The timer is not saved: quitting muse cancels it.


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
- **Media keys and Now Playing.** On Linux muse registers on the session bus
  as an MPRIS player (`org.mpris.MediaPlayer2.muse`), so keyboard media keys,
  headset buttons, GNOME and KDE media widgets and `playerctl` see the playing
  track and can play, pause, skip and seek. On macOS, muse.app does the same
  through Control Center's Now Playing, the keyboard's media keys and AirPods;
  the plain terminal muse on macOS has no system media controls (see
  [`docs/karabiner.md`](../karabiner.md) for hotkeys instead). Turn it off with
  `media_controls = false`. Without a session bus (for example over SSH) muse
  simply runs without it.
- **Remote control** of a running instance from another terminal:
  `muse toggle-play`, `muse next`, `muse previous`, `muse volume-up`,
  `muse volume-down`.
