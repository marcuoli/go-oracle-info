#!/usr/bin/env python3
"""Normalize reviewed MOS recommendation captures into a private SDK input."""

import argparse
from datetime import datetime
from email import policy
from email.parser import BytesParser
import json
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import parse_qs, urlsplit

from seed_release_catalog import Tables, sha, words

HEADING = re.compile(r"Recommended Patches for (19\.[0-9]+|23\.26\.[0-9]+) (DB|GI) Home")
HEAD_TAGS = {"h1", "h2", "h3", "h4", "h5", "h6"}
LINK_PATH = "/epmos/faces/ui/patch/PatchDetail.jspx"
NUMBER = re.compile(r"[0-9]{6,10}")
PLATFORM = re.compile(r"^(Linux|Windows|AIX|Solaris|HP-UX) Only:", re.I)
PLATFORM_NAMES = {p.casefold(): p for p in ("Linux", "Windows", "AIX", "Solaris", "HP-UX")}
CURRENT_TITLE = "Oracle Database 19c and Oracle AI Database 26ai Important Recommended One-off Patches"


def scope_key(scope):
    return scope["line"], scope["train"], scope["home"]


class RecommendationTables(Tables):
    """Associate a selected heading only with its own first table."""

    def __init__(self, text, wanted):
        self.wanted = wanted
        self.sections = {}
        self.heading = None
        self.scope = None
        super().__init__(text)

    def handle_starttag(self, tag, attrs):
        if self._table is None:
            if tag in HEAD_TAGS:
                self.scope, self.heading = None, (tag, [])
            elif tag == "table" and self.scope in self.wanted:
                super().handle_starttag(tag, attrs)
                self._table["links"] = []
            return
        super().handle_starttag(tag, attrs)
        if tag == "tr":
            self._table["links"].append({})
        elif tag == "a" and self._cell is not None:
            self._table["links"][-1].setdefault(len(self._row), []).append(dict(attrs).get("href", ""))

    def handle_data(self, data):
        if self.heading is not None:
            self.heading[1].append(data)
        if self._table is not None:
            super().handle_data(data)

    def handle_endtag(self, tag):
        if self.heading is not None and tag == self.heading[0]:
            heading = HEADING.fullmatch(words(" ".join(self.heading[1])))
            self.heading = None
            if heading:
                train, home = heading.groups()
                self.scope = ("19" if train.startswith("19.") else "26", train,
                              "database" if home == "DB" else "grid_infrastructure")
        if self._table is not None:
            if tag == "table":
                if self.scope in self.sections:
                    raise ValueError("duplicate reviewed recommendation table")
                self.sections[self.scope] = self._table
                self.scope = None
            super().handle_endtag(tag)


def capture_html(entry):
    source = entry["source"]
    doc = source["document"]
    archive = entry.get("archive")
    capture = entry.get("capture")
    if bool(archive) == bool(capture):
        raise ValueError("review must identify exactly one capture")
    if archive:
        repository = Path(archive["repository"]).resolve(strict=True).as_posix()
        commit = source["archive_commit"]
        path = archive["path"]
        if not re.fullmatch(r"[0-9a-f]{40}", commit) or path != "assets/docs/555.1.html" or doc["id"] != "555.1":
            raise ValueError("invalid reviewed 555.1 archive identity")
        raw = subprocess.check_output(["git", "-c", f"safe.directory={repository}", "-C", repository, "show",
                                       f"{commit}:./{path}"], stderr=subprocess.DEVNULL)
    else:
        raw = Path(capture).read_bytes()
    if sha(raw) != doc["sha256"]:
        raise ValueError("reviewed Oracle recommendation capture hash mismatch")
    if archive:
        text = raw.decode("utf-8-sig")
        if not re.search(r"<title[^>]*>\s*Document\s+555\.1\s*</title>", text, re.I | re.S):
            raise ValueError("archived capture does not identify Oracle Doc ID 555.1")
        return text

    message = BytesParser(policy=policy.default).parsebytes(raw)
    parts = [part.get_content() for part in message.walk() if part.get_content_type() == "text/html"]
    captured = datetime.fromisoformat(source["captured_utc"])
    if len(parts) != 1 or not message["Date"] or message["Date"].datetime != captured:
        raise ValueError("unexpected Oracle MHTML payload or capture date")
    if (doc["id"] != "KB188772" or doc["old_doc_id"] != "555.1" or doc["title"] != CURRENT_TITLE
            or not all(value in parts[0] for value in (doc["id"], doc["title"], doc["revision"]))):
        raise ValueError("capture does not match the reviewed Oracle article and revision")
    return parts[0]


