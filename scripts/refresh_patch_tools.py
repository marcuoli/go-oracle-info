#!/usr/bin/env python3
"""Generate private SDK inputs from reviewed Oracle Gold Image and AutoUpgrade captures."""

import argparse
from datetime import datetime, timezone
from email import policy
from email.parser import BytesParser
from html.parser import HTMLParser
import json
from pathlib import Path
import re
import subprocess
from urllib.parse import parse_qs, urlsplit

from seed_release_catalog import Tables, manifest_source, sha, verified_mhtml, words

GOLD_HEADINGS = {
    "2.0 Database Release Update Goldimage for DB Home": "database",
    "3.0 GI Release Update Goldimage for GI Home": "grid_infrastructure",
    "8.0 Database Release Update Goldimage for DB Client Home": "client",
}
RESTRICTION = "Applicable to 26ai Cloud Services, Exadata and ODA Engineered Systems only."
BUILD = re.compile(r"AutoUpgrade (\d+\.\d+) build version (\d+\.\d+\.\d+)\.jar")
LEGACY_BUILD = re.compile(
    r"AutoUpgrade (\d+\.\d+) build version (\d+\.\d+\.\d+)\s*"
    r"\(SHA-256 checksum ([a-f0-9]{64})\)", re.I,
)


class GoldTables(Tables):
    """Bind each selected table and its preceding scope text to one article heading."""

    def __init__(self, text):
        self._heading = None
        self._context = []
        self._home = None
        self.sections = {}
        super().__init__(text)

    def handle_starttag(self, tag, attrs):
        if tag in {"h1", "h2", "h3", "h4", "h5", "h6"} and self._table is None:
            self._heading = (tag, [])
            self._home = None
            self._context = []
        if self._home:
            super().handle_starttag(tag, attrs)
            if tag == "table":
                self._table["patch_links"] = []
            elif tag == "tr":
                self._table["patch_links"].append([])
            elif tag == "a" and self._cell is not None and len(self._row) == 2:
                self._table["patch_links"][-1].append(dict(attrs).get("href", ""))

    def handle_data(self, data):
        if self._heading:
            self._heading[1].append(data)
        elif self._home and self._table is None:
            self._context.append(data)
        if self._table is not None:
            super().handle_data(data)

    def handle_endtag(self, tag):
        if self._heading and tag == self._heading[0]:
            self._home = GOLD_HEADINGS.get(words(" ".join(self._heading[1])))
            self._heading = None
        if self._table is not None:
            super().handle_endtag(tag)
            if tag == "table":
                if self._home in self.sections:
                    raise ValueError("duplicate Gold Image home table")
                self.sections[self._home] = (self.tables[-1], words(" ".join(self._context)))
                self._home = None


class AutoUpgradeLinks(HTMLParser):
    def __init__(self, text):
        super().__init__()
        self.links = []
        self._link = None
        self.feed(text)

    def handle_starttag(self, tag, attrs):
        if tag == "a":
            self._link = (dict(attrs), [])

    def handle_data(self, data):
        if self._link is not None:
            self._link[1].append(data)

    def handle_endtag(self, tag):
        if tag == "a" and self._link is not None:
            attrs, label = self._link
            self.links.append((attrs, words(" ".join(label))))
            self._link = None


def capture(docs, item, expected_id, old_id, title):
    document = manifest_source(item)
    if (document["id"], document["old_doc_id"], document["title"]) != (expected_id, old_id, title):
        raise ValueError("wrong Oracle Support source identity")
    text = verified_mhtml(docs, item)
    if not all(marker in text for marker in (expected_id, title, document["revision"])):
        raise ValueError("capture does not match the reviewed article revision")

    # Match provenance to the exact verified bytes and actual MHTML capture time.
    raw = (docs / item["file"]).read_bytes()
    if sha(raw) != document["sha256"]:
        raise ValueError("capture changed during review")
    envelope = BytesParser(policy=policy.default).parsebytes(raw)
    captured = envelope["Date"].datetime.astimezone(timezone.utc)
    if captured > datetime.fromisoformat(document["checked_utc"]):
        raise ValueError("capture is newer than its recorded check")
    document["url"] = f"https://support.oracle.com/support/?kmContentId={expected_id}"
    return text, document, captured.isoformat().replace("+00:00", "Z")


