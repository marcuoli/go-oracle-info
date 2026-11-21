# Oracle release and patch catalog

`releases.Load(releases.Database)`, `releases.Load(releases.Exadata)`,
`releases.Load(releases.ODA)`, and `releases.Load(releases.SuperCluster)` read the snapshots bundled in
[`releases/data`](../releases/data). All catalog JSON files live in that
directory. The loader reads embedded data and does not open a database
connection, query Oracle Support, or select an upgrade target.

## Evidence and scope

- **Database and Grid Infrastructure:** the local patch metadata export (generated November 9, 2026), the independently published
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

1. Select the new module version and record the exact current canonical main
   commit as `history_base` before starting the refresh. Review the applicable
   sources and applicability. Obtain a fresh `patch-catalog.json` and the
   corresponding `mos-capture-manifest.json` and article files in the patch input
   directory. Verify the captured articles, including the
   Database quarterly patch note (KB106822 / 888.1), release schedule
   (PNEWS1360 / 742060.1), and Exadata supported versions article
   (KB153930 / 888828.1). Use authenticated MOS to verify SuperCluster QFSDP
   availability for the applicable product and release. Verify Oracle's public
   lifecycle, availability, and ODA release documentation as well. Keep original
   IDs and exact scope, and investigate source conflicts before approving inputs.
2. Capture the ODA index and release pages in a fresh V: evidence directory.
   Make a private `review.json` for the selected version in that release's
   evidence directory. Bind it to the exact local patch export, MOS capture manifest,
   ODA manifest and actual Oracle evidence. A reviewer checks document contents,
   patches, versions, platform/model scope, and dates. The script checks the
   evidence files and hashes and cannot replace that review.
3. Run `refresh` and review the generated changes. Merge them via a PR alongside
   the release preparation. The reconciler retains dropped release, patch and
   platform rows with their original source snapshots. Changed metadata for the
   same identity retains the previous observation under `history`. Review each
   correction against its official scoped evidence.
4. Run `scripts/publish_module.ps1 -Action Check` on synchronized canonical main.
   The command rebuilds the exact four files using the reviewed inputs and
   rejects stale evidence, missing scopes, an intervening catalog change, or a
   committed byte mismatch. The export, ODA capture, three captured MOS
   authorities, and every review check must be within 24 hours. Even a
   no-change result must be reviewed for the new module version. Use the
   [gated publication actions](mirroring.md) for tag, mirror, and release
   operations. The receipt is stored outside Git.

```powershell
python -B scripts/capture_oda_release_sources.py --output-dir <V-oda-evidence-dir>
python -B scripts/refresh_release_catalog.py refresh --version v0.X.Y --review <V-review.json> --patch-inputs <patch-input-directory> --oda-captures <V-oda-evidence-dir> --output-dir <V-preview-dir>
./scripts/publish_module.ps1 -Action Check -Version v0.X.Y -Review <V-review.json> -PatchInputs <patch-input-directory> -OdaCaptures <V-oda-evidence-dir> -EvidenceDirectory <V-release-evidence-dir>
```

The review JSON is a private receipt input using `schema_version: 1`, `version`,
`history_base`, `input_revision`, `reviewed_by`, `reviewed_utc`,
`patch_export_sha256`, `mos_manifest_sha256`, `oda_manifest_sha256`
and a `checks` array. Use full Git commit IDs and ISO 8601 timestamps with offsets.
Each check has `role`, `status: "verified"`, `title`, `identifier`,
`old_doc_id` when proven, `url`, `scope`, `finding`, `checked_utc`, `evidence`
and `evidence_sha256`. Evidence filenames are relative to the review JSON
directory and their bytes must match the reviewed hash. For the three MOS
articles in the patch input directory, the current/old IDs, title and evidence bytes must
match the capture manifest.

| Role | Evidence for the check |
| --- | --- |
| `database_lifecycle` | Oracle Lifetime Support Policy and its review of the supported dates. |
| `database_patches` | Captured MOS KB106822 / 888.1, scoped to relevant release lines and patch delivery. |
| `database_roadmap` | Captured MOS PNEWS1360 / 742060.1, including applicable release milestones. |
| `exadata_patches` | Captured MOS KB153930 / 888828.1 for component patch history and scope. |
| `exadata_availability` | Oracle's published availability evidence for new exact builds. |
| `oda` | Oracle public release and patch tables, including scope distinctions. |
| `supercluster` | MOS SuperCluster QFSDP search or patch evidence for the admitted hardware/software scope. |

Report a missing or conflicting check before proceeding. Update curated source
facts in the seed script when Oracle changes them. If a source no longer lists
a verified old record, retain it through reconciliation. Review private inputs
outside source control; the output contains normalized metadata only.
