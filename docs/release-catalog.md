# Oracle release and patch catalog

`releases.Load(releases.Database)`, `releases.Load(releases.Exadata)`,
`releases.Load(releases.ODA)`, and `releases.Load(releases.SuperCluster)` read the snapshots bundled in
[`releases/data`](../releases/data). All catalog JSON files live in that
directory. The loader reads embedded data and does not open a database
connection, query Oracle Support, or select an upgrade target.

## Evidence and scope

- **Database and Grid Infrastructure:** the historical local patch metadata export (generated November 9, 2026), the independently published
  [Lifetime Support policy](https://www.oracle.com/assets/lsp-tech-chart-069290.pdf),
  and the saved Oracle roadmap (PNEWS1360, old Doc ID 742060.1). Product
  branding and numeric patch levels remain separate. The export covers
  stored historical metadata from 11.2 onward. The source's labels and
  disabled state survive. A saved document reference does not verify every
  historical row, and the source's excluded Windows row is not recreated.
- **Exadata:** the saved Exadata supported versions article (KB153930, old
  Doc ID 888828.1) supplies software and full-stack download history. The
  [November 18 Oracle announcement](https://blogs.oracle.com/exadata/software-2026-nov-17)
  adds exact published builds when no matching component patch row was
  verified. Hardware applicability and cloud availability remain unqualified.
- **Oracle Database Appliance:** the
  [Oracle documentation index](https://docs.oracle.com/en/engineered-systems/oracle-database-appliance/)
  supplies the release listing. Package rows come from the matching patch
  tables or explicit version/patch references in archived release notes.
  A reused package number is scoped by the release and table context. When a
  source row lists multiple versions without one explicit package version,
  the original label is retained and the exact version field is empty.

- **SuperCluster:** Local patch export records explicitly labeled QFSDP_SUPERCLUSTER
  seed the distinct catalog. Line and version values describe associated
  database releases. The Oracle [SuperCluster M8/M7 guide](https://docs.oracle.com/cd/E58626_01/html/E58636/z400034a1004245704.html)
  establishes the patch family. Selected Oracle announcements corroborate
  package numbers; the seed does not establish the latest applicable bundle
  for every model.

A record's `source` references an entry in `sources`. Use that entry to identify
the input or cite the Oracle document. `local-patch-export` identifies the imported
snapshot by its input revision and SHA-256 digest. The loader returns these fields
as data for callers. The release refresh also uses the source ID to reconcile
records and retain their historical provenance.

An empty `artifacts` array means the source provides no platform-specific package
records for that patch group. This includes the imported 11.2.0.4 Grid Infrastructure
PSU entries. Use the matching Oracle patch documentation to verify package details
for a specific platform.

The top-level snapshot date records when the seed was prepared. A source's
check date, article revision, and artifact release date are separate fields.
An absent date or package number is unresolved. A published `tbd`,
`not_available`, or `indefinite` milestone remains explicit. Coverage states
identify material read and gaps; they do not certify a complete Oracle
inventory, release availability on a live system, or patch eligibility.

## Refresh before publish

1. Record the selected module version and the exact canonical main commit as
   `history_base`. Review the required Oracle authorities using the available
   Oracle web materials and verified original Oracle Support captures, including
   copies retained with the local Harvester catalog. Check the relevant product,
   numeric release, platform or model, and patch scope. Retain the original
   capture time, identifier, known legacy Doc ID, title and Oracle revision.
   Bind a private copy of the supporting evidence to its stable Oracle URL and
   exact hash. An older capture can establish a historical scoped observation.
   Current applicability, completeness and latest availability each need
   supporting evidence. Obtain another Oracle capture when a required finding
   needs evidence beyond the copies already available.
2. Review the public Oracle authorities and existing ODA index and release
   capture, collecting an updated capture if needed. Create a private
   `review.json` for this release. For each document,
   record the scoped finding and copy verified new or corrected facts into the
   review's `records` array. Cite a release-specific patch article when the
   quarterly overview lacks package details. Record an explicit no-change result
   when the review finds no changed facts. Keep raw evidence and the review
   outside source control.
3. Run `refresh`, inspect the generated JSON diff, and merge it through a PR.
   Existing release, patch and platform records remain in the catalog, including
   records from the historical import. Reconciliation retains the previous
   observation and its source when the same source-scoped record changes.
4. On synchronized canonical main, run `publish_module.ps1 -Action Check`.
   The command rebuilds the exact four files from the review and its historical
   baseline. It rejects missing evidence, stale applicability checks, changed evidence bytes,
   intervening catalog changes and mismatches with the committed catalogs.
   Review all required sources for every module version, including when no
   changes are found. See [publication and resume](mirroring.md).

```powershell
python -B scripts/capture_oda_release_sources.py --output-dir <V-oda-evidence-dir>
python -B scripts/refresh_release_catalog.py refresh --version v0.X.Y --review <V-review.json> --oda-captures <V-oda-evidence-dir> --output-dir <V-preview-dir>
./scripts/publish_module.ps1 -Action Check -Version v0.X.Y -Review <V-review.json> -OdaCaptures <V-oda-evidence-dir> -EvidenceDirectory <V-release-evidence-dir>
```

### Direct Oracle review

The private review contains `schema_version: 2`, `version`, `history_base`,
`reviewed_by`, `reviewed_utc`, `oda_manifest_sha256` (the hash of the ODA
`sources.json`), and a `checks` array. Each check supplies `role`,
`status: "verified"`, `title`, `identifier`, `url`, `scope`, `finding`,
`captured_utc`, `checked_utc`, `evidence`, `evidence_sha256`, and `result`.
Include `old_doc_id` and `revision` where known. Evidence filenames are relative to
the review directory. Times are ISO 8601 with offsets. Each capture precedes or
equals its review time, which in turn precedes or equals the overall review
time. The 24-hour release window starts at the oldest applicability check or overall review. Keep original capture times unchanged when reviewing an existing snapshot.

| Role | Required evidence | Records |
| --- | --- | --- |
| `database_lifecycle` | Oracle Lifetime Support Policy and applicable edition dates. | Release facts. |
| `database_patches` | MOS KB106822 / 888.1. Add the relevant proactive patch notes as separate checks for release-specific details. | Database and GI patch groups and packages. |
| `database_roadmap` | MOS PNEWS1360 / 742060.1 and applicable release milestones. | Release facts. |
| `exadata_patches` | MOS KB153930 / 888828.1, scoped to the documented software and packages. | Exadata software patches and full-stack bundles. |
| `exadata_availability` | Oracle's public availability notice for the exact builds. | Release facts. |
| `oda` | Public Oracle release index and captured patch tables or archived release notes. | Extracted from the checked capture. |
| `supercluster` | MOS QFSDP results or patch details for the reviewed hardware and software scope. | SuperCluster patch groups and packages. |

For ODA, set `result: "oda_capture"`. For the other roles, use
`result: "no_changes"` with no records or `result: "updated"` with one or more
records. Enter the fields from a catalog release or patch record and omit
`source`. The tool assigns provenance from the check's document identifier and
evidence hash. A patch record includes `line`, `kind`, and an exact `version` or
the original `source_name`. Include only packages whose number or exact version
and scope were actually verified. For a correction, use the same source ID and
record identity, and include the observed group-level fields. Multiple source
observations for a numeric version may remain in the catalog.

The commands validate the review structure and captured bytes. The reviewer
checks the article content and patch applicability before approving records.
Report a missing or conflicting source before publishing.
