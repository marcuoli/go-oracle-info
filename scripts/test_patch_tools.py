"""Regression checks for Gold Image rows and AutoUpgrade archive inputs."""

from hashlib import sha256
import json
from pathlib import Path
from tempfile import TemporaryDirectory
import unittest
from unittest.mock import patch

from refresh_patch_tools import GOLD_HEADINGS, RESTRICTION, autoupgrade, gold_images


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


class AutoUpgradeArchiveTest(unittest.TestCase):
    def test_local_archive_matches_git_archive_and_requires_reviewed_bytes(self):
        build_hash = "a" * 64
        archived = (
            f'<title>Document 2485457.1</title><a title="AutoUpgrade 25.6 build version 25.6.251016 '
            f'(SHA-256 checksum {build_hash})" href="https://mosemp.us.oracle.com'
            '/epmos/main/downloadattachmentprocessor?attachid=2485457.1:AUTOUPGRADE256251016'
            '&amp;sourceId=2485457.1">20251016</a>'
        ).encode()
        commit, archive_hash = "1" * 40, sha256(archived).hexdigest()
        asset = "A" * 32
        metadata = json.dumps(dict(assetId=asset, filename="autoupgrade.jar"))
        current = (
            f"<a data-ucm-meta-data='{metadata}' "
            f'data-href="https://fixture.fa.ocs.oraclecloud.com/api/knowledge-management/v2/assets/{asset}/download">'
            'AutoUpgrade 26.6 build version 26.6.260925.jar</a>'
        )
        args = (Path(), {}, "2026-11-23T00:00:00Z")

        with (TemporaryDirectory() as directory,
              patch("refresh_patch_tools.capture", return_value=(current, {}, "")),
              patch("refresh_patch_tools.subprocess.check_output") as git):
            repo = Path(directory)
            file = repo / "au_2485457.1.html"
            file.write_bytes(archived)
            local = autoupgrade(*args, None, commit, archive_hash, archive_file=file)
            self.assertEqual([row["build"] for row in local["builds"]], ["26.6.260925", "25.6.251016"])
            self.assertEqual(local["builds"][1]["sha256"], build_hash)
            git.assert_not_called()

            with self.assertRaisesRegex(ValueError, "one local archive file or one Git archive"):
                autoupgrade(*args, repo, commit, archive_hash, archive_file=file)
            file.write_bytes(archived + b" ")
            with self.assertRaisesRegex(ValueError, "archive bytes do not match"):
                autoupgrade(*args, None, commit, archive_hash, archive_file=file)
            git.assert_not_called()

            git.return_value = archived
            self.assertEqual(autoupgrade(*args, repo, commit, archive_hash), local)
            git.assert_called_once()
            self.assertEqual(git.call_args.args[0][-1], f"{commit}:./assets/docs/au_2485457.1.html")


if __name__ == "__main__":
    unittest.main()
