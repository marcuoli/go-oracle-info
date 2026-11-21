#!/usr/bin/env python3
"""Capture the public ODA documentation index and patch evidence for catalog refresh."""

import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from hashlib import sha256
from html.parser import HTMLParser
import json
from pathlib import Path
import re
from urllib.error import HTTPError, URLError
from urllib.parse import urljoin, urlsplit
from urllib.request import Request, urlopen

INDEX = "https://docs.oracle.com/en/engineered-systems/oracle-database-appliance/"


class Links(HTMLParser):
    def __init__(self, page):
        super().__init__(convert_charrefs=True)
        self.url = None
        self.text = []
        self.links = []
        self.feed(page.decode("utf-8", errors="replace"))

    def handle_starttag(self, name, attrs):
        if name == "a":
            self.url = dict(attrs).get("href")
            self.text = []

    def handle_data(self, data):
        if self.url:
            self.text.append(data)

    def handle_endtag(self, name):
        if name == "a" and self.url:
            self.links.append((self.url, " ".join("".join(self.text).split())))
            self.url = None


def is_oracle_docs(url):
    parsed = urlsplit(url)
    return parsed.scheme == "https" and parsed.hostname == "docs.oracle.com"


def get(url):
    if not is_oracle_docs(url):
        raise ValueError(f"unexpected source host: {url}")
    with urlopen(Request(url, headers={"User-Agent": "go-oracle-info-catalog/1.0"}), timeout=20) as reply:
        if not is_oracle_docs(reply.url):
            raise ValueError(f"unexpected final source host: {reply.url}")
        return reply.url, reply.read()


def save(root, filename, url, raw):
    (root / filename).write_bytes(raw)
    return dict(url=url, file=filename, sha256=sha256(raw).hexdigest())


def archive(root, release, index_url):
    index_url, page = get(index_url)
    links = dict.fromkeys(
        urljoin(index_url, url) for url, label in Links(page).links
        if "release notes" in label.casefold()
        and is_oracle_docs(urljoin(index_url, url))
        and urlsplit(urljoin(index_url, url)).path.startswith("/cd/")
        and not urlsplit(url).path.endswith(".pdf")
    )
    saved = []
    for link in links:
        url, content = get(link)
        if release.rsplit(".", 1)[0].encode() in content or release.encode() in content:
            saved.append(save(root, f"{release}-archive-{len(saved) + 1}.html", url, content))
    if saved:
        return dict(status="archive_release_notes", archive=saved)
    return dict(status="archive_index_only")


def capture_release(root, release, index_url):
    record = dict(release=release, index_url=index_url)
    try:
        if "/oracle-database-appliance/" not in urlsplit(index_url).path:
            record.update(archive(root, release, index_url))
        else:
            target = urljoin(index_url, "cmtrn/oda-patches.html")
            url, page = get(target)
            if f"/oracle-database-appliance/{release}/" not in urlsplit(url).path or b"Patch Number" not in page:
                return dict(record, status="no_matching_patch_table", attempted_url=target)
            record.update(save(root, f"{release}.html", url, page), status="patch_table")
    except (HTTPError, URLError, TimeoutError, ValueError) as err:
        record.update(status="source_unavailable", error=str(err))
    return record


def main():
    args = argparse.ArgumentParser(description=__doc__)
    args.add_argument("--output-dir", type=Path, required=True)
    root = args.parse_args().output_dir
    index_url, data = get(INDEX)
    indexes = {}
    for url, title in Links(data).links:
        version = re.fullmatch(r"(?:Get Started with Oracle Database Appliance )?Release (\d+(?:\.\d+)+)(?: \(X10\))?", title)
        url = urljoin(index_url, url)
        if version and is_oracle_docs(url):
            indexes[version[1]] = url
    if len(indexes) < 35:
        raise ValueError("ODA documentation index is incomplete or its format has changed")
    root.mkdir(parents=True, exist_ok=False)
    index = save(root, "index.html", index_url, data)
    with ThreadPoolExecutor(max_workers=4) as workers:
        releases = list(workers.map(lambda item: capture_release(root, *item), indexes.items()))
    receipt = dict(index_url=index["url"], index_sha256=index["sha256"],
                   checked_utc=datetime.now(timezone.utc).isoformat(), releases=releases)
    (root / "sources.json").write_text(json.dumps(receipt, indent=2) + "\n", encoding="utf-8")
    print(f"{len(releases)} releases, {sum(r['status'] == 'patch_table' for r in releases)} patch tables")
    for record in releases:
        if record["status"] != "patch_table":
            print(record["release"], record["status"])


if __name__ == "__main__":
    main()
