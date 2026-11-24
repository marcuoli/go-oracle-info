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
  identifies the exact published builds. The November 24 MOS capture supplies
  component package numbers and documented scopes for the 25.2, 26.1, and 26.2
  maintenance builds. Hardware applicability and cloud availability remain
  unqualified.
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

An empty `artifacts` array means the source provides no package records for
that patch group. For the 27 originally imported 11.2.0.4 Grid Infrastructure
PSU groups, a separate [Oracle Support observation](https://support.oracle.com/support/?documentId=KB111315)
supplies exact patch numbers and release dates. Platform fields remain empty in
these observations because the article links to Patch Details without listing
platforms for those rows. The original imported observations remain in the catalog.

The top-level snapshot date records when the seed was prepared. A source's
check date, article revision, and artifact release date are separate fields.
An absent date or package number is unresolved. A published `tbd`,
`not_available`, or `indefinite` milestone remains explicit. Coverage states
identify material read and gaps; they do not certify a complete Oracle
inventory, release availability on a live system, or patch eligibility.



## Select a patch package by release and platform

See [Gold Images and AutoUpgrade](patch-tools.md) for exact platform alias resolution, Oracle Gold Image references, and AutoUpgrade manifest and archive metadata.

Use `Catalog.ExactPatchPackage` and `Catalog.LatestPatchPackage` on a catalog
loaded with `releases.Load(releases.Database)`. The result identifies a package
number observed in the bundled snapshot. Live file resolution, authentication,
entitlement, and compatibility with an installation require separate evidence.

```go
catalog, err := releases.Load(releases.Database)
if err != nil {
    return err
}
filter := releases.PatchPackageFilter{
    Line: "19", Family: releases.DatabaseRU, Platform: releases.LinuxX8664,
}
pkg, err := catalog.ExactPatchPackage(filter, "19.33")
if err != nil {
    return err
}
fmt.Println(pkg.Artifact.Number, pkg.Version, pkg.Train, pkg.AsOf)
```

Supported lines are `18`, `19`, `21`, `23`, and `26`. Use
`PatchReleaseLine` to derive the line from a numeric train or full version
without checking package coverage. For 26ai, `23.26.4` maps to line `26`
and the incomplete `23.26` is invalid. For 23ai, `23.9` maps to line `23`.

An exact lookup of a train such as `19.33` or `23.26.4` selects the
highest eligible version of the requested family within that train. When a
version includes more numeric components, the lookup requires that exact
version. Each result's `Version` can be passed back to the exact selector.
Use `Train` to find an optional component for the same base release. For
example, the 19.33 JDK package has a later version qualifier than the DB RU.
Latest selects the highest numeric version with an eligible package for
that exact family and platform anywhere in the requested line. Retain the
primary result's train for companion selections to keep all packages aligned.

Platform input is an exact, case-sensitive MOS label. Use
`LinuxX8664` or `WindowsX64` where applicable. Other supported labels are
`Linux x86`, `Linux ARM 64-bit`, `IBM AIX on POWER Systems (64-bit)`,
`IBM: Linux on System z`, `HP-UX Itanium`, `Oracle Solaris on SPARC (32-bit)`,
`Oracle Solaris on SPARC (64-bit)`, and `Oracle Solaris on x86-64 (64-bit)`.
MOS UI tokens and the catalog's internal numeric platform IDs are separate
identifiers. A package must have the exact requested label and a positive
patch number. Platform Independent and unresolved platform rows cannot
satisfy a named-platform lookup.

| Family | Snapshot evidence and scope |
| --- | --- |
| `DatabaseRU`, `GridInfrastructureRU` | Explicit modern RU rows. Database RU selection is for Unix-like platforms. |
| `WindowsDatabaseBundle` | Dedicated Windows x64 DB Bundle Patch rows. The 19c snapshot ends at 19.32 for this family. |
| `DatabaseCombo`, `GridInfrastructureCombo` | Explicit source combo bundles, including 18c. |
| `DatabaseMRP`, `GridInfrastructureMRP` | Historical 19c Linux x86-64 records through 19.20. The retained current proactive article does not establish newer or Windows MRP coverage. |
| `OJVM`, `JDK` | Exact source component rows on named platforms. JDK has 19c and 21c entries. |
| `DataPumpBundle` | Exact Data Pump rows, including 19c Linux and Windows with distinct package numbers. The 23ai and 26ai observations include Linux x86-64 and do not establish Windows coverage. |

Disabled or restricted observations are ineligible. A negative observation
also excludes an otherwise matching observation of the same version and
package number. Equivalent observations at the selected version coalesce
by number, retain the first artifact, and resolve group, artifact, recorded,
and corroborating source references when present. `AsOf` is the catalog
snapshot date. The raw artifact retains the package's own release date and
recorded source identifier. `ErrInvalidPatchPackageQuery`,
`ErrPatchPackageUnavailable`, and `ErrAmbiguousPatchPackage` distinguish
malformed requests, absent eligible packages, and competing numbers at the
selected version. All original raw rows remain accessible through `Load`.

## Look up reviewed patch recommendations

The `releases.LoadPatchRecommendations(reader)` API reads a versioned,
caller-supplied JSON snapshot. The caller obtains the approved snapshot and
verifies it against the review receipt. For example, look up
`releases.PatchRecommendationScope{Line: "19", Train: "19.28", Home: releases.DatabaseHome}`
using the catalog's `Lookup` method.

A successful lookup returns source-ordered rows with the listed patch number,
fixed-in RU and MRP text, and any reviewed operating-system family restriction.
An unspecified platform family leaves applicability unresolved. Each set
retains the document identifier, stable Oracle link, and original capture hash.
Use the captured timestamp and source revision when present. For archived
HTML without a verified capture timestamp, the Git commit identifies the
retained bytes.

The caller can distinguish a reviewed empty table from a missing section.
An empty table returns a non-nil empty `Rows` slice. An absent exact
train and home returns `ErrPatchRecommendationCoverageUnavailable`.
Invalid scopes and snapshots return `ErrInvalidPatchRecommendations`.
Use a canonical train such as `19.28` or `23.26.4` as the scope.
Fixed-in text and platform restrictions remain attached to the source rows.
A package filename, applicability to a particular installation, entitlement,
and download verification are separate Oracle Support operations.

### Prepare the private snapshot

Use the project-owned private MOS evidence directory for KB188772 and
archived Doc ID 555.1. Its `import-manifest.json` identifies the dated
captures, archived HTML and reviewed replay inputs. Record a version 1
review with a `sources` array. Each entry contains `source.document`
with the existing `Source` fields and one evidence input:

- An absolute `capture` path to MHTML and its `source.captured_utc`.
- An `archive.file` path to the local archived HTML and its original
  `source.archive_commit`. Existing reviews may also identify the Git archive
  with `archive.repository` and `archive.path` (`assets/docs/555.1.html`).

Review each `sections` entry by exact `line`, `train`, `home` and
`expected_rows`. Preserve the reviewed source identity, scope, SHA-256,
and check date. On another machine, point the review paths at that machine's
private evidence copy.

```powershell
python -B scripts/refresh_patch_recommendations.py --review <private-review.json> --output <private-recommendations.json>
```

The command verifies the capture and extracts only structured facts from
the reviewed tables. The snapshot contains one record per approved train
and home under `sections`, with `scope`, `source`, and `rows`. Keep the
snapshot, review, and Oracle captures on the approved private evidence route.
Keep the original article and credentials out of SDK source and release
artifacts. Public distribution of private recommendation data needs its own
scope review.

## Refresh before publish

1. Record the selected module version and the exact canonical main commit as
   `history_base`. Review the required Oracle authorities using the available
   Oracle web materials and verified original Oracle Support captures, including
   copies retained in the project-owned private MOS evidence directory. Check the relevant product,
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

An Oracle support announcement with an explicit MOS patch link can establish
the patch facts shown publicly. Retain the announcement URL, linked MOS
reference and exact observed scope in the evidence. A historical announcement
can corroborate its named patch and scope. Latest availability and broader
hardware or platform coverage require their own evidence.

The commands validate the review structure and captured bytes. The reviewer
checks the article content and patch applicability before approving records.
Report a missing or conflicting source before publishing.
