"""Checks for the changelog and version handling in release.py."""

import unittest

from release import bump, parse_version, promote_changelog


CHANGELOG = """# Changelog

## [Unreleased]

### Fixed

- A bug.

## [0.2.0] - 2026-10-24

- Older.

[Unreleased]: https://github.com/sglavoie/muse/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/sglavoie/muse/compare/v0.1.0...v0.2.0
"""


class ReleaseTests(unittest.TestCase):
    def test_promote_dates_unreleased_and_updates_links(self):
        out = promote_changelog(CHANGELOG, "0.3.0", "0.2.0", "2026-10-26")
        self.assertIn("## [Unreleased]\n\n## [0.3.0] - 2026-10-26\n\n### Fixed\n", out)
        self.assertIn(
            "[Unreleased]: https://github.com/sglavoie/muse/compare/v0.3.0...HEAD\n"
            "[0.3.0]: https://github.com/sglavoie/muse/compare/v0.2.0...v0.3.0\n"
            "[0.2.0]:", out)

    def test_empty_unreleased_is_refused(self):
        empty = CHANGELOG.replace("### Fixed\n\n- A bug.\n\n", "")
        with self.assertRaisesRegex(ValueError, "empty"):
            promote_changelog(empty, "0.3.0", "0.2.0", "2026-10-26")

    def test_versions(self):
        self.assertEqual(parse_version("v1.2.3"), (1, 2, 3))
        self.assertEqual(parse_version("1.2.3"), (1, 2, 3))
        with self.assertRaises(ValueError):
            parse_version("1.2")
        self.assertEqual(bump((0, 2, 5), "patch"), (0, 2, 6))
        self.assertEqual(bump((0, 2, 5), "minor"), (0, 3, 0))
        self.assertEqual(bump((0, 2, 5), "major"), (1, 0, 0))


if __name__ == "__main__":
    unittest.main()
