#!/usr/bin/env python3
"""Drive and record muse.app using synthetic music and disposable XDG state."""

import argparse
import json
import os
from pathlib import Path
import plistlib
import shlex
import shutil
import signal
import socket
import sqlite3
import subprocess
import sys
import tempfile
import time


ROOT = Path(__file__).resolve().parents[2]
BUILD = ROOT / "macos/build/demo"
IDENTITY = "io.github.sglavoie.muse.demo"
ALBUMS = [
    ("The Paper Satellites", "Signals at Dusk", "Electronic", ["Amber Signal", "Night Transit", "Soft Focus", "Last Light"]),
    ("Mira Vale", "Blue Hour", "Jazz", ["Blue Hour", "Window Seat", "Slow Rain", "After the Storm"]),
    ("Northbound", "Open Roads", "Indie", ["Coastline", "Open Roads", "Daybreak", "Far From Home"]),
    ("Juniper Fields", "Small Hours", "Ambient", ["First Snow", "Still Water", "Paper Lanterns", "Quiet Company"]),
    ("Solstice Club", "Golden", "Soul", ["Golden", "Sunday Morning", "Easy Motion", "Stay Awhile"]),
    ("Glass Harbor", "Tides", "Instrumental", ["Low Tide", "Driftwood", "Northern Lights", "Homeward"]),
]
SCENES = [
    {"name": "Browse your library", "keys": [], "seconds": 3, "contains": ["Library", "Glass Harbor"]},
    {"name": "Filter by artist", "keys": ["/", "Mira"], "seconds": 3, "contains": ["Mira Vale", "Blue Hour"]},
    {"name": "Start playback", "keys": ["enter", "enter"], "seconds": 3, "contains": ["Mira Vale"]},
    {"name": "Build a queue", "keys": ["esc", "esc", "1", "enter", "down", "enter", "down", "enter", "3"], "seconds": 3, "contains": ["Queue"]},
    {"name": "Pause and resume", "keys": ["space"], "seconds": 2, "contains": ["Queue"]},
    {"name": "Browse artists", "keys": ["space", "4"], "seconds": 3, "contains": ["Browse", "Glass Harbor"]},
    {"name": "Search commands", "keys": [":", "equalizer"], "seconds": 3, "contains": ["equalizer"]},
    {"name": "Tune Auto-DJ", "keys": ["esc", ":", "tune auto", "enter"], "seconds": 4, "contains": ["Auto-DJ"]},
    {"name": "Return to the music", "keys": ["esc", "1"], "seconds": 3, "contains": ["Library", "Glass Harbor"]},
]


def run(*args, **kwargs):
    return subprocess.run([str(arg) for arg in args], check=True, **kwargs)


def prepare_library(work, mpv):
    """Only generated audio and fictional tags; never reads a user's library."""
    music = work / "music"
    music.mkdir()
    for album_index, (artist, album, genre, titles) in enumerate(ALBUMS):
        for track, title in enumerate(titles, 1):
            number = album_index * 4 + track
            # Distinct, quiet tones ensure distinct content hashes. mpv's null
            # output below silences both music and notification subprocesses.
            run("ffmpeg", "-v", "error", "-f", "lavfi", "-i",
                f"sine=frequency={180 + number * 17}:sample_rate=8000:duration={120 + number}",
                "-af", "volume=0.01", "-c:a", "flac",
                "-metadata", f"title={title}", "-metadata", f"artist={artist}",
                "-metadata", f"album={album}", "-metadata", f"genre={genre}",
                "-metadata", f"track={track}", "-metadata", "date=2026",
                music / f"{number:02d}.flac", stdout=subprocess.DEVNULL)
    environment = {}
    for kind in ("CONFIG", "DATA", "STATE", "RUNTIME", "CACHE"):
        directory = work / kind.lower()
        directory.mkdir(mode=0o700)
        environment[f"XDG_{kind}_HOME" if kind != "RUNTIME" else "XDG_RUNTIME_DIR"] = str(directory)
    config = work / "config/muse"
    config.mkdir()
    (config / "config.toml").write_text(
        f"music_directories = [{json.dumps(str(music))}]\n"
        'theme = "dark"\nnotify_on_track_change = false\ndefault_volume = 50\n'
        'mpv_extra_flags = ["--ao=null"]\n'
    )
    binaries = work / "bin"
    binaries.mkdir()
    silent_mpv = binaries / "mpv"
    silent_mpv.write_text(f'#!/bin/sh\nexec {shlex.quote(mpv)} --ao=null "$@"\n')
    silent_mpv.chmod(0o755)
    environment["PATH"] = str(binaries) + os.pathsep + os.environ["PATH"]
    wrapper = binaries / "muse-demo"
    exports = "".join(f"export {name}={shlex.quote(value)}\n" for name, value in environment.items())
    wrapper.write_text("#!/bin/sh\nset -eu\nunset NO_COLOR\n" + exports +
                       'if [ "${1-}" = "--control" ] && [ "$#" -eq 2 ]; then\n'
                       f'  printf "%s\\n" "$2" > {shlex.quote(str(work / "control-path"))}\n'
                       'fi\n' +
                       f'exec {shlex.quote(str(ROOT / "muse"))} "$@"\n')
    wrapper.chmod(0o755)
    return wrapper


