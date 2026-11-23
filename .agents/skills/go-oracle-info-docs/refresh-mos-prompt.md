$go-oracle-info-docs

Refresh go-oracle-info’s Oracle evidence and regenerate all seven JSON files. Use the existing signed-in MOS browser session and the project’s local private evidence directory. Work directly from Oracle sources and the local archives, without Harvester.

1. Record the exact canonical main commit as `history_base` and the starting Git status. Read `AGENTS.md`, `docs/release-catalog.md`, `docs/patch-tools.md`, and the private `import-manifest.json`. Use the intended module version if one is already set. Otherwise, use the next unused stable patch version for the catalog preview only.

2. Save fresh MOS captures in a new dated directory outside the Git checkout. Review the relevant tables and patch details for KB106822 / 888.1, the applicable Database and GI proactive patch notes, PNEWS1360 / 742060.1, KB153930 / 888828.1, and the relevant SuperCluster QFSDP entries. Also capture KB188772 / 555.1 for recommendations, KB153394 / 3053981.1 for Gold Images, and KB123450 / 2485457.1 for AutoUpgrade. Check Oracle’s public Lifetime Support Policy, Exadata availability notices, and ODA release documentation.

3. Record each source’s title, current ID, legacy Doc ID, Oracle URL, revision, actual capture and review times, SHA-256, and exact release, home, platform, and restriction scope. Use the existing manifest and review schemas. Preserve the old captures and archived HTML with their original hashes, dates, and archive commits. If a source is inaccessible or a parser rejects a changed article layout, report the exact gap and keep the last verified data for that scope.

4. Generate the four bundled-catalog previews outside Git. Use `scripts/capture_oda_release_sources.py` for the public ODA evidence and run `scripts/refresh_release_catalog.py refresh` with the version-bound private review, the ODA capture directory, and an external preview directory. Generate `database.json`, `exadata.json`, `oda.json`, and `supercluster.json`. Compare them with the committed versions and preserve historical records.

5. Generate the three private feeds in a separate external directory. Run `scripts/refresh_patch_recommendations.py` with the reviewed MHTML captures and local `archive.file` entries for historical 555.1 HTML to generate `recommendations.json`. Run `scripts/refresh_patch_tools.py` with the new capture directory and its `mos-capture-manifest.json`, plus `--archive-file` and the recorded commit, SHA-256, and review time for the historical AutoUpgrade HTML. Generate `gold-images.json` and `autoupgrade-archive.json`. Preserve exact train and home coverage, explicit empty sections, deployment restrictions, and attachment-specific checksums.

6. Run `python -B -m unittest discover -s scripts -p 'test_*.py'`. Report the absolute path and SHA-256 of each of the seven outputs, what changed, and any gaps in the evidence. Verify the Git working tree and index match their starting state.

Keep all captures, archived HTML, manifests, review files, and private feeds outside Git and release artifacts. Leave the four bundled catalogs as previews for a separate reviewed PR. This run is for capture, regeneration, and validation only.
