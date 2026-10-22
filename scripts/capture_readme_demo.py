#!/usr/bin/env python3
"""Export a checked ANSI storyboard through muse's headless TUI harness.

Uses the real model with in-memory fixtures: no windows, music, or user state.
"""

import argparse
import json
from pathlib import Path
import subprocess


# Actions are ordinary keystrokes. Expected views/text guard each published frame.
SCENES = [
    ([], 2400, "Your library, at your fingertips", "Library", "3 tracks"),
    ([{"key": "/"}, {"text": "Alice"}], 1800, "/  filter by artist", "Library", "Alice"),
    ([{"key": "enter"}, {"key": "enter"}], 1800, "Enter  add a track to the queue", "Library", "Track A1"),
    ([{"key": "esc"}, {"key": "esc"}, {"key": "3"}], 1800, "3  see what's up next", "Queue", "Track A1"),
    ([{"key": "space"}], 1400, "Space  pause", "Queue", "Track A1"),
    ([{"key": "space"}, {"key": "4"}], 2000, "Space  resume · 4  browse artists", "Browse", "Alice"),
    ([{"key": ":"}, {"text": "equalizer"}], 2200, ":  find a command", "command_palette", "Toggle equalizer"),
    ([{"key": "esc"}, {"key": "ctrl+a"}], 2400, "Ctrl+A  tune Auto-DJ", "autodj_tune", "Discovery"),
    ([{"key": "tab"}, {"wait": "2s"}], 2400, "Tab  preview the next batch", "autodj_preview", "Draft batch"),
    ([{"key": "tab"}], 2000, "Tab  reuse a recipe", "autodj_recipes", "Late Night"),
    ([{"key": "esc"}, {"key": "1"}], 1800, "Esc · 1  back to the music", "Library", "3 tracks"),
]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--muse", type=Path, default=Path("muse"))
    parser.add_argument("--output", type=Path, default=Path("macos/build/demo/readme-frames.json"))
    args = parser.parse_args()
    requests = []
    captures = []
    for actions, hold_ms, caption, view, contains in SCENES:
        requests.extend({"type": "action", "action": action} for action in actions)
        captures.append((len(requests), hold_ms, caption, view, contains))
        requests.append({"type": "observe"})
    requests.append({"type": "quit"})
    result = subprocess.run(
        [str(args.muse.resolve()), "tui-test", "session", "--fixture", "playback", "--size", "100x26", "--ansi"],
        input="".join(json.dumps(request) + "\n" for request in requests),
        text=True, capture_output=True, check=True, timeout=60,
    )
    replies = [json.loads(line) for line in result.stdout.splitlines()]
    if len(replies) != len(requests):
        raise RuntimeError("The TUI harness returned an incomplete storyboard")
    for index, reply in enumerate(replies):
        if not reply.get("ok"):
            raise RuntimeError(f"Action {index} failed: {reply}")
        issues = reply.get("observation", {}).get("layout_issues")
        if issues:
            raise RuntimeError(f"Action {index} has layout issues: {issues}")
    frames = []
    for index, hold_ms, caption, view, contains in captures:
        obs = replies[index]["observation"]
        if obs["current_view"] != view or contains not in obs["plain_screen"]:
            raise RuntimeError(f"Unexpected scene {caption}: {obs['plain_screen']}")
        if "! errors" in obs["plain_screen"] or "file missing" in obs["plain_screen"]:
            raise RuntimeError(f"Scene {caption} contains an application error")
        if "\x1b[" not in obs["ansi_screen"]:
            raise RuntimeError(f"Scene {caption} is missing ANSI styles")
        frames.append({"ansi": obs["ansi_screen"], "hold_ms": hold_ms, "caption": caption})
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(frames, indent=2) + "\n")
    print(f"Verified {len(frames)} scenes; wrote {args.output}")


if __name__ == "__main__":
    main()
