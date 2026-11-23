# Gold Images and AutoUpgrade

The releases package reads patch metadata from caller-supplied inputs.
The application owns HTTP, Oracle Support authentication and file verification.

## Resolve a patch platform

Use `ResolvePatchPlatform(input, mosLabels)` before constructing an exact or
latest `PatchPackageFilter`. The resolver accepts canonical catalog labels and
the established Linux x86-64 and Windows x64 MOS tokens `226P`/`233P` (including
`226`/`233`). For other numeric tokens, supply the labels read from Oracle's
`plat_lang` options under the application's existing authenticated access.
A supported display label may include the Oracle filename-style suffix.
Unknown inputs return `ErrInvalidPatchPackageQuery`. A recognized token whose
label has no supported exact name returns `ErrPatchPackageUnavailable`.

A resolved name can still have no package for the chosen line, train and family.
The existing exact/latest selectors, their supported names, and the bundled
catalog bytes stay the same. Some historical records only carry `NOT_SET` or
`Platform Independent`. These records provide no exact-platform fallback.
A separately supplied patch number can still be verified in Oracle Support
against the requested platform and file.

## Find a Gold Image reference

`LoadGoldImages` takes a reviewed, versioned JSON snapshot. Call
`GoldImageCatalog.Lookup(GoldImageScope{Line, Train, Home})` with the exact
numeric train and the requested Database, Grid Infrastructure or Client home.
A missing section returns `ErrGoldImageCoverageUnavailable`. A reviewed empty
section returns an empty, non-nil `Rows` slice. Multiple references for the same
train and home return `ErrAmbiguousPatchPackage` at load time.

The November 9, 2026 capture of Oracle Support
[KB153394 (formerly 3053981.1)](https://support.oracle.com/support/?kmContentId=KB153394)
contains nine trains for each home, from 23.6 to 23.9 and 23.26.0 to 23.26.4.
Database and GI rows preserve the article's Cloud Services, Exadata and ODA
restriction (`EngineeredOrCloudOnly`) with no named platform. Client's
`AllPlatforms` flag preserves the Oracle table header. Every reference still
requires checking the precise platform and file using Oracle Support. For
example, the 23.9 Database, GI and Client identifiers are independent references.
The examined 18c, 19c and 21c captures establish no Gold Image section for this
workflow. CMAN and GSM are separate homes outside these selectors.

## Read AutoUpgrade metadata

Fetch `AutoUpgradeManifestURL` from Oracle's
[Database Upgrades page](https://www.oracle.com/database/upgrades/) and pass the
public JSON to `DecodeAutoUpgradeManifest`. The result identifies the manifest
version, exact Oracle JAR URL, filename, release timestamp, size, and SHA-256.
Use size and digest from the same observation when verifying the downloaded JAR.
The URL can serve a different release later. On November 22, 2026 the Oracle
manifest declared 26.7.2.

`LatestArchivedAutoUpgrade` reads a reviewed JSON archive of
[KB123450 (formerly 2485457.1)](https://support.oracle.com/support/?kmContentId=KB123450).
It selects the highest numeric build and returns its source, opaque attachment
reference and filename when the source identifies one. A versioned filename, if present, belongs to that build.
An empty archive returns `ErrAutoUpgradeUnavailable`; two records for the same
numeric build or one attachment reused for different builds are ambiguous.
An archive result has no public JAR URL. Use any archive digest only with the
attachment identified by the archive row. The current saved article lists
26.6.260925 but its separately linked checksum file was not captured. Older
25.6.251016 and 25.4.250730 records in the verified archive provide
attachment-specific checksums. Authenticated attachment retrieval is outside
the SDK and may be unavailable to a caller.

## Prepare a local review input

Use the project-owned private MOS evidence directory. Its `import-manifest.json`
identifies the dated capture directory containing `mos-capture-manifest.json`
and the archived AutoUpgrade HTML. Reuse the original reviewed commit, SHA-256,
and review timestamp when replaying the archive.

```powershell
python -B scripts/refresh_patch_tools.py --oracle-docs <capture-directory> --archive-file <archived-HTML> --archive-commit <recorded-commit> --archive-sha256 <recorded-sha256> --reviewed-utc <review-time-UTC> --output-dir <private-output-directory>
```

The output directory must already exist. The generator verifies the Oracle
capture manifest, archive identity and hash, article revisions, scoped table
headers, patch links and attachment identity before writing `gold-images.json`
and `autoupgrade-archive.json`. The previous `--archive-repo` option still
accepts the original Git archive instead of `--archive-file`. Keep review
inputs and generated Oracle Support snapshots outside the checkout. Recheck
platform/file availability and evidence scope when applying a capture.
