import json
from datetime import datetime, timedelta, timezone
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

import refresh_release_catalog as refresh
from refresh_release_catalog import MOS_ARTICLES, ROLES, merge_catalog, validate_review
from seed_release_catalog import sha, write_catalogs


def fixture(checked="2026-11-19T12:00:00Z"):
    return {
        "schema_version": 1,
        "product": "database",
        "as_of": "2026-11-19",
        "sources": [{"id": "source", "title": "Fixture", "checked_utc": checked}],
        "releases": [{"line": "19c", "premier_ends": "2026-12", "source": "source"}],
        "patches": [
            {"line": "19", "version": "19.32.0.0.0", "kind": "RU", "source": "source",
             "artifacts": [{"number": 12345678, "platform_id": 13, "release_date": "2026-07-21"},
                           {"number": 12345679, "platform_id": 2}]},
            {"line": "11.2.0.4", "version": "11.2.0.4.160119", "kind": "PSU", "source": "source"},
        ],
        "coverage": [{"line": "19", "source": "source", "status": "fixture"}],
        "notes": [],
    }


class ReconcileTests(unittest.TestCase):
    def test_unchanged(self):
        old = fixture()
        self.assertEqual(merge_catalog(old, old), old)

    def test_missing_patch_and_variant_keep_their_captured_source(self):
        old = fixture()
        fresh = fixture("2026-11-21T12:00:00Z")
        fresh["patches"].pop()
        fresh["patches"][0]["artifacts"].pop()
        merged = merge_catalog(old, fresh)
        archived = next(s for s in merged["sources"] if s["id"].startswith("source@"))
        self.assertEqual(archived["checked_utc"], "2026-11-19T12:00:00Z")
        self.assertEqual(merged["patches"][1]["source"], archived["id"])
        missing = merged["patches"][0]["artifacts"][1]
        self.assertEqual(missing["platform_id"], 2)
        self.assertEqual(missing["source"], archived["id"])
        self.assertEqual(merged["patches"][0]["source"], "source")
        self.assertNotIn("history", merged)
        self.assertEqual(merge_catalog(merged, fresh), merged)

    def test_corrected_values_keep_prior_observation_without_duplication(self):
        old = fixture()
        fresh = fixture("2026-11-21T12:00:00Z")
        fresh["releases"][0]["premier_ends"] = "2027-12"
        fresh["patches"][0]["artifacts"][0]["release_date"] = "2026-07-22"
        merged = merge_catalog(old, fresh)
        self.assertEqual(merged["releases"][0]["premier_ends"], "2027-12")
        self.assertEqual(merged["history"]["releases"][0]["premier_ends"], "2026-12")
        earlier = merged["history"]["patches"][0]
        self.assertEqual(earlier["artifacts"], [{"number": 12345678, "platform_id": 13,
                                                "release_date": "2026-07-21"}])
        self.assertTrue(earlier["source"].startswith("source@"))
        self.assertEqual(merge_catalog(merged, fresh), merged)

    def test_legacy_export_rename_reconciles_rows_and_archived_sources(self):
        fresh = fixture()
        current_id = "local-patch-export"
        old_id = "harvester-patch-export"
        fresh = json.loads(json.dumps(fresh).replace('"source": "source"', f'"source": "{current_id}"'))
        fresh["sources"] = [{"id": current_id, "title": "Local patch metadata export", "sha256": "a" * 64}]
        fresh["coverage"][0]["status"] = "local_patch_snapshot"
        old = json.loads(json.dumps(fresh).replace(current_id, old_id))
        old["sources"][0]["title"] = "Legacy export title"
        old["coverage"][0]["status"] = "harvester_snapshot"
        self.assertEqual(merge_catalog(old, fresh), fresh)

        # One package and one observation were recorded against a prior export.
        archive_id = old_id + "@" + "c" * 64
        old["sources"].append(dict(old["sources"][0], id=archive_id, sha256="b" * 64))
        old["patches"][0]["artifacts"][1].update(source=archive_id, corroborated_by=[old_id])
        old["history"] = {"patches": [dict(old["patches"][1], source=archive_id)]}
        fresh["patches"][0]["artifacts"].pop()
        merged = merge_catalog(old, fresh)
        archived = next(s for s in merged["sources"] if s["id"].startswith(current_id + "@"))
        self.assertEqual(archived["sha256"], "b" * 64)
        self.assertEqual(len(merged["patches"]), len(fresh["patches"]))
        self.assertEqual(merged["history"]["patches"][0]["source"], archived["id"])
        self.assertEqual(merged["patches"][0]["artifacts"][1]["source"], archived["id"])
        self.assertEqual(merged["patches"][0]["artifacts"][1]["corroborated_by"], [current_id])
        self.assertEqual(merge_catalog(merged, fresh), merged)

    def test_conflicting_duplicate_fails(self):
        old = fixture()
        fresh = fixture()
        fresh["releases"].append(dict(fresh["releases"][0], premier_ends="2039-12"))
        with self.assertRaisesRegex(ValueError, "duplicate release"):
            merge_catalog(old, fresh)


class ReviewTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name).resolve()
        self.now = datetime(2026, 11, 21, 15, tzinfo=timezone.utc)
        stamp = self.now.isoformat()
        captured = json.dumps({"generated_at": stamp}).encode()
        (self.root / "patch-catalog.json").write_bytes(captured)
        oda = json.dumps({"checked_utc": stamp, "releases": [{"status": "patch_table"}]}).encode()
        (self.root / "sources.json").write_bytes(oda)
        evidence = b"test fixture evidence"
        (self.root / "evidence.txt").write_bytes(evidence)
        articles = [{"current_id": id_, "title": "Fixture authority", "checked_utc": stamp, "sha256": sha(evidence)}
                    for id_ in MOS_ARTICLES.values()]
        manifest = json.dumps({"articles": articles}).encode()
        (self.root / "mos-capture-manifest.json").write_bytes(manifest)
        checks = []
        for role, kind in ROLES.items():
            checks.append(dict(
                role=role, title="Fixture authority", identifier=MOS_ARTICLES.get(role, "fixture-1"),
                url="https://support.oracle.com/fixture" if kind == "mos" else "https://docs.oracle.com/fixture",
                scope="fixture scope", finding="fixture finding", status="verified", checked_utc=stamp,
                evidence="evidence.txt", evidence_sha256=sha(evidence),
            ))
        self.review = dict(
            schema_version=1, version="v0.23.0", history_base="1" * 40, input_revision="2" * 40,
            reviewed_by="Fixture", reviewed_utc=stamp, patch_export_sha256=sha(captured),
            oda_manifest_sha256=sha(oda), mos_manifest_sha256=sha(manifest), checks=checks,
        )

    def verify(self):
        validate_review(self.review, "v0.23.0", self.root, self.root, self.root, self.now)

    def test_complete_review(self):
        self.verify()

    def test_missing_product(self):
        self.review["checks"].pop()
        with self.assertRaisesRegex(ValueError, "each Oracle authority"):
            self.verify()

    def test_stale_verification(self):
        self.review["checks"][0]["checked_utc"] = (self.now - timedelta(hours=25)).isoformat()
        with self.assertRaisesRegex(ValueError, "preceding 24 hours"):
            self.verify()

    def test_conflicting_source(self):
        self.review["checks"][0]["status"] = "conflicting"
        with self.assertRaisesRegex(ValueError, "not verified"):
            self.verify()

    def test_wrong_version(self):
        self.review["version"] = "v0.23.1"
        with self.assertRaisesRegex(ValueError, "requested version"):
            self.verify()

    def test_changed_patch_export(self):
        (self.root / "patch-catalog.json").write_text("{}", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "export hash"):
            self.verify()

    def test_mos_review_must_match_saved_capture(self):
        check = next(item for item in self.review["checks"] if item["role"] == "database_patches")
        check["identifier"] = "different"
        with self.assertRaisesRegex(ValueError, "current MOS capture"):
            self.verify()

    def test_changed_evidence(self):
        (self.root / "evidence.txt").write_bytes(b"changed")
        with self.assertRaisesRegex(ValueError, "source evidence hash mismatch"):
            self.verify()


    def run_publish_verifier(self, *, changed_product=None, skipped_history=False):
        base = self.review["history_base"]
        head = "3" * 40
        parent = "4" * 40
        review_file = self.root / "release-review.json"
        review_file.write_text(json.dumps(self.review), encoding="utf-8")
        earlier = [dict(fixture(), product=p) for p in refresh.PRODUCTS]
        fresh = [dict(c, as_of="2026-11-21") for c in earlier]
        expected_dir = self.root / "expected"
        write_catalogs(fresh, expected_dir)
        output_dir = self.root / "output"
        git_responses = {
            ("merge-base", "--is-ancestor", base, "HEAD"): "",
            ("for-each-ref", "--merged", "HEAD", "--format=%(refname:short)", "refs/tags"): "",
            ("rev-parse", "--show-toplevel"): str(self.root / "checkout"),
            ("rev-parse", "HEAD"): head,
            ("branch", "--show-current"): "main",
            ("status", "--porcelain=v1"): "",
            ("rev-parse", "origin/main"): head,
            ("rev-list", "--first-parent", "-1", f"{base}..{head}", "--", "releases/data"): head,
            ("rev-parse", f"{head}^1"): parent,
        }
        def committed(ref, product, *, missing_ok=False):
            if ref == head:
                data = (expected_dir / f"{product}.json").read_bytes()
                return data + b"\n" if product == changed_product else data
            previous = next(c for c in earlier if c["product"] == product)
            if skipped_history and ref == parent:
                previous = dict(previous, notes=["intervening catalog"])
            return json.dumps(previous).encode()
        args = ["refresh_release_catalog.py", "verify", "--version", "v0.23.0",
                "--review", str(review_file), "--patch-inputs", str(self.root),
                "--oda-captures", str(self.root), "--output-dir", str(output_dir)]
        with (mock.patch.object(refresh, "git", side_effect=lambda *a: git_responses[a]),
              mock.patch.object(refresh, "committed", side_effect=committed),
              mock.patch.object(refresh, "build_catalogs", return_value=fresh),
              mock.patch.object(refresh, "datetime", wraps=datetime) as clock,
              mock.patch.object(sys, "argv", args)):
            clock.now.return_value = self.now
            refresh.main()
        return json.loads((output_dir / "catalog-publish-receipt.json").read_bytes())

    def test_publish_receipt_binds_exact_commit_version_and_four_catalogs(self):
        receipt = self.run_publish_verifier()
        self.assertEqual(receipt["commit"], "3" * 40)
        self.assertEqual(receipt["version"], "v0.23.0")
        self.assertEqual(receipt["valid_until_utc"], (self.now + timedelta(hours=24)).isoformat())
        self.assertEqual(set(receipt["catalogs"]), set(refresh.PRODUCTS))
        self.assertEqual(receipt["catalogs"]["oda"], sha((self.root / "expected/oda.json").read_bytes()))

    def test_resume_checks_the_exact_tag_and_catalog_bytes(self):
        receipt = self.run_publish_verifier()
        receipt["tag_object"] = "5" * 40
        with (mock.patch.object(refresh, "git", side_effect=[receipt["tag_object"], receipt["commit"]]),
              mock.patch.object(refresh, "committed", side_effect=lambda _, p: (self.root / f"expected/{p}.json").read_bytes())):
            refresh.verify_transaction(receipt, "v0.23.0")
        with mock.patch.object(refresh, "git", return_value="6" * 40):
            with self.assertRaisesRegex(ValueError, "immutable tag differs"):
                refresh.verify_transaction(receipt, "v0.23.0")

    def test_publish_rejects_mismatched_committed_bytes(self):
        with self.assertRaisesRegex(ValueError, "oda catalog differs"):
            self.run_publish_verifier(changed_product="oda")
        self.assertFalse((self.root / "output/catalog-publish-receipt.json").exists())

    def test_publish_rejects_older_baseline_when_history_changed(self):
        with self.assertRaisesRegex(ValueError, "intervening catalog"):
            self.run_publish_verifier(skipped_history=True)
        self.assertFalse((self.root / "output/catalog-publish-receipt.json").exists())


if __name__ == "__main__":
    unittest.main()
