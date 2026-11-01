"""Checks for the license-notice generator, third_party_notices.py."""

from pathlib import Path
import tempfile
import unittest

import third_party_notices as tpn


class LicenseFileTests(unittest.TestCase):
    def test_picks_license_like_files_only(self):
        with tempfile.TemporaryDirectory() as d:
            for name in ["LICENSE", "LICENSE-3RD-PARTY.md", "COPYING", "NOTICE.txt", "PATENTS",
                         "license.md", "README.md", "licenses.go", "LICENSING"]:
                Path(d, name).write_text("x")
            Path(d, "LICENSE.d").mkdir()
            got = [p.name for p in tpn.license_files(d)]
        self.assertEqual(got, ["COPYING", "LICENSE", "LICENSE-3RD-PARTY.md", "license.md", "NOTICE.txt", "PATENTS"])


class SectionTests(unittest.TestCase):
    def test_section_has_title_url_note_and_text(self):
        with tempfile.TemporaryDirectory() as d:
            f = Path(d, "LICENSE")
            f.write_text("\nMIT License\r\nCopyright\n\n")
            lines = tpn.section("pkg v1", "https://example.org", [f], "a note")
        self.assertEqual(lines, [tpn.RULE, "pkg v1", "https://example.org", tpn.RULE, "",
                                 "a note", "", "MIT License\nCopyright", ""])


class ManifestTests(unittest.TestCase):
    def test_vendored_texts_exist(self):
        for c in tpn.GHOSTTY_COMPONENTS:
            for ref in c.files:
                if ref.startswith("vendored:"):
                    self.assertTrue(tpn.resolve(ref).is_file(), ref)

    def test_every_reference_has_a_known_kind(self):
        for c in tpn.GHOSTTY_COMPONENTS:
            self.assertTrue(c.files, c.name)
            for ref in c.files:
                self.assertIn(ref.partition(":")[0], {"ghostty", "zig", "vendored"}, ref)


if __name__ == "__main__":
    unittest.main()
