#!/usr/bin/env python3
"""Build portable Oracle catalog JSON from explicit, read-only source captures."""

import argparse
from collections import defaultdict
from datetime import date
from email import policy
from email.parser import BytesParser
import hashlib
from html.parser import HTMLParser
import json
from pathlib import Path
import re
from urllib.parse import urlsplit

LIFETIME_URL = "https://www.oracle.com/assets/lsp-tech-chart-069290.pdf"
FAQ_URL = "https://www.oracle.com/database/faq/"
EXADATA_UPDATE_URL = "https://blogs.oracle.com/exadata/software-2026-nov-17"
VERSION = r"\d+(?:\.\d+)+"
PATCH_NUMBER = r"[1-9]\d{6,8}"

# Oracle Technology Products Lifetime Support policy, effective 2026-10-02.
# Keep the authority's edition distinctions and month-versus-day precision.
LIFECYCLE = (
    ("8.1.7", "", "", "2000-09", "2004-12", "2006-12"),
    ("9.2", "", "", "2002-07", "2007-07", "2010-07"),
    ("10.1", "", "", "2004-01", "2009-01", "2012-01"),
    ("10.2", "", "", "2005-07", "2010-07", "2013-07"),
    ("11.1", "", "", "2007-08", "2012-08", "2015-08"),
    ("11.2", "", "", "2009-09", "2015-01", "2020-12"),
    ("12.1", "Enterprise Edition", "", "2013-06", "2018-07", "2022-07"),
    ("12.1", "Standard Edition", "", "2013-06", "2016-08", "not_available"),
    ("12.1", "Standard Edition One", "", "2013-06", "2016-08", "not_available"),
    ("12.1", "Standard Edition 2", "", "2015-09", "2018-07", "2022-07"),
    ("12.2.0.1", "", "", "2017-03", "2020-11-30", "not_available"),
    ("18c", "", "", "2018-07", "2021-06", "not_available"),
    ("19c", "", "lts", "2019-04", "2029-12", "2032-12"),
    ("21c", "", "innovation", "2021-08", "2027-07", "not_available"),
    ("26ai", "Enterprise Edition", "lts", "2025-10", "2031-12", "tbd"),
)


def clean(value):
    return {k: v for k, v in value.items() if v is not None and v != ""}


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def read_json(path):
    return json.loads(path.read_text(encoding="utf-8-sig"))


def oracle_url(url):
    parsed = urlsplit(url or "")
    host = parsed.hostname or ""
    return parsed.scheme == "https" and (host == "oracle.com" or host.endswith(".oracle.com"))


def source(id_, title, url=None, **metadata):
    if url and not oracle_url(url):
        raise ValueError(f"non-Oracle source URL for {id_}")
    return clean(dict(id=id_, title=title, url=url, **metadata))


def catalog(product, as_of, sources, releases, patches, coverage, notes):
    ids = [r["id"] for r in sources]
    if len(set(ids)) != len(ids):
        raise ValueError(f"duplicate {product} source ID")
    for record in [*releases, *patches, *coverage]:
        if record["source"] not in ids:
            raise ValueError(f"unknown {product} source: {record['source']}")
        for artifact in record.get("artifacts", []):
            if artifact.get("source", record["source"]) not in ids:
                raise ValueError(f"unknown {product} artifact source")
    return dict(
        schema_version=1, product=product, as_of=as_of,
        sources=sorted(sources, key=lambda s: s["id"]), releases=releases,
        patches=patches, coverage=coverage, notes=notes,
    )


def manifest_source(item):
    return source(
        item["current_id"], item["title"], item["source_url"],
        old_doc_id=item.get("old_doc_id"), checked_utc=item["checked_utc"],
        revision=item["oracle_last_updated"], sha256=item["sha256"],
    )


def patch_kind(patch):
    if patch["type"] == "RECOMMENDED":
        return "one_off"
    if patch["type"] != "BUNDLE":
        raise ValueError(f"unknown Harvester record type {patch['type']}")
    label = patch["title"]
    if label in {"PSU", "MRP", "SPU"}:
        return label
    if label in {"DB_RUR", "GI_RUR"}:
        return "RUR"
    if label in {"RU_DB", "RU_GI"} and patch["release"] not in {"11.2.0.4", "12.1.0.2"}:
        return "RU"
    # Historical Harvester bundles sometimes carry RU_DB/RU_GI labels on
    # legacy releases. Preserve the label without assigning modern RU semantics.
    return "bundle"


