#!/usr/bin/env python3
"""Reconcile Oracle catalog history and verify the exact commit before a module publish."""

import argparse
from collections import Counter
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import re
import subprocess
import sys
from tempfile import TemporaryDirectory

from seed_release_catalog import build_catalogs, oracle_url, read_json, sha, write_catalogs

PRODUCTS = ("database", "exadata", "oda", "supercluster")
ROLES = {
    "database_lifecycle": "public",
    "database_patches": "mos",
    "database_roadmap": "mos",
    "exadata_patches": "mos",
    "exadata_availability": "public",
    "oda": "public",
    "supercluster": "mos",
}
VERSION = re.compile(r"v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)")
COMMIT = re.compile(r"[0-9a-f]{40}")
MOS_ARTICLES = {"database_patches": "KB106822", "database_roadmap": "PNEWS1360", "exadata_patches": "KB153930"}


def key_json(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def original_id(id_):
    return id_.split("@", 1)[0]


def release_key(row):
    return tuple(row.get(field, "") for field in ("line", "version", "edition")) + (original_id(row["source"]),)


def patch_key(row):
    origin = original_id(row["source"])
    identity = tuple(row.get(field, "") for field in (
        "line", "version", "kind", "component", "source_kind", "source_name", "source_subtype", "product_name",
    )) + (origin,)
    if origin.startswith("oracle-oda-"):
        # Some ODA tables use the same description for different hardware
        # and numbered packages, even within one documented release.
        packages = {(a.get("scope", ""), a.get("number", 0)) for a in row.get("artifacts", [])}
        return (*identity, tuple(sorted(packages)))
    return identity


def artifact_key(row, source):
    platform = row.get("platform_id", row.get("platform", ""))
    return (
        platform, *(row.get(field, "") for field in ("number", "version", "component", "scope")),
        original_id(row.get("source", source)),
    )


def coverage_key(row):
    return row.get("line", ""), original_id(row["source"])


def index(rows, key, label):
    seen = {}
    for row in rows:
        ident = key(row)
        if ident in seen and seen[ident] != row:
            raise ValueError(f"conflicting duplicate {label} identity: {ident}")
        seen[ident] = row
    return seen


def remap_artifact(row, aliases):
    copy = dict(row)
    if "source" in copy:
        copy["source"] = aliases[copy["source"]]
    if "corroborated_by" in copy:
        copy["corroborated_by"] = [aliases[id_] for id_ in copy["corroborated_by"]]
    return copy


def remap_row(row, aliases):
    copy = dict(row, source=aliases[row["source"]])
    if "artifacts" in copy:
        copy["artifacts"] = [remap_artifact(a, aliases) for a in copy["artifacts"]]
    return copy


def prior_aliases(old, fresh):
    sources = index(fresh["sources"], lambda s: s["id"], "source")
    old_sources = index(old["sources"], lambda s: s["id"], "prior source")
    aliases = {}
    for id_, value in old_sources.items():
        if id_ not in sources or sources[id_] == value:
            sources[id_] = value
            aliases[id_] = id_
            continue
        # A rolling article can remove historical rows. Any retained row must
        # resolve to the source snapshot it used, including the old check date.
        pin = f"{original_id(id_)}@{sha(key_json(value).encode())}"
        archived = dict(value, id=pin)
        if pin in sources and sources[pin] != archived:
            raise ValueError(f"source snapshot collision: {pin}")
        sources[pin] = archived
        aliases[id_] = pin
    return sources, aliases


def merge_rows(old, fresh, identify, aliases, history, label):
    old = index(old, identify, f"prior {label}")
    current = index(fresh, identify, label)
    for key, before in old.items():
        after = current.get(key)
        if after is None:
            current[key] = remap_row(before, aliases)
        elif before != after:
            history.append(remap_row(before, aliases))
    return list(current.values())


def merge_patches(old, fresh, aliases, history):
    old = index(old, patch_key, "prior patch")
    current = index(fresh, patch_key, "patch")
    for key, previous in old.items():
        incoming = current.get(key)
        if incoming is None:
            current[key] = remap_row(previous, aliases)
            continue
        before = dict(previous)
        base_fields = before.pop("artifacts", [])
        after = dict(incoming)
        new_fields = after.pop("artifacts", [])
        # A missing artifact is history, even if the fresh source still has the parent release.
        prior = index(base_fields, lambda row: artifact_key(row, previous["source"]), "prior patch artifact")
        present = index(new_fields, lambda row: artifact_key(row, incoming["source"]), "patch artifact")
        overwritten = [artifact for ident, artifact in prior.items()
                       if ident in present and artifact != present[ident]]
        if before != after:
            history.append(remap_row(previous, aliases))
        elif overwritten:
            history.append(remap_row(dict(before, artifacts=overwritten), aliases))
        for artifact_id, artifact in prior.items():
            if artifact_id not in present:
                kept = remap_artifact(artifact, aliases)
                # Without its own source the historical package would inherit
                # the fresh patch's provenance.
                kept.setdefault("source", aliases[previous["source"]])
                present[artifact_id] = kept
        if present or "artifacts" in incoming:
            after["artifacts"] = list(present.values())
        current[key] = after
    return list(current.values())


def consolidate_history(entries, aliases, incoming):
    records = [remap_row(row, aliases) for row in entries] + incoming
    return list({key_json(row): row for row in records}.values())


def used_sources(catalog):
    for row in (*catalog["releases"], *catalog["patches"], *catalog["coverage"],
                *catalog.get("history", {}).get("releases", []),
                *catalog.get("history", {}).get("patches", [])):
        yield row["source"]
        for artifact in row.get("artifacts", []):
            if "source" in artifact:
                yield artifact["source"]
            yield from artifact.get("corroborated_by", [])


def merge_catalog(old, fresh):
    if old["product"] != fresh["product"] or old["schema_version"] != fresh["schema_version"]:
        raise ValueError("cannot reconcile different products or schemas")
    sources, aliases = prior_aliases(old, fresh)
    result = dict(fresh)
    release_history, patch_history = [], []
    result["releases"] = merge_rows(old["releases"], fresh["releases"], release_key, aliases,
                                    release_history, "release")
    result["patches"] = merge_patches(old["patches"], fresh["patches"], aliases, patch_history)
    previous_coverage = {coverage_key(r): remap_row(r, aliases) for r in old["coverage"]}
    previous_coverage.update(index(fresh["coverage"], coverage_key, "coverage"))
    result["coverage"] = list(previous_coverage.values())
    history = {}
    for label, additional in (("releases", release_history), ("patches", patch_history)):
        records = consolidate_history(old.get("history", {}).get(label, []), aliases, additional)
        if records:
            history[label] = records
    if history:
        result["history"] = history
    referenced = set(used_sources(result))
    if not referenced <= sources.keys():
        raise ValueError(f"unknown catalog source: {referenced - sources.keys()}")
    current = {s["id"] for s in fresh["sources"]}
    result["sources"] = [s for id_, s in sorted(sources.items()) if id_ in referenced or id_ in current]
    return result


def git(*args):
    return subprocess.check_output(["git", *args], stderr=subprocess.PIPE).decode("utf-8").strip()


def committed(ref, product, *, missing_ok=False):
    path = f"releases/data/{product}.json"
    if not git("ls-tree", "-r", "--name-only", ref, "--", path):
        if missing_ok:
            return None
        raise ValueError(f"no {product} catalog at {ref}")
    return subprocess.check_output(["git", "show", f"{ref}:{path}"])


def recent(value, now, field):
    parsed = datetime.fromisoformat(value)
    if parsed.tzinfo is None or not now - timedelta(hours=24) <= parsed <= now:
        raise ValueError(f"{field} is outside the preceding 24 hours")
    return parsed


def validate_review(review, version, docs, oda, evidence_root, now):
    if review.get("schema_version") != 1 or review.get("version") != version:
        raise ValueError("review does not match the requested version or schema")
    for field in ("history_base", "harvester_revision"):
        if not COMMIT.fullmatch(review.get(field, "")):
            raise ValueError(f"review needs a full commit ID for {field}")
    if not review.get("reviewed_by", "").strip():
        raise ValueError("reviewer is required")
    timestamps = [recent(review["reviewed_utc"], now, "review")]
    captured = (docs / "patch-catalog.json").read_bytes()
    if sha(captured) != review.get("harvester_export_sha256"):
        raise ValueError("Harvester export hash differs from the reviewed input")
    timestamps.append(recent(json.loads(captured)["generated_at"], now, "Harvester export"))
    oda_input = (oda / "sources.json").read_bytes()
    if sha(oda_input) != review.get("oda_manifest_sha256"):
        raise ValueError("ODA capture manifest differs from the reviewed input")
    oda_manifest = json.loads(oda_input)
    timestamps.append(recent(oda_manifest["checked_utc"], now, "ODA capture"))
    if any(row["status"] in {"source_unavailable", "no_matching_patch_table", "patch_page_unavailable"}
           for row in oda_manifest["releases"]):
        raise ValueError("ODA source capture is incomplete; reconcile the unavailable patch pages before publishing")
    manifest = (docs / "mos-capture-manifest.json").read_bytes()
    if sha(manifest) != review.get("harvester_manifest_sha256"):
        raise ValueError("Harvester capture manifest differs from the reviewed input")
    captures = index(json.loads(manifest)["articles"], lambda r: r["current_id"], "Harvester capture")
    checks = review["checks"]
    if Counter(x["role"] for x in checks) != Counter(ROLES.keys()):
        raise ValueError("review must cover each Oracle authority exactly once")
    for check in checks:
        role, url = check["role"], check["url"]
        if not oracle_url(url) or (ROLES[role] == "mos" and not re.match(r"^https://support\.oracle\.com/", url)):
            raise ValueError(f"unexpected authority URL for {role}")
        if any(not check.get(field, "").strip() for field in ("title", "identifier", "scope", "finding")):
            raise ValueError(f"incomplete official-source review: {role}")
        if check.get("status") != "verified":
            raise ValueError(f"source review not verified: {role}")
        timestamps.append(recent(check["checked_utc"], now, role))
        if role in MOS_ARTICLES:
            article = MOS_ARTICLES[role]
            capture = captures.get(article, {})
            if (check["identifier"] != article or check["title"] != capture.get("title")
                    or check.get("old_doc_id") != capture.get("old_doc_id")
                    or check["evidence_sha256"] != capture.get("sha256")):
                raise ValueError(f"review must match the current Harvester MOS capture for {role}")
            timestamps.append(recent(capture["checked_utc"], now, role + " capture"))
        pathname = Path(check["evidence"])
        if pathname.is_absolute() or ".." in pathname.parts:
            raise ValueError(f"evidence outside the review directory: {role}")
        resolved = (evidence_root / pathname).resolve(strict=True)
        if not resolved.is_relative_to(evidence_root) or sha(resolved.read_bytes()) != check["evidence_sha256"]:
            raise ValueError(f"source evidence hash mismatch: {role}")
    return min(timestamps) + timedelta(hours=24)


def verify_transaction(saved, version):
    if saved.get("schema_version") != 1 or saved.get("version") != version:
        raise ValueError("release transaction does not match the requested version")
    head, tag = saved.get("commit", ""), saved.get("tag_object", "")
    for item in (head, tag):
        if not COMMIT.fullmatch(item):
            raise ValueError("invalid commit or tag in the release transaction")
    if git("rev-parse", "--verify", f"refs/tags/{version}") != tag or git("rev-parse", f"{tag}^{{commit}}") != head:
        raise ValueError("local immutable tag differs from the release transaction")
    hashes = saved["catalogs"]
    if set(hashes) != set(PRODUCTS) or any(sha(committed(head, p)) != hashes[p] for p in PRODUCTS):
        raise ValueError("committed catalogs differ from the exact tagged release transaction")
    if not re.fullmatch(r"[0-9a-f]{64}", saved.get("review_sha256", "")):
        raise ValueError("release transaction does not identify its source review")
    checked = datetime.fromisoformat(saved["checked_utc"])
    deadline = datetime.fromisoformat(saved["valid_until_utc"])
    if (checked.tzinfo is None or deadline.tzinfo is None or checked > datetime.now(timezone.utc)
            or not checked <= deadline <= checked + timedelta(hours=24)):
        raise ValueError("invalid release transaction verification time")


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("mode", choices=("refresh", "verify", "resume"))
    ap.add_argument("--version", required=True)
    ap.add_argument("--review", type=Path)
    ap.add_argument("--harvester-docs", type=Path)
    ap.add_argument("--oda-captures", type=Path)
    ap.add_argument("--output-dir", type=Path)
    ap.add_argument("--transaction", type=Path)
    args = ap.parse_args()
    if not VERSION.fullmatch(args.version):
        ap.error("a stable module version such as v0.23.0 is required")
    if args.mode == "resume":
        if args.transaction is None:
            ap.error("the original release transaction is required")
        verify_transaction(read_json(args.transaction), args.version)
        return
    if not all((args.review, args.harvester_docs, args.oda_captures, args.output_dir)):
        ap.error("review, Harvester source, ODA capture and output paths are required")
    review_file = args.review.resolve(strict=True)
    raw_review = review_file.read_bytes()
    review = json.loads(raw_review)
    now = datetime.now(timezone.utc)
    valid_until = validate_review(review, args.version, args.harvester_docs, args.oda_captures, review_file.parent, now)
    base = review["history_base"]
    git("merge-base", "--is-ancestor", base, "HEAD")
    # All tagged history already shipped must be within the selected baseline.
    tags = git("for-each-ref", "--merged", "HEAD", "--format=%(refname:short)", "refs/tags")
    prior_tags = [tag for tag in tags.splitlines() if VERSION.fullmatch(tag) and tag != args.version]
    if prior_tags:
        latest = max(prior_tags, key=lambda tag: tuple(int(s) for s in tag[1:].split(".")))
        git("merge-base", "--is-ancestor", f"refs/tags/{latest}^{{commit}}", base)
    results = []
    as_of = datetime.fromisoformat(review["reviewed_utc"]).astimezone(timezone.utc).date().isoformat()
    for candidate in build_catalogs(args.harvester_docs, review["harvester_revision"], args.oda_captures,
                                    as_of, "2026-11-21"):
        product = candidate["product"]
        previous = committed(base, product, missing_ok=product == "supercluster")
        results.append(merge_catalog(json.loads(previous), candidate) if previous else candidate)
    # The reviewer commits to exact inputs; a background refresh may not
    # exchange manifests or the export while the preview is being produced.
    for path, field in (
        (args.harvester_docs / "patch-catalog.json", "harvester_export_sha256"),
        (args.harvester_docs / "mos-capture-manifest.json", "harvester_manifest_sha256"),
        (args.oda_captures / "sources.json", "oda_manifest_sha256"),
    ):
        if sha(path.read_bytes()) != review[field]:
            raise ValueError(f"reviewed input changed while building: {path.name}")
    dest = args.output_dir.resolve()
    repo = Path(git("rev-parse", "--show-toplevel")).resolve()
    if dest == repo or dest.is_relative_to(repo):
        raise ValueError("catalog refresh evidence must be staged outside the source checkout")
    if args.mode == "refresh":
        write_catalogs(results, dest)
        return
    head = git("rev-parse", "HEAD")
    if git("branch", "--show-current") != "main" or git("status", "--porcelain=v1"):
        raise ValueError("publish preflight requires the clean canonical main checkout")
    if head != git("rev-parse", "origin/main"):
        raise ValueError("canonical main must match origin/main")
    change = git("rev-list", "--first-parent", "-1", f"{base}..{head}", "--", "releases/data")
    if change:
        predecessor = git("rev-parse", f"{change}^1")
        if any(committed(base, p, missing_ok=True) != committed(predecessor, p, missing_ok=True)
               for p in PRODUCTS):
            raise ValueError("history base omits an intervening catalog; refresh again from the previous canonical snapshot")
    dest.mkdir(parents=True, exist_ok=True)
    with TemporaryDirectory(prefix="catalog-", dir=dest) as scratch:
        root = Path(scratch)
        write_catalogs(results, root)
        hashes = {}
        for product in PRODUCTS:
            raw = (root / f"{product}.json").read_bytes()
            if raw != committed(head, product):
                raise ValueError(f"{product} catalog differs from the reviewed refresh; integrate the generated data first")
            hashes[product] = sha(raw)
    receipt = dict(schema_version=1, version=args.version, commit=head, history_base=base, checked_utc=now.isoformat(),
                   review_sha256=sha(raw_review), catalogs=hashes, valid_until_utc=valid_until.isoformat())
    path = dest / "catalog-publish-receipt.json"
    path.write_text(json.dumps(receipt, indent=2) + "\n", encoding="utf-8")
    print(f"Catalog publish preflight passed for {args.version} at {head}. Receipt: {path}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as err:
        sys.exit(f"Catalog publish gate: {err}")
