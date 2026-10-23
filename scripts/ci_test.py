#!/usr/bin/env python3
"""Run race tests with live failures, process-group cancellation and CI shards."""

import argparse
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys


SHARDS = ("core", "cli", "tui-0", "tui-1")


def split_tests(output, index):
    # Include examples and fuzz seed corpora, just like an ordinary go test.
    names = sorted(line for line in output.splitlines()
                   if re.fullmatch(r"(?:Test|Example|Fuzz)\w*", line))
    selected = names[index::2]
    if not selected:
        raise ValueError("TUI shard contains no tests")
    return "^(" + "|".join(selected) + ")$"


def shard_args(shard):
    module = subprocess.check_output(["go", "list", "-m"], text=True).strip()
    tui = module + "/internal/tui"
    if shard == "cli":
        return [module]
    if shard.startswith("tui-"):
        # Match the actual run's build mode so listing warms its compile cache.
        output = subprocess.check_output(
            ["go", "test", "-race", "-list", ".", tui], text=True)
        return ["-run", split_tests(output, int(shard[-1])), tui]
    packages = subprocess.check_output(["go", "list", "./..."], text=True)
    return [p for p in packages.splitlines() if p not in (module, tui)]


def stop_processes(proc):
    # Killing only `go test` leaves already-running test binaries alive.
    try:
        try:
            os.killpg(proc.pid, signal.SIGTERM)
        except ProcessLookupError:
            return
        try:
            proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            pass
    finally:
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        proc.wait()


def run_tests(command, output_path):
    """Save all JSON events, print failures immediately, stop on the first one."""
    buffers = {}
    timings = []
    failed = False
    with Path(output_path).open("w") as log:
        with subprocess.Popen(command, stdout=subprocess.PIPE,
                              stderr=subprocess.STDOUT, text=True,
                              start_new_session=True) as proc:
            try:
                for line in proc.stdout:
                    log.write(line)
                    log.flush()
                    try:
                        event = json.loads(line)
                    except json.JSONDecodeError:
                        print(line, end="", flush=True)
                        continue
                    action = event.get("Action")
                    package = event.get("Package", "")
                    test = event.get("Test", "")
                    key = (package, test)
                    if action == "output":
                        if test:
                            buffers.setdefault(key, []).append(event["Output"])
                        else:
                            print(event["Output"], end="", flush=True)
                    if action in ("pass", "skip", "fail", "build-fail"):
                        output = buffers.pop(key, [])
                        if test and "/" not in test:
                            timings.append((event.get("Elapsed", 0), package, test))
                        if action in ("fail", "build-fail"):
                            print("".join(output), end="", flush=True)
                            print(f"FAIL {package} {test}; stopping remaining tests", flush=True)
                            failed = True
                            break
                if failed:
                    stop_processes(proc)
                result = proc.wait()
            finally:
                if proc.poll() is None:
                    stop_processes(proc)
    print("Slowest completed top-level tests:", flush=True)
    for elapsed, package, test in sorted(timings, reverse=True)[:10]:
        print(f"{elapsed:7.2f}s {package} {test}", flush=True)
    return 1 if failed else (result if result >= 0 else 1)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--shard", choices=SHARDS)
    parser.add_argument("--json-output", required=True)
    args = parser.parse_args()
    selection = shard_args(args.shard) if args.shard else ["./..."]
    return run_tests(
        ["go", "test", "-race", "-count=1", "-failfast", "-timeout=5m", "-json", *selection],
        args.json_output,
    )


if __name__ == "__main__":
    sys.exit(main())