def component(patch):
    label = patch["title"]
    if label in {"RU_GI", "GI_RUR", "GI_COMBO"}:
        return "GI"
    if label in {"RU_DB", "DB_RUR", "DB_COMBO"}:
        return "RDBMS"
    return patch["subtype"]


def database_catalog(docs, manifest, as_of, harvester_revision):
    raw = (docs / "patch-catalog.json").read_bytes()
    export = json.loads(raw)
    if export["schema_version"] != 1:
        raise ValueError("unexpected Harvester export schema")
    primary = "harvester-patch-export"
    sources = [
        source(primary, "Harvester portable patch catalog", checked_utc=None,
               revision=harvester_revision, sha256=sha(raw)),
        source("oracle-lifetime-policy", "Oracle Lifetime Support Policy: Technology Products",
               LIFETIME_URL, checked_utc="2026-11-21", revision="2026-10-02"),
        source("oracle-database-faq", "Oracle AI Database FAQ", FAQ_URL, checked_utc="2026-11-21"),
    ]
    sources_by_id = {}
    for item in manifest.values():
        if item.get("source_url"):
            for id_ in [item["current_id"], item.get("old_doc_id")]:
                if id_:
                    sources_by_id[id_] = item
    used = set()
    grouped = defaultdict(list)
    parent_keys = {p["key"] for p in export["patches"]}
    if len(parent_keys) != len(export["patches"]):
        raise ValueError("duplicate Harvester patch key")
    row_keys = set()
    for r in export["platform_patches"]:
        if r["key"] in row_keys or r["patch_key"] not in parent_keys:
            raise ValueError("duplicate platform record or missing parent patch")
        row_keys.add(r["key"])
        document = r.get("recorded_source_document_id")
        source_id = primary
        if document in sources_by_id:
            source_id = sources_by_id[document]["current_id"]
            used.add(source_id)
        grouped[r["patch_key"]].append(clean(dict(
            number=r["patch_number"] or None, platform=r["platform"], mos_platform=r["mos_platform"],
            platform_id=r["oracle_platform"], release_date=r["release_date"],
            source=source_id, recorded_source_id=document,
            base_release=r["base_release"], terminal_release=r["terminal_release"],
            disabled_at=r["disabled_at"],
        )))
    sources.extend(manifest_source(manifest[item]) for item in sorted(used | {"PNEWS1360"}))
    patches = []
    for p in export["patches"]:
        patches.append(clean(dict(
            line=p["release"],
            version=p["name"] if re.fullmatch(VERSION, p["name"]) else None,
            source_name=p["name"] if not re.fullmatch(VERSION, p["name"]) else None,
            product_name=p["version"],
            kind=patch_kind(p), component=component(p), source_kind=p["title"],
            source_subtype=p["subtype"], source=primary, base_release=p["base_release"],
            terminal_release=p["terminal_release"], restricted=p["restricted"],
            disabled_at=p["disabled_at"], artifacts=grouped.get(p["key"], []),
        )))
    releases = [
        clean(dict(line=line, name=line if line.endswith(("c", "ai")) else None,
                   edition=edition, classification=track, general_availability=ga,
                   premier_ends=premier, extended_ends=extended, sustaining_ends="indefinite",
                   source="oracle-lifetime-policy"))
        for line, edition, track, ga, premier, extended in LIFECYCLE
    ]
    releases.insert(-1, dict(line="23ai", name="23ai", superseded_by="26ai", source="oracle-database-faq"))
    roadmap = Tables(verified_mhtml(docs, manifest["PNEWS1360"]))
    for _, rows in roadmap.with_header("Release", "Support End Dates"):
        for row in rows:
            if not row:
                continue
            label = row[0]
            match = re.match(r"^(11\.2\.0\.4|12\.1\.0\.[12])\b", label)
            if match:
                releases.append(clean(dict(
                    line=match[1], version=match[1],
                    terminal_release=True if "Terminal Release" in label else None,
                    source="PNEWS1360",
                )))
            elif label == "18c Innovation Release":
                releases.append(dict(line="18c", classification="innovation", source="PNEWS1360"))
    coverage = [
        dict(line=line, source=primary, status="harvester_snapshot")
        for line in sorted({p["line"] for p in patches})
    ] + [dict(source="oracle-lifetime-policy", status="lifecycle_table"),
         dict(source="PNEWS1360", status="saved_release_roadmap")]
    notes = [
        f"Harvester export generated {export['generated_at']}; the stored metadata is a historical snapshot.",
        "Recorded article IDs link to document-level provenance. Prior stored rows are not necessarily present in the current article.",
        "Legacy RU_DB/RU_GI labels are preserved under bundle; they do not establish modern RU semantics.",
        "The source's conflicting Windows patch row was excluded by Harvester. Unresolved dates remain absent.",
        "Edition and error-correction exceptions are in the Oracle policy footnotes. Lifecycle dates do not establish availability for a platform.",
        "Historical patch coverage begins in the Oracle 11.2 family. The lifecycle includes older Oracle versions.",
    ]
    return catalog("database", as_of, sources, releases, patches, coverage, notes)


