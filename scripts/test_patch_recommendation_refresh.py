from copy import deepcopy
import unittest
from unittest.mock import patch

from refresh_patch_recommendations import build

DB = '<h3>Recommended Patches for 19.28 DB Home</h3>'
GI = '<h3>Recommended Patches for 19.28 GI Home</h3>'
HEADER = ("<tr><th>Bug</th><th>Fixed in RU</th><th>Fixed in MRP</th>"
          "<th>Description</th><th>Patches</th></tr>")
ROW = ('<tr><td>12345678</td><td>19.28</td><td>MRP 19.28.1</td>'
       '<td>Linux Only: example</td><td>'
       '<a href="/epmos/faces/ui/patch/PatchDetail.jspx?patchId=23456789">Download</a></td></tr>')


def review(rows=1):
    return {"schema_version": 1, "sources": [{"source": {}, "sections": [
        {"line": "19", "train": "19.28", "home": "database", "expected_rows": rows},
    ]}]}


class RecommendationsTest(unittest.TestCase):
    def test_linked_number_and_scope_with_explicit_restriction(self):
        with patch("refresh_patch_recommendations.capture_html", return_value=DB + "<table>" + HEADER + ROW + "</table>"):
            snapshot = build(review())
        self.assertEqual(snapshot["sections"][0]["rows"], [{
            "number": 23456789, "fixed_in_ru": "19.28", "fixed_in_mrp": "MRP 19.28.1",
            "platform_family": "Linux",
        }])

    def test_explicit_empty_and_absent_table(self):
        table = "<table>" + HEADER + "</table>"
        with patch("refresh_patch_recommendations.capture_html", return_value=DB + table):
            self.assertEqual(build(review(0))["sections"][0]["rows"], [])
        with patch("refresh_patch_recommendations.capture_html", return_value=DB + GI + table):
            with self.assertRaisesRegex(ValueError, "section was not captured"):
                build(review(0))

    def test_conflicting_numbers_and_incomplete_rows(self):
        conflict = ROW.replace("Download", "Patch 34567890")
        short = ROW.replace("<td>19.28</td>", "")
        for row, error in ((conflict, "one exact patch reference"), (short, "incomplete")):
            with self.subTest(error=error):
                with patch("refresh_patch_recommendations.capture_html", return_value=DB + "<table>" + HEADER + row + "</table>"):
                    with self.assertRaisesRegex(ValueError, error):
                        build(review())

    def test_duplicate_review_and_source_tables(self):
        duplicate_review = deepcopy(review())
        duplicate_review["sources"].append(duplicate_review["sources"][0])
        section = DB + "<table>" + HEADER + ROW + "</table>"
        with patch("refresh_patch_recommendations.capture_html", return_value=section):
            with self.assertRaisesRegex(ValueError, "duplicate or empty reviewed scope"):
                build(duplicate_review)
        with patch("refresh_patch_recommendations.capture_html", return_value=section * 2):
            with self.assertRaisesRegex(ValueError, "duplicate reviewed recommendation table"):
                build(review())