def gold_images(docs, item):
    text, document, captured = capture(docs, item, "KB153394", "3053981.1",
                                       "Oracle AI Database 26ai Proactive Patch Information")
    tables = GoldTables(text).sections
    if set(tables) != set(GOLD_HEADINGS.values()):
        raise ValueError("missing reviewed Gold Image home")
    sections = {}
    for home, (table, context) in tables.items():
        client = home == "client"
        heading = "Download Link (all platforms)" if client else "Download Link"
        has_restriction = RESTRICTION in context
        if table["rows"][0][:3] != ["Release Date", "Version", heading] or has_restriction != (not client):
            raise ValueError("unexpected Gold Image applicability or table heading")
        for row, links in zip(table["rows"][1:], table["patch_links"][1:], strict=True):
            if len(links) != 1:
                raise ValueError("expected one Oracle Gold Image patch link per row")
            version, qualifier = row[1].split(maxsplit=1) if " " in row[1] else (row[1], "")
            parts = version.split(".")
            expected = {""} if client else (
                {"DB RU Goldimage"} if home == "database" else {"GI Goldimage", "GI RU Goldimage"}
            )
            if len(parts) != 5 or not re.fullmatch(r"23(?:\.\d+){4}", version) or qualifier not in expected:
                raise ValueError("unexpected Oracle Gold Image version label")
            ref = urlsplit(links[0])
            params = parse_qs(ref.query, keep_blank_values=True)
            patch_ids = params.get("patchId", [])
            if (not re.fullmatch(r"[0-9]{6,10}", row[2]) or
                    (ref.scheme, ref.netloc, ref.path, ref.fragment) != ("https", "support.oracle.com", "/support/", "")
                    or set(params) != {"patchId"} or [value.strip() for value in patch_ids] != [row[2]]):
                raise ValueError("Gold Image number does not match the Oracle patch link")
            line, width = ("26", 3) if parts[1] == "26" else ("23", 2)
            train = ".".join(parts[:width])
            scope = dict(line=line, train=train, home=home)
            reference = dict(number=int(row[2]), version=version)
            reference["all_platforms" if client else "engineered_or_cloud_only"] = True
            sections.setdefault((line, train, home), dict(scope=scope, rows=[]))["rows"].append(reference)
    return dict(schema_version=1, source=document, captured_utc=captured, sections=list(sections.values()))