def supercluster_catalog(database, as_of):
    guide = source("oracle-supercluster-qfsdp-guide", "Oracle SuperCluster M8 and M7 Configuration Guide",
                   "https://docs.oracle.com/cd/E58626_01/html/E58636/z400034a1004245704.html",
                   checked_utc="2026-11-21")
    announcements = {
        36867121: ("2024-Q4", "https://community.oracle.com/mosc/discussion/4577639/were-releasing-quarterly-full-stack-download-patch-for-supercluster-q4-2024-patch-36867121"),
        37610906: ("2025-Q2", "https://community.oracle.com/mosc/discussion/4586923/were-releasing-quarterly-full-stack-download-patch-for-supercluster-q2-2025-patch-37610906"),
        38273760: ("2025-Q4", "https://community.oracle.com/mosc/discussion/4594830/were-releasing-quarterly-full-stack-download-patch-for-supercluster-q4-2025-patch-38273760"),
    }
    notices = {number: source(f"oracle-supercluster-{period}", f"Oracle SuperCluster {period} QFSDP announcement",
                              url, checked_utc="2026-11-21")
               for number, (period, url) in announcements.items()}
    patches = []
    used = set()
    for old in database["patches"]:
        if old.get("source_kind") != "QFSDP_SUPERCLUSTER":
            continue
        patch = dict(old, kind="QFSDP", component="supercluster")
        artifacts = []
        for a in old.get("artifacts", []):
            record = dict(a)
            if a.get("number") in notices:
                record["corroborated_by"] = [notices[a["number"]]["id"]]
            used.add(a.get("source", old["source"]))
            artifacts.append(record)
        if artifacts:
            patch["artifacts"] = artifacts
        used.add(old["source"])
        patches.append(patch)
    if not patches:
        raise ValueError("Harvester export has no SuperCluster QFSDP records")
    sources = [s for s in database["sources"] if s["id"] in used] + [guide, *notices.values()]
    coverage = [dict(line=line, source="harvester-patch-export", status="harvester_qfsdp_snapshot")
                for line in sorted({p["line"] for p in patches})]
    coverage.append(dict(source=guide["id"], status="official_patch_family_reference"))
    return catalog("supercluster", as_of, sources, [], patches, coverage, [
        "Harvester lines and versions are associated Database release levels, not SuperCluster hardware or image versions.",
        "Only explicit SuperCluster QFSDP labels are admitted. Oracle announcements corroborate named package identities only.",
        "The saved source does not establish the current latest SuperCluster QFSDP or every model and platform applicability.",
    ])


def verified_mhtml(docs, item):
    raw = (docs / item["file"]).read_bytes()
    if sha(raw) != item["sha256"]:
        raise ValueError(f"Harvester capture hash mismatch: {item['current_id']}")
    message = BytesParser(policy=policy.default).parsebytes(raw)
    parts = [p.get_content() for p in message.walk() if p.get_content_type() == "text/html"]
    if len(parts) != 1:
        raise ValueError(f"expected one HTML payload: {item['current_id']}")
    return parts[0]


def words(text):
    return " ".join(text.split())