def wait_for_library(process, database, expected=24):
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError("The demo app exited before scanning its library; see app.log.")
        if database.exists():
            try:
                with sqlite3.connect(database.as_uri() + "?mode=ro", uri=True) as connection:
                    if connection.execute("SELECT count(*) FROM tracks").fetchone()[0] == expected:
                        time.sleep(2)
                        return
            except sqlite3.OperationalError:
                pass  # The database or migrations may still be starting.
        time.sleep(0.5)
    raise RuntimeError("The demo library did not finish scanning within 60 seconds; see app.log.")


def stop_app(process):
    if process.poll() is not None:
        return
    # NSRunningApplication.terminate requests the app's ordinary quit/drain path.
    script = f'import AppKit; NSRunningApplication(processIdentifier: {process.pid})?.terminate()'
    subprocess.run(["swift", "-e", script], check=False, stdout=subprocess.DEVNULL, timeout=30)
    try:
        process.wait(timeout=20)
    except subprocess.TimeoutExpired:
        process.send_signal(signal.SIGTERM)
        process.wait(timeout=5)


def verify_playback(path):
    """Require actual player state as well as the driver's visual assertions."""
    with socket.socket(socket.AF_UNIX) as connection:
        connection.settimeout(10)
        connection.connect(str(path))
        connection.sendall(b'{"id":1,"op":"state"}\n')
        with connection.makefile("r") as replies:
            for line in replies:
                reply = json.loads(line)
                if reply.get("reply") == 1:
                    state = reply.get("state", {})
                    if not reply.get("ok") or state.get("playback") != "playing" or not state.get("track"):
                        raise RuntimeError(f"Demo did not leave a track playing: {state}")
                    return state
    raise RuntimeError("Demo control socket closed without reporting playback state.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="only check dependencies and macOS permissions")
    parser.add_argument("--skip-build", action="store_true", help="use the existing muse binary and macos/build/muse.app")
    args = parser.parse_args()
    if sys.platform != "darwin":
        parser.error("recording the desktop app requires macOS 14 or later")
    for tool in ("swiftc", "ffmpeg", "mpv", "just", "codesign"):
        if not shutil.which(tool):
            parser.error(f"missing required tool: {tool}")
    BUILD.mkdir(parents=True, exist_ok=True)
    driver = BUILD / "demo-driver"
    run("swiftc", "-parse-as-library", ROOT / "macos/scripts/demo-driver.swift", "-o", driver)
    run(driver, "--check")
    if args.check:
        return
    if not args.skip_build:
        run("just", "mac-app", "Release", cwd=ROOT)
    source = ROOT / "macos/build/muse.app"
    if not source.is_dir() or not (ROOT / "muse").is_file():
        parser.error("build first with just mac-app Release")
    # Short path matters: the app's Unix control socket has a 104-byte limit.
    with tempfile.TemporaryDirectory(prefix="muse-demo-", dir="/tmp") as directory:
        work = Path(directory)
        wrapper = prepare_library(work, shutil.which("mpv"))
        app = work / "muse Demo.app"
        run("ditto", source, app)
        info_path = app / "Contents/Info.plist"
        with info_path.open("rb") as file:
            info = plistlib.load(file)
        info["CFBundleIdentifier"] = IDENTITY
        info["CFBundleName"] = "muse Demo"
        with info_path.open("wb") as file:
            plistlib.dump(info, file)
        run("codesign", "--force", "--sign", "-", app)
        scenes = work / "scenes.json"
        scenes.write_text(json.dumps(SCENES))
        frames = work / "frames"
        with (BUILD / "app.log").open("w") as log:
            process = subprocess.Popen([
                str(app / "Contents/MacOS/muse"), "-museBinaryPath", str(wrapper),
                "-museNotifications", "NO", "-museHotKey", "", "-museDefaultFontSize", "15",
                "-museActivationPolicy", "regular",
            ], env={**os.environ, "TMPDIR": str(work) + "/"}, stdout=log, stderr=log)
            try:
                wait_for_library(process, work / "data/muse/muse.db")
                run(driver, process.pid, scenes, frames, BUILD / "report.json", timeout=180)
                # Foundation may choose a per-app temp directory rather than
                # TMPDIR. The launch wrapper records the app's actual argument.
                state = verify_playback(Path((work / "control-path").read_text().strip()))
                report_path = BUILD / "report.json"
                report = json.loads(report_path.read_text())
                report["playback"] = state
                report_path.write_text(json.dumps(report, indent=2) + "\n")
                # Keep raster animation in ignored build output; the README uses SVG.
                run("ffmpeg", "-v", "error", "-y", "-framerate", "4", "-i", frames / "%04d.png",
                    "-vf", "scale=1200:800:flags=lanczos,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=3",
                    "-loop", "0", BUILD / "muse-desktop.gif")
                run("ffmpeg", "-v", "error", "-y", "-framerate", "4", "-i", frames / "%04d.png",
                    "-vf", "scale=1200:800:flags=lanczos", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", BUILD / "muse-desktop.mp4")
                output = ROOT / "docs/demo"
                output.mkdir(parents=True, exist_ok=True)
                shutil.copy2(frames / "0000.png", output / "muse-desktop.png")
                print(f"Demo written to {output}; MP4 and verification report are in {BUILD}.")
            finally:
                stop_app(process)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, subprocess.CalledProcessError, subprocess.TimeoutExpired) as error:
        sys.exit(f"record-demo: {error}")
