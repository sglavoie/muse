#!/usr/bin/env python3
"""Cut a release: date the changelog, commit, tag, and push the tag to GitHub.

Pushing a v* tag runs .github/workflows/release.yml, which re-runs CI and then
GoReleaser (binaries, GitHub release), then writes the Homebrew cask. See CONTRIBUTING.md.
"""

import argparse
import datetime
from pathlib import Path
import re
import subprocess
import sys


CHANGELOG = Path("CHANGELOG.md")
REPO_URL = "https://github.com/sglavoie/muse"
REMOTE = "origin"
SEMVER = re.compile(r"v?(\d+)\.(\d+)\.(\d+)")


def parse_version(text):
    match = SEMVER.fullmatch(text.strip())
    if not match:
        raise ValueError(f"not a version like 1.2.3: {text!r}")
    return tuple(int(part) for part in match.groups())


def bump(previous, part):
    major, minor, patch = previous
    if part == "major":
        return (major + 1, 0, 0)
    if part == "minor":
        return (major, minor + 1, 0)
    return (major, minor, patch + 1)


def fmt(version):
    return ".".join(str(part) for part in version)


def promote_changelog(text, version, previous, date):
    """Move the Unreleased entries under a dated section and update the links."""
    heading = "## [Unreleased]\n"
    start = text.index(heading) + len(heading)
    next_section = text.find("\n## [", start)
    if not text[start:next_section].strip():
        raise ValueError("the Unreleased section of CHANGELOG.md is empty")
    text = text[:start] + f"\n## [{version}] - {date}\n" + text[start:]
    old_link = f"[Unreleased]: {REPO_URL}/compare/v{previous}...HEAD\n"
    if old_link not in text:
        raise ValueError(f"CHANGELOG.md has no link line {old_link.strip()!r}")
    new_links = (f"[Unreleased]: {REPO_URL}/compare/v{version}...HEAD\n"
                 f"[{version}]: {REPO_URL}/compare/v{previous}...v{version}\n")
    return text.replace(old_link, new_links)


def git(*args, capture=True):
    result = subprocess.run(["git", *args], check=True, text=True,
                            capture_output=capture)
    return result.stdout.strip() if capture else ""


def latest_tag():
    tags = git("tag", "--list", "v*", "--sort=-v:refname").splitlines()
    if not tags:
        raise SystemExit("no v* tag to release after")
    return parse_version(tags[0])


def check_repo():
    if git("rev-parse", "--abbrev-ref", "HEAD") != "main":
        raise SystemExit("release from main")
    if git("status", "--porcelain"):
        raise SystemExit("the working tree has uncommitted changes")
    git("fetch", "--quiet", REMOTE, "main")
    if git("rev-list", "--count", f"HEAD..{REMOTE}/main") != "0":
        raise SystemExit(f"main is behind {REMOTE}/main; pull first")


def confirm(prompt):
    return input(f"{prompt} [y/N] ").strip().lower() in ("y", "yes")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("version", help="1.2.3, or patch, minor or major to bump the latest tag")
    parser.add_argument("--summary", default="",
                        help="a paragraph for the tag message under 'muse vX.Y.Z'")
    parser.add_argument("--yes", action="store_true", help="push without asking")
    args = parser.parse_args(argv)

    previous = latest_tag()
    if args.version in ("patch", "minor", "major"):
        version = bump(previous, args.version)
    else:
        version = parse_version(args.version)
    if version <= previous:
        raise SystemExit(f"v{fmt(version)} is not newer than v{fmt(previous)}")
    tag = f"v{fmt(version)}"

    check_repo()
    today = datetime.date.today().isoformat()
    try:
        updated = promote_changelog(CHANGELOG.read_text(), fmt(version), fmt(previous), today)
    except ValueError as err:
        raise SystemExit(str(err))

    print(f"Releasing {tag} (previous v{fmt(previous)}). Commits since then:")
    print(git("log", "--oneline", f"v{fmt(previous)}..HEAD"))
    if not (args.yes or confirm(f"Date the changelog, commit and tag {tag}?")):
        raise SystemExit("aborted; nothing changed")

    CHANGELOG.write_text(updated)
    git("commit", "--quiet", "-m", f"chore: release {tag}", "--", str(CHANGELOG))
    message = f"muse {tag}\n\n"
    if args.summary:
        message += args.summary.strip() + " "
    message += f"See CHANGELOG.md for the changes since v{fmt(previous)}."
    git("tag", "-a", tag, "-m", message)
    print(f"Committed and tagged {tag}.")

    if not (args.yes or confirm(f"Push main and {tag} to {REMOTE}, starting the release?")):
        print(f"Not pushed. Later: git push --atomic {REMOTE} main {tag}")
        print(f"Or undo: git tag -d {tag} && git reset --hard HEAD~1")
        return
    git("push", "--atomic", REMOTE, "main", tag, capture=False)
    print(f"Pushed. Follow the release with: gh run watch $(gh run list -w Release -L1 --json databaseId -q '.[0].databaseId')")


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as err:
        sys.exit(f"{' '.join(err.cmd)} failed:\n{err.stderr or ''}")