class Tables(HTMLParser):
    """Read visible table cells/captions without using scripts or account chrome."""

    def __init__(self, html):
        super().__init__(convert_charrefs=True)
        self.tables = []
        self.paragraphs = []
        self._table = None
        self._row = None
        self._cell = None
        self._caption = None
        self._paragraph = None
        self.feed(html)

    def handle_starttag(self, tag, attrs):
        if tag == "table":
            if self._table is not None:
                raise ValueError("nested source table")
            self._table = dict(rows=[], scope=words(dict(attrs).get("title") or ""))
        elif tag == "tr" and self._table is not None:
            self._row = []
        elif tag in ("td", "th") and self._row is not None:
            self._cell = []
        elif tag == "caption":
            self._caption = []
        elif tag == "p":
            self._paragraph = []

    def handle_data(self, data):
        for capture in (self._cell, self._caption, self._paragraph):
            if capture is not None:
                capture.append(data)

    def handle_endtag(self, tag):
        if tag in ("td", "th") and self._cell is not None:
            self._row.append(words(" ".join(self._cell)))
            self._cell = None
        elif tag == "tr" and self._row is not None:
            self._table["rows"].append(self._row)
            self._row = None
        elif tag == "caption" and self._caption is not None:
            if self._table is not None and words(" ".join(self._caption)):
                self._table["scope"] = words(" ".join(self._caption))
            self._caption = None
        elif tag == "table" and self._table is not None:
            self.tables.append(self._table)
            self._table = None
        elif tag == "p" and self._paragraph is not None:
            self.paragraphs.append(words(" ".join(self._paragraph)))
            self._paragraph = None

    def with_header(self, *prefix):
        for table in self.tables:
            rows = table["rows"]
            if rows and tuple(rows[0][:len(prefix)]) == prefix:
                yield table["scope"], rows[1:]


def exadata_scope(label):
    scopes = [
        (marker, scope)
        for marker, scope in (
            ("bare metal", "bare_metal"), ("KVM Host", "kvm_host"), ("KVM Guest", "kvm_guest"),
            ("Xen Guest", "xen_guest"), ("domU", "xen_domu"), ("dom0", "xen_dom0"),
            ("Admin/RDMA", "admin_rdma_switch"), ("InfiniBand", "infiniband_switch"),
        )
        if marker in label
    ]
    return ",".join(scope for _, scope in scopes)


def exadata_catalog(docs, manifest, as_of, announcement_check):
    item = manifest["KB153930"]
    origin = manifest_source(item)
    parser = Tables(verified_mhtml(docs, item))
    patches = []
    for _, rows in parser.with_header("Version", "Software"):
        for row in rows:
            if len(row) < 2:
                continue
            version = re.match(rf"^({VERSION})(?=\s|$)", row[0])
            if not version:
                continue
            artifacts = []
            for number, label in re.findall(
                rf"Patch ({PATCH_NUMBER})\s*-\s*(.*?)(?=\s+Patch {PATCH_NUMBER}\b|$)", row[1]
            ):
                label = re.split(r"\s+(?:NOTE|Note)\s*[-:]", label, maxsplit=1)[0]
                component = None
                for prefix, kind in (
                    ("Storage server software", "storage_server"),
                    ("Database server", "database_server"),
                    ("Admin/RDMA network switch", "network_switch"),
                    ("InfiniBand", "network_switch"),
                ):
                    if label.startswith(prefix):
                        component = kind
                        break
                build = re.search(rf"\(({VERSION})\)\s*$", label)
                artifacts.append(clean(dict(
                    number=int(number), component=component, scope=exadata_scope(label),
                    version=build[1] if build else None,
                )))
            if artifacts:
                line = ".".join(version[1].split(".")[:2])
                patches.append(dict(line=line, version=version[1], kind="system_software",
                                    source=origin["id"], artifacts=artifacts))
    for _, rows in parser.with_header("Version", "Patch"):
        for row in rows:
            if len(row) < 2 or "Critical Patch Update (CPU)" not in row[0]:
                continue
            numbers = re.findall(rf"\bPatch ({PATCH_NUMBER})\b", row[1])
            if not numbers:
                continue
            period = re.search(r"\b([A-Z][a-z]{2} 20\d{2})\b", row[0])
            if period:
                patches.append(dict(line="full-stack", source_name=period[1], kind="QFSDP",
                                    source=origin["id"], artifacts=[dict(number=int(n)) for n in dict.fromkeys(numbers)]))
    if sum(p["kind"] == "system_software" for p in patches) < 25:
        raise ValueError("Exadata history could not be safely extracted")
    release_source = source("oracle-exadata-2026-11-18", "Exadata System Software Updates – November 18, 2026",
                            EXADATA_UPDATE_URL, checked_utc=announcement_check)
    # Exact builds named as generally available in Oracle's public announcement.
    releases = [
        dict(line=line, version=build, source=release_source["id"])
        for line, build in (
            ("25.2", "25.2.16.0.0.261117.1"),
            ("26.1", "26.1.5.0.0.261117.2"),
            ("26.2", "26.2.2.0.0.261117.2"),
        )
    ]
    coverage = [dict(line=line, source=origin["id"], status="saved_patch_table")
                for line in sorted({p["line"] for p in patches})]
    coverage += [dict(line=r["line"], source=release_source["id"], status="public_availability")
                 for r in releases]
    return catalog("exadata", as_of, [origin, release_source], releases, patches, coverage, [
        "System software package history comes from the captured MOS article. Oracle's public November announcement adds the identified builds without inferring their package numbers.",
        "This seed does not resolve model compatibility, recommended target, cloud service availability or every historical release line.",
    ])


