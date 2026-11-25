#!/usr/bin/env python3
"""Reconcile Oracle catalog history and verify the exact commit before a module publish."""

import argparse
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import re
import subprocess
import sys
from tempfile import TemporaryDirectory

from direct_catalog_sources import COMMIT, reviewed_catalogs, validate_review, verify_file
from seed_release_catalog import (
    LEGACY_PATCH_EXPORT, LOCAL_PATCH_EXPORT, LOCAL_PATCH_EXPORT_TITLE, oda_catalog, read_json, sha, write_catalogs,
)

PRODUCTS = ("database", "exadata", "oda", "supercluster")
VERSION = re.compile(r"v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)")


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


def reconcile_legacy_11g_psus(patches, history):
    """Archive imported observations covered by the exact MOS DB or GI PSU."""
    candidates = {}
    for pos, row in enumerate(patches):
        source = original_id(row["source"])
        component = row.get("component")
        version = row.get("version", "")
        if (source not in {LOCAL_PATCH_EXPORT, "KB111315"}
                or row["line"] != "11.2.0.4" or row["kind"] != "PSU"
                or row.get("source_kind") != "PSU" or row.get("product_name") != "11gR2"
                or component not in {"GI", "RDBMS"} or row.get("source_name")
                or row.get("source_subtype") not in {None, "", component}
                or not re.fullmatch(r"11\.2\.0\.4\.[0-9]{6}", version)):
            continue
        candidates.setdefault((version, component), {}).setdefault(source, []).append(pos)

    archived = set()
    for group in candidates.values():
        imported = group.get(LOCAL_PATCH_EXPORT, [])
        verified = group.get("KB111315", [])
        if len(imported) != 1 or len(verified) != 1:
            continue
        legacy, oracle = patches[imported[0]], patches[verified[0]]
        # Preserve conflicting flags and restrictions still supported only
        # by the import. Missing booleans make no affirmative claim.
        default_flags = {"base_release": False, "terminal_release": False,
                         "restricted": False, "disabled_at": ""}
        if any(legacy.get(flag, default) != oracle.get(flag, default)
               for flag, default in default_flags.items()):
            continue
        reviewed_packages = {package_observation(artifact) for artifact in oracle.get("artifacts", [])
                             if artifact.get("number", 0) > 0}
        if reviewed_packages and all(package_observation(artifact) in reviewed_packages
                                     for artifact in legacy.get("artifacts", [])):
            archived.add(imported[0])
            history.append(legacy)
    return [row for pos, row in enumerate(patches) if pos not in archived]


def package_observation(artifact):
    # Compare exact package and scope fields, including unknown or omitted
    # platform information. The complete original provenance goes to history.
    return key_json({field: value for field, value in artifact.items()
                     if field not in {"source", "recorded_source_id", "corroborated_by"}})


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


def migrate_patch_export_source(catalog):
    # Main snapshots created before #96 used the previous export ID. Reconcile
    # those rows with the same source identity and retain archived evidence.
    sources = []
    aliases = {}
    for source in catalog["sources"]:
        id_ = source["id"]
        if original_id(id_) == LEGACY_PATCH_EXPORT:
            renamed = LOCAL_PATCH_EXPORT + id_[len(LEGACY_PATCH_EXPORT):]
            sources.append(dict(source, id=renamed, title=LOCAL_PATCH_EXPORT_TITLE))
            aliases[id_] = renamed
        else:
            sources.append(source)
            aliases[id_] = id_
    if all(old == new for old, new in aliases.items()):
        return catalog

    migrated = dict(catalog, sources=sources)
    for field in ("releases", "patches", "coverage"):
        migrated[field] = [remap_row(row, aliases) for row in catalog[field]]
    statuses = {
        "harvester_snapshot": "local_patch_snapshot",
        "harvester_qfsdp_snapshot": "local_qfsdp_snapshot",
    }
    for row in migrated["coverage"]:
        if original_id(row["source"]) == LOCAL_PATCH_EXPORT and row["status"] in statuses:
            row["status"] = statuses[row["status"]]
    if "history" in catalog:
        migrated["history"] = {field: [remap_row(row, aliases) for row in rows]
                               for field, rows in catalog["history"].items()}
    return migrated


def merge_catalog(old, fresh):
    if old["product"] != fresh["product"] or old["schema_version"] != fresh["schema_version"]:
        raise ValueError("cannot reconcile different products or schemas")
    old = migrate_patch_export_source(old)
    sources, aliases = prior_aliases(old, fresh)
    result = dict(fresh)
    release_history, patch_history = [], []
    result["releases"] = merge_rows(old["releases"], fresh["releases"], release_key, aliases,
                                    release_history, "release")
    result["patches"] = merge_patches(old["patches"], fresh["patches"], aliases, patch_history)
    if result["product"] == "database":
        result["patches"] = reconcile_legacy_11g_psus(result["patches"], patch_history)
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
    if not all((args.review, args.oda_captures, args.output_dir)):
        ap.error("review, ODA capture and output paths are required")
    repo = Path(git("rev-parse", "--show-toplevel")).resolve()
    review_file = args.review.resolve(strict=True)
    oda_captures = args.oda_captures.resolve(strict=True)
    dest = args.output_dir.resolve()
    if any(path.is_relative_to(repo) for path in (review_file, oda_captures, dest)):
        raise ValueError("catalog refresh evidence must stay outside the source checkout")
    raw_review = review_file.read_bytes()
    review = json.loads(raw_review)
    now = datetime.now(timezone.utc)
    valid_until, proofs = validate_review(review, args.version, oda_captures, review_file.parent, now)
    base = review["history_base"]
    git("merge-base", "--is-ancestor", base, "HEAD")
    # All tagged history already shipped must be within the selected baseline.
    tags = git("for-each-ref", "--merged", "HEAD", "--format=%(refname:short)", "refs/tags")
    prior_tags = [tag for tag in tags.splitlines() if VERSION.fullmatch(tag) and tag != args.version]
    if prior_tags:
        latest = max(prior_tags, key=lambda tag: tuple(int(s) for s in tag[1:].split(".")))
        git("merge-base", "--is-ancestor", f"refs/tags/{latest}^{{commit}}", base)
    previous = {product: json.loads(raw) if (raw := committed(base, product, missing_ok=product == "supercluster"))
                else None for product in PRODUCTS}
    as_of = datetime.fromisoformat(review["reviewed_utc"]).astimezone(timezone.utc).date().isoformat()
    candidates = reviewed_catalogs(review["checks"], as_of, previous)
    candidates["oda"] = oda_catalog(oda_captures, as_of)
    results = [
        merge_catalog(previous[product], candidates[product]) if previous[product] else candidates[product]
        for product in PRODUCTS
    ]
    # A reviewer pins the exact files. Catch an exchange during preview generation.
    for proof in proofs:
        verify_file(*proof)
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
    except (ValueError, KeyError, TypeError, OSError, subprocess.CalledProcessError) as err:
        sys.exit(f"Catalog publish gate: {err}")
