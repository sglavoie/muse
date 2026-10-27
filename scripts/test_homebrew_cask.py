"""Checks for the Homebrew cask that homebrew_cask.py writes."""

import unittest

from homebrew_cask import render


DARWIN = "a" * 64
ARM64 = "b" * 64
AMD64 = "c" * 64
CHECKSUMS = f"""\
{DARWIN}  muse_0.5.0_darwin_all.tar.gz
{ARM64}  muse_0.5.0_linux_arm64.tar.gz
{AMD64}  muse_0.5.0_linux_amd64.tar.gz
{"d" * 64}  muse_0.4.0_linux_amd64.tar.gz
"""


class HomebrewCaskTests(unittest.TestCase):
    def test_token_avoids_homebrew_cask_muse(self):
        out = render("0.5.0", CHECKSUMS)
        self.assertIn('cask "muse-player" do\n  version "0.5.0"\n', out)

    def test_each_platform_gets_its_archive_checksum(self):
        out = render("0.5.0", CHECKSUMS)
        self.assertIn(f'sha256 "{DARWIN}"\n\n    url "https://github.com/sglavoie/muse'
                      '/releases/download/v#{version}/muse_#{version}_darwin_all.tar.gz"', out)
        self.assertIn(f'sha256 "{ARM64}"\n\n      url', out)
        self.assertIn(f'sha256 "{AMD64}"\n\n      url', out)
        self.assertNotIn("d" * 64, out)

    def test_macos_links_the_notarized_muse_inside_the_app(self):
        out = render("0.5.0", CHECKSUMS)
        self.assertIn('app "muse.app"\n'
                      '    binary "#{appdir}/muse.app/Contents/Resources/bin/muse"\n', out)
        self.assertNotIn("quarantine", out)

    def test_missing_archive_is_refused(self):
        partial = CHECKSUMS.replace("darwin_all", "darwin_arm64")
        with self.assertRaisesRegex(ValueError, "muse_0.5.0_darwin_all.tar.gz"):
            render("0.5.0", partial)

    def test_tag_with_v_is_refused(self):
        with self.assertRaisesRegex(ValueError, "no leading v"):
            render("v0.5.0", CHECKSUMS)


if __name__ == "__main__":
    unittest.main()
