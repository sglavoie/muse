# /// script
# requires-python = ">=3.10"
# dependencies = ["rich==14.2.0"]
# ///
"""Build a checked visual gallery from fixture scenarios; never opens real state."""

import argparse
import html
import json
from pathlib import Path
import subprocess

from render_terminal_svg import export

ROOT = Path(__file__).resolve().parents[1]
SCENARIOS = ("dense-visual-review", "inline-editor-polish", "spare-space-identity")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--muse", type=Path, default=ROOT / "muse")
    parser.add_argument("--output", type=Path, default=ROOT / "macos/build/ui-review")
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    cards = []
    for name in SCENARIOS:
        scenario_path = ROOT / "docs/tui-testing/scenarios" / f"{name}.json"
        scenario = json.loads(scenario_path.read_text())
        result = subprocess.run(
            [str(args.muse.resolve()), "tui-test", "run", "--scenario", str(scenario_path),
             "--json", "--observations", "--ansi"],
            text=True, capture_output=True, check=False, timeout=90,
        )
        if result.returncode:
            try:
                failure = json.loads(result.stdout).get("error", {})
                detail = f"{failure.get('label', '')} action {failure.get('index')}: {failure.get('reason')}"
                screen = failure.get("observation", {}).get("plain_screen", "")
            except json.JSONDecodeError:
                detail, screen = result.stderr, ""
            raise RuntimeError(f"{name}: {detail}\n{screen}")
        report = json.loads(result.stdout)
        if not report["ok"] or len(report["observations"]) != len(scenario["actions"]):
            raise RuntimeError(f"{name}: incomplete review")
        (args.output / f"{name}.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        for index, (action, obs) in enumerate(zip(scenario["actions"], report["observations"])):
            if obs.get("layout_issues"):
                raise RuntimeError(f"{name} action {index}: {obs['layout_issues']}")
            if not action.get("label"):
                continue
            label = action["label"]
            filename = f"{name}-{index:03}.svg"
            svg = export(obs["ansi_screen"], unique_id=f"review-{name}-{index}", width=obs["width"])
            (args.output / filename).write_text(svg)
            title = html.escape(f"{label} · {obs['width']}×{obs['height']}")
            cards.append(f'<article><h2>{title}</h2><a href="{filename}"><img src="{filename}" alt="{title}"></a></article>')
    page = '''<!doctype html><html lang="en"><meta charset="utf-8">
<title>muse UI review</title><style>
body{font:16px system-ui,sans-serif;background:#11151d;color:#e2e8f0;margin:24px}
h1{font-size:24px}h2{font-size:16px}main{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,620px),1fr));gap:24px}
article{min-width:0}img{width:100%;height:auto}a{color:#8cb4ff}
</style><h1>muse UI review</h1>
<p>Compare adjacent sizes, song identity, editing hints, focus and selection, and playback with notifications.
Click a frame to inspect it at full size. JSON reports retain ANSI and plain-text observations.</p><main>'''
    (args.output / "index.html").write_text(page + "\n".join(cards) + "</main></html>\n")
    print(f"Verified {len(SCENARIOS)} scenarios; wrote {len(cards)} frames to {args.output / 'index.html'}")


if __name__ == "__main__":
    main()
