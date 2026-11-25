"""Validate direct Oracle release evidence and build source-scoped catalog updates."""

from datetime import datetime, timedelta
import json
from pathlib import Path
import re
from urllib.parse import parse_qsl, urlsplit

from seed_release_catalog import LEGACY_PATCH_EXPORT, LOCAL_PATCH_EXPORT, catalog, oracle_url, sha, source

ROLES = {
    "database_lifecycle": ("database", "public", "releases"),
    "database_patches": ("database", "mos", "patches"),
    "database_roadmap": ("database", "mos", "releases"),
    "exadata_patches": ("exadata", "mos", "patches"),
    "exadata_availability": ("exadata", "public", "releases"),
    "oda": ("oda", "public", None),
    "supercluster": ("supercluster", "mos", "patches"),
}
MOS_ARTICLES = {
    "database_patches": ("KB106822", "888.1"),
    "database_roadmap": ("PNEWS1360", "742060.1"),
    "exadata_patches": ("KB153930", "888828.1"),
}
IDENTIFIER = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]{0,95}")
COMMIT = re.compile(r"[0-9a-f]{40}")
DIGEST = re.compile(r"[0-9a-f]{64}")
MOS_DOCUMENT_PARAMETERS = {"documentId", "kmContentId"}
MOS_REFERENCE_PARAMETERS = MOS_DOCUMENT_PARAMETERS | {"anchorId", "page", "sptemplate", "patchId", "patchNumber"}

RELEASE_FIELDS = dict.fromkeys((
    "line", "name", "version", "edition", "classification", "general_availability",
    "premier_ends", "extended_ends", "sustaining_ends", "superseded_by",
), str) | {"terminal_release": bool}
PATCH_FIELDS = dict.fromkeys((
    "line", "version", "kind", "component", "source_kind", "source_name", "source_subtype",
    "product_name", "disabled_at",
), str) | dict.fromkeys(("base_release", "terminal_release", "restricted"), bool) | {"artifacts": list}
ARTIFACT_FIELDS = dict.fromkeys((
    "version", "component", "scope", "platform", "mos_platform", "release_date", "disabled_at",
), str) | dict.fromkeys(("number", "platform_id"), int) | dict.fromkeys(("base_release", "terminal_release"), bool) | {"corroborated_by": list}


def evidence_time(value, field):
    parsed = datetime.fromisoformat(value)
    if parsed.utcoffset() is None:
        raise ValueError(f"{field} needs a time zone")
    return parsed


def recent(value, now, field):
    parsed = evidence_time(value, field)
    if not now - timedelta(hours=24) <= parsed <= now:
        raise ValueError(f"{field} is outside the preceding 24 hours")
    return parsed


def verify_file(path, digest, label):
    content = path.read_bytes()
    if not content or not DIGEST.fullmatch(digest) or sha(content) != digest:
        raise ValueError(f"{label} hash mismatch")
    return content


def evidence_path(root, name):
    pathname = Path(name)
    if pathname.anchor or ".." in pathname.parts:
        raise ValueError("evidence outside the review directory")
    resolved = (root / pathname).resolve(strict=True)
    if not resolved.is_relative_to(root) or not resolved.is_file():
        raise ValueError("evidence outside the review directory")
    return resolved


def authority_url(url, kind):
    parts = urlsplit(url)
    if not oracle_url(url) or parts.netloc != parts.hostname or parts.fragment:
        return False
    if kind == "mos":
        # Publish only stable Oracle references, never signed-in session links.
        return parts.hostname == "support.oracle.com" and all(
            key in MOS_REFERENCE_PARAMETERS
            for key, _ in parse_qsl(parts.query, keep_blank_values=True)
        )
    return parts.hostname != "support.oracle.com"


def article_matches(url, identifier, old_doc_id):
    if not re.fullmatch(r"(?:KB|PNEWS)[0-9]+", identifier):
        return True
    parts = urlsplit(url)
    aliases = {identifier}
    if old_doc_id:
        aliases.update((old_doc_id, old_doc_id.split(".", 1)[0]))
    ids = {value for key, value in parse_qsl(parts.query, keep_blank_values=True) if key in MOS_DOCUMENT_PARAMETERS}
    return ids <= aliases if ids else any(part in aliases for part in parts.path.split("/"))


def valid_value(value, value_type):
    if type(value) is not value_type:
        return False
    if value_type is str:
        return bool(value.strip())
    if value_type is int:
        return 0 <= value <= 2**31 - 1
    return True


def validate_fields(record, fields, required):
    if not isinstance(record, dict) or not required <= record.keys() or record.keys() - fields.keys():
        raise ValueError("unexpected catalog fields in reviewed record")
    for field, value in record.items():
        if not valid_value(value, fields[field]):
            raise ValueError(f"invalid {field} in reviewed record")


def validate_records(check, field):
    records = check.get("records", [])
    result = check.get("result")
    if field is None:
        if records != [] or result != "oda_capture":
            raise ValueError("ODA updates must come from the bound public capture")
        return
    if not isinstance(records, list) or result != ("updated" if records else "no_changes"):
        raise ValueError("review must record updates or an explicit no-change result")
    for row in records:
        if field == "releases":
            validate_fields(row, RELEASE_FIELDS, {"line"})
            if not row.keys() - {"line"}:
                raise ValueError("reviewed release needs a fact beyond the release line")
            continue
        validate_fields(row, PATCH_FIELDS, {"line", "kind"})
        if "version" not in row and "source_name" not in row:
            raise ValueError("reviewed patch needs a version or Oracle source name")
        for artifact in row.get("artifacts", []):
            validate_fields(artifact, ARTIFACT_FIELDS, set())
            if not artifact.get("number") and not artifact.get("version"):
                raise ValueError("reviewed package needs a patch number or exact version")
            if "corroborated_by" in artifact:
                supplemental = artifact["corroborated_by"]
                if (not supplemental
                        or any(type(id_) is not str or not IDENTIFIER.fullmatch(id_) for id_ in supplemental)
                        or check["identifier"] in supplemental
                        or len(set(supplemental)) != len(supplemental)):
                    raise ValueError("invalid package corroboration")