def oda_kind(label):
    value = label.casefold()
    for phrase, kind in (
        ("server patch for monthly update", "monthly_update"),
        ("gi clone", "gi_clone"),
        ("rdbms clone", "database_clone"),
        ("db clone", "database_clone"),
        ("db system image", "db_system_image"),
        ("db system template", "db_system_image"),
        ("iso image", "iso_image"),
        ("end user bundle", "bundle"),
        ("server patch", "server_patch"),
    ):
        if phrase in value:
            return kind
    return "patch"


def verified_html(root, entry):
    name = entry["file"]
    if Path(name).name != name:
        raise ValueError("unexpected ODA capture path")
    raw = (root / name).read_bytes()
    if sha(raw) != entry["sha256"]:
        raise ValueError(f"ODA capture hash mismatch: {name}")
    return raw.decode("utf-8-sig", errors="replace")


def oda_catalog(root, as_of):
    receipt = read_json(root / "sources.json")
    index = source("oracle-oda-index", "Oracle Database Appliance Documentation", receipt["index_url"],
                   checked_utc=receipt["checked_utc"], sha256=receipt["index_sha256"])
    if sha((root / "index.html").read_bytes()) != index["sha256"]:
        raise ValueError("ODA index hash mismatch")
    sources, releases, coverage, patches = [index], [], [], []
    identities = set()
    for entry in receipt["releases"]:
        line = entry["release"]
        if not re.fullmatch(VERSION, line):
            raise ValueError(f"unexpected ODA release line {line}")
        releases.append(dict(line=line, source=index["id"]))
        if entry["status"] == "patch_table":
            sid = f"oracle-oda-{line}-patches"
            sources.append(source(sid, f"Oracle Database Appliance {line} Patches", entry["url"],
                                  checked_utc=receipt["checked_utc"], sha256=entry["sha256"]))
            parser = Tables(verified_html(root, entry))
            added = 0
            for scope, rows in parser.with_header("Patch Type", "Patch Number"):
                for row in rows:
                    if len(row) < 2 or not row[0]:
                        continue
                    numbers = list(dict.fromkeys(re.findall(rf"(?<![\d.]){PATCH_NUMBER}(?![\d.])", row[1])))
                    if not numbers:
                        continue
                    versions = set(re.findall(rf"\b({VERSION})\b", row[0]))
                    version = next(iter(versions)) if len(versions) == 1 else None
                    key = (line, row[0], scope, tuple(numbers))
                    if key in identities:
                        continue
                    identities.add(key)
                    patches.append(clean(dict(
                        line=line, version=version, kind=oda_kind(row[0]),
                        source_kind=row[0], source=sid,
                        artifacts=[clean(dict(number=int(n), scope=scope)) for n in numbers],
                    )))
                    added += 1
            coverage.append(dict(line=line, source=sid,
                                 status="public_patch_table" if added else "unparsed_patch_table"))
        elif entry["status"] == "archive_release_notes":
            for i, saved in enumerate(entry["archive"], 1):
                sid = f"oracle-oda-{line}-archive-{i}"
                sources.append(source(sid, f"Oracle Database Appliance {line} archived release notes",
                                      saved["url"], checked_utc=receipt["checked_utc"], sha256=saved["sha256"]))
                parser = Tables(verified_html(root, saved))
                matching, other = False, False
                for paragraph in parser.paragraphs:
                    match = re.search(rf"\bOracle Database Appliance (?:Patch )?({VERSION}) Patch ({PATCH_NUMBER})\b", paragraph)
                    if not match:
                        continue
                    version, number = match.groups()
                    if version in (line, line + ".0"):
                        matching = True
                    else:
                        other = True
                    key = (version, number)
                    if key in identities:
                        continue
                    identities.add(key)
                    patches.append(dict(line=version, version=version, kind="bundle", source=sid,
                                        artifacts=[dict(number=int(number))]))
                status = "archive_notes_only"
                if matching:
                    status = "archived_patch_references"
                elif other:
                    status = "archive_other_release_references"
                coverage.append(dict(line=line, source=sid, status=status))
        else:
            coverage.append(dict(line=line, source=index["id"], status="public_index_only"))
    indexed = {r["line"] for r in releases}
    for patch in patches:
        version = patch.get("version", "")
        if patch["kind"] != "bundle" or not version:
            continue
        public_line = version.removesuffix(".0")
        if version in indexed or public_line in indexed:
            continue
        source_id = patch["source"]
        releases.append(dict(line=version, version=version, source=source_id))
        coverage.append(dict(line=version, source=source_id, status="legacy_bundle_reference"))
        indexed.add(version)
    if len(patches) < 100:
        raise ValueError("ODA patch data could not be safely extracted")
    return catalog("oda", as_of, sources, releases, patches, coverage, [
        "Documentation index entries include releases whose exact patch numbers were not recovered.",
        "Package titles and table context are preserved. ODA bundle, server, database and GI clone versions can differ within one indexed release.",
        "The same package number can occur across multiple releases, revisions or hardware groups.",
    ])


