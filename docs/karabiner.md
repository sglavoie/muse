# Karabiner-Elements Integration (macOS)

Control muse playback with global hotkeys via [Karabiner-Elements](https://karabiner-elements.pqrs.org/).

## Prerequisites

- `muse` on your PATH (verify with `which muse`)
- A caps lock override rule (remaps caps lock to a variable for use as a modifier)

## Karabiner Rule

Add this to your `karabiner.json` complex modifications. Adjust the key codes and modifier conditions to match your setup. Use absolute path to `muse` if needed.

```json
{
    "description": "Capslock override & caps lock toggle replacement (caps+\\) (required by all other groups)",
    "manipulators": [
        {
            "from": {
                "key_code": "caps_lock",
                "modifiers": { "optional": ["any"] }
            },
            "to": [
                {
                    "set_variable": {
                        "name": "caps_lock pressed",
                        "value": 1
                    }
                }
            ],
            "to_after_key_up": [
                {
                    "set_variable": {
                        "name": "caps_lock pressed",
                        "value": 0
                    }
                }
            ],
            "type": "basic"
        },
        {
            "conditions": [
                {
                    "name": "caps_lock pressed",
                    "type": "variable_if",
                    "value": 1
                }
            ],
            "from": {
                "key_code": "backslash",
                "modifiers": { "optional": ["any"] }
            },
            "to": [{ "key_code": "caps_lock" }],
            "type": "basic"
        },
        {
            "conditions": [
                {
                    "name": "caps_lock pressed",
                    "type": "variable_if",
                    "value": 1
                }
            ],
            "from": {
                "key_code": "f11",
                "modifiers": { "optional": ["any"] }
            },
            "to": [{ "shell_command": "/Users/sglavoie/.go/bin/muse volume-down" }],
            "type": "basic"
        },
        {
            "conditions": [
                {
                    "name": "caps_lock pressed",
                    "type": "variable_if",
                    "value": 1
                }
            ],
            "from": {
                "key_code": "f12",
                "modifiers": { "optional": ["any"] }
            },
            "to": [{ "shell_command": "/Users/sglavoie/.go/bin/muse volume-up" }],
            "type": "basic"
        }
    ]
}
```

```json
{
    "description": "Fn+media keys → muse playback control",
    "manipulators": [
        {
            "from": {
                "key_code": "f6",
                "modifiers": {
                    "mandatory": ["fn"],
                    "optional": ["any"]
                }
            },
            "to": [{ "shell_command": "/Users/sglavoie/.go/bin/muse now-playing" }],
            "type": "basic"
        },
        {
            "from": {
                "key_code": "f7",
                "modifiers": {
                    "mandatory": ["fn"],
                    "optional": ["any"]
                }
            },
            "to": [{ "shell_command": "/Users/sglavoie/.go/bin/muse previous" }],
            "type": "basic"
        },
        {
            "from": {
                "key_code": "f8",
                "modifiers": {
                    "mandatory": ["fn"],
                    "optional": ["any"]
                }
            },
            "to": [{ "shell_command": "/Users/sglavoie/.go/bin/muse toggle-play" }],
            "type": "basic"
        },
        {
            "from": {
                "key_code": "f9",
                "modifiers": {
                    "mandatory": ["fn"],
                    "optional": ["any"]
                }
            },
            "to": [{ "shell_command": "/Users/sglavoie/.go/bin/muse next" }],
            "type": "basic"
        },
        {
            "from": {
                "key_code": "f10",
                "modifiers": {
                    "mandatory": ["fn"],
                    "optional": ["any"]
                }
            },
            "to": [{ "consumer_key_code": "mute" }],
            "type": "basic"
        }
    ]
}
```

## Available Commands

```bash
muse --help
```

## Troubleshooting

- **Keys not working?** Use `kitty +kitten show_key` or Karabiner-EventViewer to verify the key codes your keyboard sends.
- **Shell command not found?** Use the full path in `shell_command` (e.g. `/usr/local/bin/muse next`).
- **muse not running?** The playback commands (`previous`, `toggle-play`, `next`, `volume-up`, `volume-down`) require a running muse instance; they connect to the `muse.ctrl` Unix domain socket in the runtime directory and exit non-zero when nothing is listening.
- **`now-playing` is different.** It reads mpv's IPC socket and the library database directly rather than going through `muse.ctrl`, so it reports the current track even when the TUI is busy and never needs the TUI to answer. It notifies via `alerter` (`/opt/homebrew/bin/alerter`), replacing any previous banner rather than stacking, and prints the same summary to stdout.
- **Want it automatic?** The TUI can post the same banner on every track change on its own: turn on **Track notify** in the Display tab of the settings overlay (`ctrl+,`), or run `Toggle track notifications` from the command palette. The manual fn+F6 binding stays useful for checking what is playing on demand.