def validate_review(review, version, oda, evidence_root, now):
    if review.get("schema_version") != 2 or review.get("version") != version:
        raise ValueError("review does not match the requested version or direct Oracle evidence schema")
    if not COMMIT.fullmatch(review.get("history_base", "")):
        raise ValueError("review needs a full commit ID for history_base")
    if not review.get("reviewed_by", "").strip():
        raise ValueError("reviewer is required")
    reviewed = recent(review["reviewed_utc"], now, "review")
    oda_proof = (oda / "sources.json", review["oda_manifest_sha256"], "ODA capture manifest")
    oda_manifest = json.loads(verify_file(*oda_proof))
    oda_checked = evidence_time(oda_manifest["checked_utc"], "ODA capture")
    allowed = {"patch_table", "archive_release_notes", "archive_index_only"}
    if not oda_manifest["releases"] or any(row["status"] not in allowed for row in oda_manifest["releases"]):
        raise ValueError("ODA source capture is incomplete; reconcile unavailable patch pages before publishing")
    if oda_checked > reviewed:
        raise ValueError("ODA capture is newer than its review")

    review_times = [reviewed]
    proofs = [oda_proof]
    seen = set()
    evidence_roles = set()
    for check in review["checks"]:
        role = check["role"]
        if role not in ROLES:
            raise ValueError("unexpected Oracle authority role")
        _, kind, field = ROLES[role]
        identifier, url = check["identifier"], check["url"]
        identity = (role, identifier)
        if identity in seen:
            raise ValueError("duplicate Oracle authority review")
        seen.add(identity)
        if role not in MOS_ARTICLES or identifier == MOS_ARTICLES[role][0]:
            evidence_roles.add(role)
        if (identifier in {LOCAL_PATCH_EXPORT, LEGACY_PATCH_EXPORT} or not IDENTIFIER.fullmatch(identifier)
                or not authority_url(url, kind)
                or any(not check.get(key, "").strip() for key in ("title", "scope", "finding"))):
            raise ValueError(f"incomplete or unexpected official source for {role}")
        if role in MOS_ARTICLES and identifier == MOS_ARTICLES[role][0] and check.get("old_doc_id") != MOS_ARTICLES[role][1]:
            raise ValueError(f"review must retain the original MOS Doc ID for {role}")
        if any(key in check and not valid_value(check[key], str) for key in ("old_doc_id", "revision")):
            raise ValueError(f"invalid Oracle source metadata for {role}")
        if kind == "mos" and not article_matches(url, identifier, check.get("old_doc_id")):
            raise ValueError(f"MOS URL must reference the reviewed article for {role}")
        if check.get("status") != "verified":
            raise ValueError(f"source review not verified: {role}")
        captured = evidence_time(check["captured_utc"], role + " capture")
        checked = recent(check["checked_utc"], now, role + " review")
        if not captured <= checked <= reviewed:
            raise ValueError(f"review times are out of order for {role}")
        if role == "oda" and oda_checked > checked:
            raise ValueError("ODA capture is newer than its review check")
        review_times.append(checked)
        validate_records(check, field)
        proof = (evidence_path(evidence_root, check["evidence"]), check["evidence_sha256"], f"{role} evidence")
        verify_file(*proof)
        proofs.append(proof)
    if evidence_roles != ROLES.keys():
        raise ValueError("review must cover each required Oracle authority")
    return min(review_times) + timedelta(hours=24), proofs


def reviewed_catalogs(checks, as_of, previous):
    products = {
        product: dict(sources={}, releases=[], patches=[], coverage=[])
        for product in ("database", "exadata", "supercluster")
    }
    # A secondary authority can corroborate one package without supplying a
    # new catalog record. Its dated evidence must be reviewed for this product.
    documents = {}
    for check in checks:
        product, _, _ = ROLES[check["role"]]
        documents.setdefault((product, check["identifier"]), []).append(check)
    for check in checks:
        product, _, field = ROLES[check["role"]]
        records = check.get("records")
        if not records:
            continue
        update = products[product]
        origin = check["identifier"]
        source_ids = {origin}
        for row in records:
            for artifact in row.get("artifacts", []):
                source_ids.update(artifact.get("corroborated_by", []))
        for id_ in sorted(source_ids):
            authority = check
            if id_ != origin:
                authorities = documents.get((product, id_), [])
                if len(authorities) != 1:
                    raise ValueError(f"missing or ambiguous corroborating source {id_} for {product}")
                authority = authorities[0]
            document = source(id_, authority["title"], authority["url"], old_doc_id=authority.get("old_doc_id"),
                              revision=authority.get("revision"), checked_utc=authority["checked_utc"],
                              sha256=authority["evidence_sha256"])
            if update["sources"].setdefault(id_, document) != document:
                raise ValueError(f"conflicting evidence for {id_}")
        update[field].extend(dict(row, source=origin) for row in records)
    return {
        product: catalog(product, as_of, list(update["sources"].values()), update["releases"],
                         update["patches"], update["coverage"], (previous[product] or {}).get("notes", []))
        for product, update in products.items()
    }