def autoupgrade(docs, item, reviewed_utc, archive_repo, archive_commit, archive_sha, *, archive_file=None):
    if (archive_repo is None) == (archive_file is None):
        raise ValueError("specify one local archive file or one Git archive")
    if not re.fullmatch(r"[a-f0-9]{40}", archive_commit) or not re.fullmatch(r"[a-f0-9]{64}", archive_sha):
        raise ValueError("expected full reviewed AutoUpgrade archive hashes")
    text, document, captured = capture(docs, item, "KB123450", "2485457.1", "AutoUpgrade Tool")
    builds = []
    for attrs, label in AutoUpgradeLinks(text).links:
        match = BUILD.fullmatch(label)
        if not match:
            continue
        metadata = json.loads(attrs["data-ucm-meta-data"])
        asset, filename = metadata["assetId"], metadata["filename"]
        attachment_path = urlsplit(attrs["data-href"])
        if (filename not in {"autoupgrade.jar", f"autoupgrade_{match[2]}.jar"}
                or not re.fullmatch(r"[0-9A-F]{32}", asset)
                or attachment_path.scheme != "https"
                or not attachment_path.netloc.endswith(".fa.ocs.oraclecloud.com")
                or attachment_path.path != f"/api/knowledge-management/v2/assets/{asset}/download"
                or attachment_path.query or attachment_path.fragment):
            raise ValueError("AutoUpgrade link and attachment identity disagree")
        builds.append(dict(version=match[1], build=match[2], filename=filename,
                           attachment_id=asset, source=document, captured_utc=captured))
    if not builds:
        raise ValueError("no observed AutoUpgrade build links")

    if archive_file is not None:
        raw = archive_file.read_bytes()
    else:
        repo = archive_repo.resolve(strict=True).as_posix()
        raw = subprocess.check_output(["git", "-c", f"safe.directory={repo}", "-C", repo, "show",
                                       f"{archive_commit}:./assets/docs/au_2485457.1.html"])
    text = raw.decode("utf-8-sig")
    if sha(raw) != archive_sha or not re.search(r"<title[^>]*>\s*Document\s+2485457\.1\s*</title>", text, re.I):
        raise ValueError("AutoUpgrade archive bytes do not match the reviewed Oracle article")
    legacy = dict(id="2485457.1", title="AutoUpgrade Tool",
                  url="https://support.oracle.com/support/?kmContentId=2485457",
                  checked_utc=reviewed_utc, sha256=archive_sha)
    # Old article markup may repeat the same attachment in two links.
    old_builds = {}
    for attrs, _ in AutoUpgradeLinks(text).links:
        match = LEGACY_BUILD.fullmatch(attrs.get("title") or "")
        if not match:
            continue
        link = urlsplit(attrs["href"])
        identity = "2485457.1:AUTOUPGRADE" + match[2].replace(".", "")
        params = parse_qs(link.query)
        if (link.scheme, link.netloc, link.path) != (
                "https", "mosemp.us.oracle.com", "/epmos/main/downloadattachmentprocessor"
        ) or params.get("attachid") != [identity] or params.get("sourceId") != ["2485457.1"]:
            raise ValueError("archived AutoUpgrade digest does not match the attachment")
        build = dict(version=match[1], build=match[2],
                     attachment_id=identity, sha256=match[3].lower(), source=legacy, archive_commit=archive_commit)
        if identity in old_builds and old_builds[identity] != build:
            raise ValueError("conflicting archived AutoUpgrade digests")
        old_builds[identity] = build
    if not old_builds:
        raise ValueError("no verified archived AutoUpgrade digest")
    return dict(schema_version=1, builds=builds + list(old_builds.values()))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--oracle-docs", type=Path, required=True)
    archive = parser.add_mutually_exclusive_group(required=True)
    archive.add_argument("--archive-file", type=Path, help="local copy of the reviewed historical AutoUpgrade HTML")
    archive.add_argument("--archive-repo", type=Path, help="checkout path containing assets/docs")
    parser.add_argument("--archive-commit", required=True)
    parser.add_argument("--archive-sha256", required=True)
    parser.add_argument("--reviewed-utc", required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args()
    output = args.output_dir.resolve(strict=True)
    if output.is_relative_to(Path(__file__).resolve().parent.parent):
        raise ValueError("keep normalized Oracle Support inputs outside source control")
    checked = datetime.fromisoformat(args.reviewed_utc)
    if checked.utcoffset() != timezone.utc.utcoffset(None):
        raise ValueError("use a UTC review timestamp")
    manifest = json.loads((args.oracle_docs / "mos-capture-manifest.json").read_bytes())
    items = {}
    for item in manifest["articles"]:
        if item["current_id"] in items:
            raise ValueError("duplicate source article in Oracle capture manifest")
        items[item["current_id"]] = item
    gold = gold_images(args.oracle_docs, items["KB153394"])
    tool = autoupgrade(args.oracle_docs, items["KB123450"], args.reviewed_utc,
                       args.archive_repo, args.archive_commit, args.archive_sha256, archive_file=args.archive_file)
    for name, data in (("gold-images.json", gold), ("autoupgrade-archive.json", tool)):
        (output / name).write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8", newline="\n")
    print(f"Verified {len(gold['sections'])} Gold Image sections and {len(tool['builds'])} AutoUpgrade attachments.")


if __name__ == "__main__":
    main()