def build_catalogs(docs, harvester_revision, oda_captures, as_of, announcement_check):
    capture = read_json(docs / "mos-capture-manifest.json")
    if capture["schema"] != 1:
        raise ValueError("unexpected Harvester source manifest schema")
    manifest = {r["current_id"]: r for r in capture["articles"]}
    if len(manifest) != len(capture["articles"]):
        raise ValueError("duplicate Harvester source ID")
    for item in manifest.values():
        if sha((docs / item["file"]).read_bytes()) != item["sha256"]:
            raise ValueError(f"Harvester capture hash mismatch: {item['current_id']}")
    database = database_catalog(docs, manifest, as_of, harvester_revision)
    return [
        database, supercluster_catalog(database, as_of),
        exadata_catalog(docs, manifest, as_of, announcement_check),
        oda_catalog(oda_captures, as_of),
    ]


def write_catalogs(catalogs, output_dir):
    output_dir.mkdir(parents=True, exist_ok=True)
    for item in catalogs:
        path = output_dir / (item["product"] + ".json")
        # Compact rows keep the large per-platform history readable in diffs.
        with path.open("w", encoding="utf-8", newline="\n") as output:
            output.write("{\n")
            for i, (key, value) in enumerate(item.items()):
                if i:
                    output.write(",\n")
                output.write(f"  {json.dumps(key)}: ")
                if isinstance(value, list):
                    output.write("[\n")
                    output.write(",\n".join("    " + json.dumps(row, ensure_ascii=False) for row in value))
                    output.write("\n  ]")
                else:
                    output.write(json.dumps(value, ensure_ascii=False))
            output.write("\n}\n")
        print(item["product"], len(item["releases"]), "release records,", len(item["patches"]),
              "patch records,", sum(len(p.get("artifacts", [])) for p in item["patches"]), "artifact records")


def main():
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("--harvester-docs", type=Path, required=True)
    cli.add_argument("--harvester-revision", required=True)
    cli.add_argument("--oda-captures", type=Path, required=True)
    cli.add_argument("--output-dir", type=Path, required=True)
    cli.add_argument("--as-of", type=date.fromisoformat, required=True)
    cli.add_argument("--exadata-announcement-checked", type=date.fromisoformat, default=date(2026, 11, 21))
    a = cli.parse_args()
    catalogs = build_catalogs(a.harvester_docs, a.harvester_revision, a.oda_captures,
                              a.as_of.isoformat(), a.exadata_announcement_checked.isoformat())
    write_catalogs(catalogs, a.output_dir)


if __name__ == "__main__":
    main()
