"""Regression checks for matching Gold Image links within their own rows."""

from pathlib import Path
import unittest
from unittest.mock import patch

from refresh_patch_tools import GOLD_HEADINGS, RESTRICTION, gold_images


class GoldImageRowsTest(unittest.TestCase):
    @staticmethod
    def article(cross_row):
        def link(number):
            return f'<a href="https://support.oracle.com/support/?patchId={number}">{number}</a>'

        headings = []
        for label, home in GOLD_HEADINGS.items():
            client = home == "client"
            header = "Download Link (all platforms)" if client else "Download Link"
            scope = "" if client else RESTRICTION
            qualifier = {"client": "", "database": " DB RU Goldimage",
                         "grid_infrastructure": " GI RU Goldimage"}[home]
            first, second = link(70000001), link(70000002)
            if cross_row and home == "database":
                # The second link used to be paired with the second row even
                # though the row did not have its own Oracle link.
                first = '<a href="https://support.oracle.com/support/?patchId=70000001">70000001</a>' \
                        '<a href="https://support.oracle.com/support/?patchId=70000002"></a>'
                second = "70000002"
            headings.append(f"""
                <h2>{label}</h2><p>{scope}</p>
                <table>
                    <tr><th>Release Date</th><th>Version</th><th>{header}</th></tr>
                    <tr><td>2026-10-20</td><td>23.26.4.0.0{qualifier}</td><td>{first}</td></tr>
                    <tr><td>2026-07-21</td><td>23.26.3.0.0{qualifier}</td><td>{second}</td></tr>
                </table>
            """)
        return "".join(headings)

    def test_link_must_be_in_its_patch_row(self):
        with patch("refresh_patch_tools.capture", return_value=(self.article(False), {}, "")):
            data = gold_images(Path(), {})
        self.assertEqual(len(data["sections"]), 6)

        with patch("refresh_patch_tools.capture", return_value=(self.article(True), {}, "")):
            with self.assertRaisesRegex(ValueError, "patch link per row"):
                gold_images(Path(), {})


if __name__ == "__main__":
    unittest.main()
