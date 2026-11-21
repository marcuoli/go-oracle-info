# Oracle release and patch catalog

`releases.Load(releases.Database)`, `releases.Load(releases.Exadata)`, and
`releases.Load(releases.ODA)` read the snapshots bundled in
[`releases/data`](../releases/data). All catalog JSON files live in that
directory. The loader reads embedded data and does not open a database
connection, query Oracle Support, or select an upgrade target.

## Evidence and scope

- **Database and Grid Infrastructure:** the Harvester portable catalog from
  Django revision `8df730c03b3ddd86757e98371319172acad24def` (exported November 9,
  2026), the independently published
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

The top-level snapshot date records when the seed was prepared. A source's
check date, article revision, and artifact release date are separate fields.
An absent date or package number is unresolved. A published `tbd`,
`not_available`, or `indefinite` milestone remains explicit. Coverage states
identify material read and gaps; they do not certify a complete Oracle
inventory, release availability on a live system, or patch eligibility.

## Refresh before publish

1. Recheck the Oracle Lifetime Support policy, the saved MOS articles using
   the Harvester refresh workflow, and public Exadata and ODA release
   documentation. Reconcile any new source conflicts and preserve original
   article IDs and verification dates. Review the curated public lifecycle
   table and Exadata announcement data in the seed script if Oracle changed
   them.
2. Use the reviewed Harvester export and matching source manifest. Capture
   ODA index and per-release pages using the companion script, with an
   explicit evidence destination on V:. The capture script reads Oracle
   public documentation only.
3. Regenerate on V: with the same three inputs and inspect the diff against
   `releases/data`. Run `go test ./releases` and normal project validation.
   Promote only the reviewed JSON into the package. Keep raw downloads and
   private evidence outside source control.

```powershell
python -B scripts/capture_oda_release_sources.py --output-dir <V-oda-evidence-dir>
python -B scripts/seed_release_catalog.py --harvester-docs <harvester-assets-docs> --harvester-revision <reviewed-commit> --oda-captures <V-oda-evidence-dir> --output-dir <V-preview-dir> --as-of <YYYY-MM-DD>
```

Oracle Support source captures are read as local inputs, and neither script
copies whole articles, download links, or account data into the catalog.
Public distribution remains a separate gate.
