from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--test", action="store_true")
    args = parser.parse_args()

    if not args.test:
        parser.error("the following arguments are required: --test")

    repo_root = Path(__file__).resolve().parent.parent
    completed = subprocess.run(["just", "test"], cwd=repo_root)
    return completed.returncode


if __name__ == "__main__":
    sys.exit(main())