def linked_patch_numbers(links):
    numbers = set()
    for link in links:
        url = urlsplit(link)
        if url.hostname not in (None, "support.oracle.com") or url.path != LINK_PATH:
            continue
        for number in parse_qs(url.query).get("patchId", []):
            if NUMBER.fullmatch(number):
                numbers.add(int(number))
    return numbers


def recommendation_rows(table):
    headers, *rows = table["rows"]
    columns = {value.upper(): i for i, value in enumerate(headers)}
    required = {"BUG", "FIXED IN RU", "FIXED IN MRP", "DESCRIPTION", "PATCHES"}
    if len(columns) != len(headers) or not required <= columns.keys():
        raise ValueError("unexpected Oracle recommendation table columns")
    patch_column = columns["PATCHES"]
    result = []
    for row, links in zip(rows, table["links"][1:], strict=True):
        if len(row) != len(headers):
            raise ValueError("incomplete Oracle recommendation row")
        numbers = linked_patch_numbers(links.get(patch_column, []))
        printed = re.fullmatch(r"(?:patch\s+)?([0-9]{6,10})", row[patch_column], re.I)
        if printed:
            numbers.add(int(printed[1]))
        if len(numbers) != 1:
            raise ValueError("recommendation row lacks one exact patch reference")
        item = {"number": numbers.pop(), "fixed_in_ru": row[columns["FIXED IN RU"]],
                "fixed_in_mrp": row[columns["FIXED IN MRP"]]}
        restriction = PLATFORM.match(row[columns["DESCRIPTION"]])
        if restriction:
            item["platform_family"] = PLATFORM_NAMES[restriction[1].casefold()]
        result.append(item)
    return result


def build(review):
    if review["schema_version"] != 1:
        raise ValueError("unexpected Oracle recommendation review schema")
    sections = {}
    for entry in review["sources"]:
        wanted = {scope_key(item): item for item in entry["sections"]}
        if not wanted or len(wanted) != len(entry["sections"]) or wanted.keys() & sections.keys():
            raise ValueError("duplicate or empty reviewed scope")
        observed = RecommendationTables(capture_html(entry), wanted).sections
        if observed.keys() != wanted.keys():
            raise ValueError("reviewed Oracle recommendation section was not captured")
        for key, scope in wanted.items():
            rows = recommendation_rows(observed[key])
            if len(rows) != scope["expected_rows"]:
                raise ValueError("Oracle recommendation row count differs from review")
            sections[key] = {"scope": {field: scope[field] for field in ("line", "train", "home")},
                             "source": entry["source"], "rows": rows}
    if not sections:
        raise ValueError("no reviewed Oracle recommendation sections")
    return {"schema_version": 1, "sections": list(sections.values())}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--review", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    review, output = args.review.resolve(strict=True), args.output.resolve()
    checkout = Path(__file__).resolve().parent.parent
    if review.is_relative_to(checkout) or output.is_relative_to(checkout):
        raise ValueError("keep Oracle review inputs and normalized output outside source control")
    data = build(json.loads(review.read_bytes()))
    output.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    print(f"Normalized {len(data['sections'])} reviewed recommendation sections.")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        print(f"Recommendation normalization failed: {error}", file=sys.stderr)
        sys.exit(1)
