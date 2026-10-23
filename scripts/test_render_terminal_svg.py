# /// script
# requires-python = ">=3.10"
# dependencies = ["rich==14.2.0"]
# ///
"""Regression checks for the visual review renderer's terminal cell geometry."""
import re
import unittest
from xml.etree import ElementTree

from render_terminal_svg import export


class TerminalSVGTests(unittest.TestCase):
    def test_wide_and_combining_runs_use_display_cells(self):
        for value, cells in [("AB", 2), ("全角", 4), ("e\u0301", 1), ("🎵", 2)]:
            with self.subTest(value=value):
                svg = export(value, width=60)
                root = ElementTree.fromstring(svg)
                run = next(node for node in root.iter() if node.tag.endswith("}text") and node.text == value)
                self.assertAlmostEqual(float(run.attrib["textLength"]), 12.2 * cells)

    def test_review_width_controls_canvas(self):
        narrow = export("song", width=60)
        wide = export("song", width=80)
        def width(svg):
            return float(re.search(r'viewBox="0 0 ([\d.]+)', svg).group(1))
        self.assertAlmostEqual(width(wide) - width(narrow), 20 * 12.2)


if __name__ == "__main__":
    unittest.main()
