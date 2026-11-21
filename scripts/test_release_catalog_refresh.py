import json
from datetime import datetime, timedelta, timezone
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

import refresh_release_catalog as refresh
from direct_catalog_sources import MOS_ARTICLES, ROLES, reviewed_catalogs, validate_review
from refresh_release_catalog import merge_catalog
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
        oda = json.dumps({"checked_utc": stamp, "releases": [{"status": "patch_table"}, {"status": "archive_index_only"}]}).encode()
        (self.root / "sources.json").write_bytes(oda)
        checks = []
        for role, (_, kind, field) in ROLES.items():
            evidence = f"visible Oracle evidence for {role}".encode()
            filename = f"{role}.html"
            (self.root / filename).write_bytes(evidence)
            identifier, old_id = MOS_ARTICLES.get(role, (f"oracle-{role}", None))
            oracle_url = "https://support.oracle.com/support/" if kind == "mos" else "https://docs.oracle.com/fixture"
            if old_id:
                oracle_url += f"?kmContentId={identifier}"
            check = dict(
                role=role, title="Fixture authority", identifier=identifier, url=oracle_url,
                scope="fixture scope", finding="fixture finding", status="verified",
                result="no_changes" if field else "oda_capture", checked_utc=stamp, captured_utc=stamp,
                evidence=filename, evidence_sha256=sha(evidence),
            )
            if old_id:
                check["old_doc_id"] = old_id
            checks.append(check)
        self.review = dict(
            schema_version=2, version="v0.23.0", history_base="1" * 40,
            reviewed_by="Fixture", reviewed_utc=stamp, oda_manifest_sha256=sha(oda), checks=checks,
        )

    def verify(self):
        return validate_review(self.review, "v0.23.0", self.root, self.root, self.now)

    def check(self, role):
        return next(item for item in self.review["checks"] if item["role"] == role)

    def test_complete_review(self):
        self.verify()

    def test_missing_product(self):
        self.review["checks"].pop()
        with self.assertRaisesRegex(ValueError, "each required Oracle authority"):
            self.verify()

    def test_stale_verification(self):
        self.review["checks"][0]["checked_utc"] = (self.now - timedelta(hours=25)).isoformat()
        with self.assertRaisesRegex(ValueError, "preceding 24 hours"):
            self.verify()

    def test_conflicting_source(self):
        self.review["checks"][0]["status"] = "conflicting"
        with self.assertRaisesRegex(ValueError, "not verified"):
            self.verify()

    def test_oda_capture_still_required_and_bound(self):
        capture = self.root / "sources.json"
        capture.write_text(json.dumps({"checked_utc": self.now.isoformat(),
                                       "releases": [{"status": "source_unavailable"}]}), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "ODA capture manifest hash mismatch"):
            self.verify()
        self.review["oda_manifest_sha256"] = sha(capture.read_bytes())
        with self.assertRaisesRegex(ValueError, "ODA source capture is incomplete"):
            self.verify()

    def test_receipt_expires_at_oldest_current_check(self):
        oldest_check = self.now - timedelta(hours=5)
        capture = self.now - timedelta(days=12)
        self.check("supercluster").update(captured_utc=capture.isoformat(), checked_utc=oldest_check.isoformat())
        expires, _ = self.verify()
        self.assertEqual(expires, oldest_check + timedelta(hours=24))

    def test_wrong_version(self):
        self.review["version"] = "v0.23.1"
        with self.assertRaisesRegex(ValueError, "requested version"):
            self.verify()

    def test_legacy_export_review_is_not_a_publish_input(self):
        self.review["schema_version"] = 1
        with self.assertRaisesRegex(ValueError, "direct Oracle evidence schema"):
            self.verify()

    def test_mos_review_requires_the_primary_article_and_legacy_id(self):
        check = self.check("database_patches")
        check["identifier"] = "KB111276"
        check["url"] = "https://support.oracle.com/support/?kmContentId=KB111276"
        with self.assertRaisesRegex(ValueError, "each required Oracle authority"):
            self.verify()
        check.update(identifier="KB106822", url="https://support.oracle.com/support/?kmContentId=888")
        check.pop("old_doc_id")
        with self.assertRaisesRegex(ValueError, "original MOS Doc ID"):
            self.verify()

    def test_mos_url_must_identify_the_reviewed_article(self):
        self.check("database_patches")["url"] = "https://support.oracle.com/support/?kmContentId=KB111276"
        with self.assertRaisesRegex(ValueError, "reviewed article"):
            self.verify()

    def test_accepts_stable_mos_document_url_and_rejects_conflicting_selectors(self):
        check = self.check("exadata_patches")
        check["url"] = "https://support.oracle.com/support/?documentId=KB153930"
        self.verify()
        check["url"] += "&kmContentId=KB111276"
        with self.assertRaisesRegex(ValueError, "reviewed article"):
            self.verify()

    def test_mos_reference_may_not_contain_session_parameters(self):
        self.check("supercluster")["url"] = "https://support.oracle.com/support/?sessionId=example"
        with self.assertRaisesRegex(ValueError, "unexpected official source"):
            self.verify()

    def test_review_cannot_reassign_the_historical_export_source(self):
        supplemental = dict(self.check("database_patches"), url="https://support.oracle.com/support/")
        self.review["checks"].append(supplemental)
        for identifier in ("local-patch-export", "harvester-patch-export"):
            with self.subTest(identifier=identifier):
                supplemental["identifier"] = identifier
                with self.assertRaisesRegex(ValueError, "unexpected official source"):
                    self.verify()

    def test_existing_mos_capture_can_be_reviewed(self):
        self.check("supercluster")["captured_utc"] = (self.now - timedelta(days=12)).isoformat()
        expires, _ = self.verify()
        self.assertEqual(expires, self.now + timedelta(hours=24))

    def test_original_capture_still_needs_offset_and_chronology(self):
        check = self.check("supercluster")
        check["captured_utc"] = self.now.replace(tzinfo=None).isoformat()
        with self.assertRaisesRegex(ValueError, "supercluster capture needs a time zone"):
            self.verify()
        check["captured_utc"] = (self.now + timedelta(seconds=1)).isoformat()
        with self.assertRaisesRegex(ValueError, "review times are out of order for supercluster"):
            self.verify()

    def test_existing_oda_capture_can_be_reviewed(self):
        capture = self.root / "sources.json"
        captured = (self.now - timedelta(days=12)).isoformat()
        capture.write_text(json.dumps({"checked_utc": captured, "releases": [{"status": "patch_table"}]}),
                           encoding="utf-8")
        self.review["oda_manifest_sha256"] = sha(capture.read_bytes())
        self.check("oda")["captured_utc"] = captured
        expires, _ = self.verify()
        self.assertEqual(expires, self.now + timedelta(hours=24))

    def test_oda_review_cannot_predate_bound_capture(self):
        earlier = (self.now - timedelta(minutes=1)).isoformat()
        self.check("oda").update(captured_utc=earlier, checked_utc=earlier)
        with self.assertRaisesRegex(ValueError, "ODA capture is newer than its review check"):
            self.verify()

    def test_requires_explicit_result_even_without_changes(self):
        del self.check("supercluster")["result"]
        with self.assertRaisesRegex(ValueError, "explicit no-change"):
            self.verify()

    def test_changed_or_missing_evidence(self):
        saved = self.root / self.check("database_patches")["evidence"]
        saved.write_bytes(b"changed")
        with self.assertRaisesRegex(ValueError, "database_patches evidence hash mismatch"):
            self.verify()
        saved.unlink()
        with self.assertRaises(FileNotFoundError):
            self.verify()

    def test_evidence_cannot_resolve_outside_review_directory(self):
        self.check("database_patches")["evidence"] = "../unrelated.html"
        with self.assertRaisesRegex(ValueError, "evidence outside"):
            self.verify()

    def test_record_cannot_assign_provenance_or_string_patch_numbers(self):
        check = self.check("database_patches")
        row = dict(line="19", version="19.32.0.0.0", kind="RU", source="local-patch-export")
        check.update(result="updated", records=[row])
        with self.assertRaisesRegex(ValueError, "unexpected catalog fields"):
            self.verify()
        del row["source"]
        row["artifacts"] = [{"number": "12345678"}]
        with self.assertRaisesRegex(ValueError, "invalid number"):
            self.verify()

    def test_verified_product_patch_joins_catalog_with_document_provenance(self):
        evidence = b"fixture release-specific MOS patch article"
        (self.root / "19c-patches.html").write_bytes(evidence)
        article = dict(self.check("database_patches"), identifier="KB111276", old_doc_id="2521164.1",
                       evidence="19c-patches.html", evidence_sha256=sha(evidence),
                       url="https://support.oracle.com/support/?kmContentId=2521164", result="updated",
                       records=[dict(line="19", version="19.33.0.0.0", kind="RU", component="GI",
                                     artifacts=[dict(number=12345678, platform_id=13)])])
        self.review["checks"].append(article)
        self.verify()
        old = fixture()
        prior = {"database": old, "exadata": None, "supercluster": None}
        incoming = reviewed_catalogs(self.review["checks"], "2026-11-21", prior)["database"]
        merged = merge_catalog(old, incoming)
        self.assertEqual(len(merged["patches"]), len(old["patches"]) + 1)
        self.assertEqual(merged["patches"][0]["source"], "KB111276")
        self.assertEqual(merged["patches"][0]["artifacts"], [dict(number=12345678, platform_id=13)])
        self.assertEqual(merged["patches"][1:], old["patches"])
        proof = next(s for s in merged["sources"] if s["id"] == "KB111276")
        self.assertEqual(proof["sha256"], article["evidence_sha256"])
        self.assertEqual(proof["checked_utc"], article["checked_utc"])
        self.assertEqual(proof["old_doc_id"], "2521164.1")

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
                "--review", str(review_file),
                "--oda-captures", str(self.root), "--output-dir", str(output_dir)]
        with (mock.patch.object(refresh, "git", side_effect=lambda *a: git_responses[a]),
              mock.patch.object(refresh, "committed", side_effect=committed),
              mock.patch.object(refresh, "oda_catalog", return_value=fresh[2]),
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
