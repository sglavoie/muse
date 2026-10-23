"""Regression checks for CI coverage and prompt failure propagation."""

import contextlib
import io
import json
from pathlib import Path
import re
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

from ci_test import run_tests, shard_args, split_tests


class ShardTests(unittest.TestCase):
    def test_tui_shards_cover_every_test_once(self):
        names = ["TestAlpha", "TestBeta", "TestNewRegression", "ExampleThing", "FuzzInput"]
        listing = "\n".join([*names, "ok module/internal/tui 0.01s", "BenchmarkRender"])
        shards = [{name for name in names if re.fullmatch(split_tests(listing, i), name)}
                  for i in range(2)]
        self.assertEqual(shards[0] | shards[1], set(names))
        self.assertFalse(shards[0] & shards[1])
        self.assertFalse(re.fullmatch(split_tests(listing, 0), "TestAlphaExtra"))

    def test_empty_shard_is_an_error(self):
        with self.assertRaisesRegex(ValueError, "no tests"):
            split_tests("ok module/internal/tui 0.01s", 0)

    def test_core_automatically_includes_new_packages_and_tui_children(self):
        packages = ["module", "module/internal/tui", "module/internal/tui/uitest",
                    "module/internal/control", "module/internal/new_package"]
        with patch("ci_test.subprocess.check_output", side_effect=["module\n", "\n".join(packages)]):
            core = shard_args("core")
        with patch("ci_test.subprocess.check_output", return_value="module\n"):
            cli = shard_args("cli")
        self.assertEqual(set(core + cli + ["module/internal/tui"]), set(packages))
        self.assertEqual(len(core + cli + ["module/internal/tui"]), len(packages))


class RunnerTests(unittest.TestCase):
    def run_script(self, source, log):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            result = run_tests([sys.executable, "-u", "-c", source], log)
        return result, output.getvalue()

    def test_success_preserves_timings_and_log(self):
        event = {"Action": "pass", "Package": "pkg", "Test": "TestOK", "Elapsed": 0.25}
        with tempfile.TemporaryDirectory() as directory:
            log = Path(directory) / "events.jsonl"
            result, output = self.run_script(f"print({json.dumps(event)!r})", log)
            self.assertEqual(result, 0)
            self.assertEqual(json.loads(log.read_text()), event)
            self.assertIn("0.25s pkg TestOK", output)

    def test_failure_stops_other_packages_and_their_child_processes(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / "survived"
            ready = Path(directory) / "ready"
            # A stubborn descendant simulates a test subprocess which outlives
            # `go test` and ignores SIGTERM. The whole process group must die.
            child = (
                "import signal,time; from pathlib import Path; "
                "signal.signal(signal.SIGTERM, signal.SIG_IGN); "
                f"Path({str(ready)!r}).touch(); time.sleep(0.5); "
                f"Path({str(marker)!r}).touch(); time.sleep(30)"
            )
            source = f"""
import json, subprocess, sys, time
from pathlib import Path
subprocess.Popen([sys.executable, '-c', {child!r}])
while not Path({str(ready)!r}).exists():
    time.sleep(0.001)
print(json.dumps({{'Action': 'output', 'Package': 'pkg', 'Test': 'TestBroken', 'Output': 'assertion detail\\n'}}), flush=True)
print(json.dumps({{'Action': 'fail', 'Package': 'pkg', 'Test': 'TestBroken'}}), flush=True)
time.sleep(30)
"""
            start = time.monotonic()
            result, output = self.run_script(source, Path(directory) / "events.jsonl")
            self.assertEqual(result, 1)
            self.assertLess(time.monotonic() - start, 5)
            self.assertIn("assertion detail", output)
            self.assertIn("stopping remaining tests", output)
            time.sleep(0.6)
            self.assertFalse(marker.exists(), "test descendant survived cancellation")

    def test_build_failure_and_plain_nonzero_exit_fail(self):
        sources = [
            "import sys; print('compile error'); sys.exit(2)",
            "import json,time; print(json.dumps({'Action': 'build-fail', 'ImportPath': 'pkg'}), flush=True); time.sleep(30)",
        ]
        with tempfile.TemporaryDirectory() as directory:
            for source in sources:
                with self.subTest(source=source):
                    start = time.monotonic()
                    result, _ = self.run_script(source, Path(directory) / "events.jsonl")
                    self.assertNotEqual(result, 0)
                    self.assertLess(time.monotonic() - start, 5)


if __name__ == "__main__":
    unittest.main()
