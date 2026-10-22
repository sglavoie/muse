# README demos

## Animated SVG

The README embeds [`muse-tour.svg`](muse-tour.svg): a 22-second, 11-scene tour
of the real TUI, rendered as vector text and shapes. It uses the same approach
as [tt](https://github.com/sglavoie/tt): export ANSI frames from the application's
fixture harness, preserve their styles with Rich, and use CSS to show one frame
at a time. No JavaScript, embedded screenshots, or external fonts are needed.
Reduced-motion preferences hold the opening frame.

Regenerate it with Go, Python 3, and `uv` installed:

```sh
just readme-demo
```

[`capture_readme_demo.py`](../../scripts/capture_readme_demo.py) drives the
`playback` fixture at 100×26, checks every action for layout issues and each
scene for expected content, and writes ANSI frames into ignored build output.
[`render_terminal_svg.py`](../../scripts/render_terminal_svg.py) converts those
frames into the checked-in SVG, using pinned Rich 14.2.0. Edit `SCENES` in the
capture script to change the tour's keys, captions, and hold times.

This runs without a desktop window or a music library. The fixture simulates
playback in memory; the SVG is a rendered interface tour, not a recording of
the native window. The desktop recording below separately verifies the app
and real mpv playback.

For this capture, the harness ran on the MacBook over SSH. To regenerate there
using the staging directory described below, also copy
`scripts/capture_readme_demo.py`, run it from the staging root with Python 3,
and retrieve `macos/build/demo/readme-frames.json`. Render the retrieved frames:

```sh
uv run scripts/render_terminal_svg.py --animate \
  macos/build/demo/readme-frames.json docs/demo/muse-tour.svg
```

## Native desktop recording

The demo records the actual Ghostty-backed macOS window. A small Swift driver
posts keyboard events to the demo process and captures only that window with
ScreenCaptureKit. Vision text recognition checks the expected content in each
scene before it is included. It is a silent, scripted walkthrough, not a
rendered mockup or a screen recording of the rest of your desktop.

The recorder also checks muse's control socket to confirm a track is playing
before saving the generated assets.

## Record

Install the [macOS build prerequisites](../../macos/README.md#prerequisites),
plus `ffmpeg` and Python 3:

```sh
brew install ffmpeg python
python3 macos/scripts/record-demo.py --check
just mac-demo
```

macOS must grant **Accessibility** and **Screen Recording** to the terminal
or agent application running the command. Set these in System Settings →
Privacy & Security, restarting the host if requested. The preflight exits
before launching muse when either permission is missing. Use a display large
enough for a 1200×800 window. The recorder brings the demo window forward;
leave the keyboard idle during capture so physical keystrokes do not interfere
with the scripted tour. Allow a few minutes for the first run: text recognition
can take longer to initialize on a new machine.

`just mac-demo` builds a fresh Release app. To use an existing build while
iterating on the recording:

```sh
python3 macos/scripts/record-demo.py --skip-build
```

The script creates 24 short synthetic tracks with fictional artist, album,
and title tags. The app scans them normally and uses real mpv playback with
null audio output. Config, database, logs, and runtime sockets use temporary
directories. A copied app uses `io.github.sglavoie.muse.demo` for separate
desktop preferences; your installed muse.app and music database are not used.
The copy is signed ad-hoc locally, then quit and deleted after capture.

## Record on another Mac over SSH

Use a second Mac when recording would interrupt work on your main computer.
The target needs a logged-in, unlocked desktop session, the Swift command-line
tools, Python 3, `just`, `ffmpeg`, `mpv`, and either ExifTool or ffprobe. A transferred
app must support the target Mac's architecture and macOS version.

Build on the source machine without opening the UI, then transfer the app and
recorder into a fresh staging directory:

```sh
just mac-app Release
remote_host=user@macbook
remote_dir=$(ssh "$remote_host" 'mktemp -d /tmp/muse-demo-remote.XXXXXX')
rsync -aR muse macos/scripts/record-demo.py macos/scripts/demo-driver.swift \
  macos/build/muse.app "$remote_host:$remote_dir/"
ssh "$remote_host" "PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin python3 '$remote_dir/macos/scripts/record-demo.py' --check"
```

Permissions must be granted **on the target Mac**. For an ordinary SSH session,
macOS may attribute the requests to `/usr/libexec/sshd-keygen-wrapper`. If the
preflight reports missing access, add that executable in the target's relevant
privacy settings (the file chooser's `Cmd+Shift+G` accepts the full path).
This grants the permission to SSH sessions, not just this recorder.

After the remote preflight passes, capture and retrieve the artifacts:

```sh
ssh "$remote_host" "PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin python3 '$remote_dir/macos/scripts/record-demo.py' --skip-build"
rsync -a "$remote_host:$remote_dir/docs/demo/" docs/demo/
rsync -a "$remote_host:$remote_dir/macos/build/demo/" macos/build/demo/
```

The UI opens only on the target Mac. The staging directory remains available
there for troubleshooting; remove that specific directory when finished.

## Output

On success, the script writes:

- `docs/demo/muse-desktop.png`: actual desktop still linked from the README.
- `macos/build/demo/muse-desktop.gif`: local raster walkthrough.
- `macos/build/demo/muse-desktop.mp4`: local video for sharing.
- `macos/build/demo/report.json`: scene assertions and recognized screen text.
- `macos/build/demo/app.log`: diagnostics, including temporary local paths.
- `macos/build/demo/failed-scene.png`: diagnostic screenshot if a scene's text
  check fails.

Build output and reports are ignored by Git. A failed capture does not replace
the desktop still. Review the image before committing it. This recorder does
not overwrite the README's animated SVG.

The storyboard is `SCENES` in
[`record-demo.py`](../../macos/scripts/record-demo.py). Edit its keys, timings,
and expected text to change the tour. The driver captures four frames per
second; it holds each screen for readability and omits the waits between scenes.
The recording shows a small freshly scanned library, so it does not pretend
to have a long listening history or populated recommendations.

## Capture status

The 27-second native recording was captured on a MacBook over SSH on
2026-10-21. All nine scene checks passed, and the control socket confirmed a
track was playing at the end. The GIF contains 108 frames at 1200×800; the
still image retains the 2400×1600 capture resolution. The demo app exited
after recording.

The driver uses accurate CPU text recognition to avoid neural-engine startup
stalls in a command-line process. The demo launcher clears `NO_COLOR` so a
terminal or agent's output preferences do not suppress the app's theme.
