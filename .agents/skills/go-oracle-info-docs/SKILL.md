---
name: go-oracle-info-docs
description: Update go-oracle-info documentation, Oracle release catalogs, or private Oracle patch metadata feeds. Use with Oracle public documentation, project-owned MOS evidence, or an already signed-in My Oracle Support (MOS) session.
---

# Update go-oracle-info documentation and catalogs

Find the go-oracle-info checkout by its `github.com/marcuoli/go-oracle-info` module path. Read its `AGENTS.md` and use the runbook for the requested output below. Use the checkout or release tag the user wants documented. If they ask about a published API, verify it at the published tag.

## Review Oracle evidence

- When the user is signed in to MOS and the task needs it, use the existing authorized session on that machine. Open the actual article or patch detail and review the relevant sections. Use the relevant article text to support each technical claim. Keep the operation read-only in MOS and leave authentication to the user. Keep credentials, cookies, tokens, and account details out of captures and logs.
- Start with Oracle's published documentation and the go-oracle-info private evidence directory. On the original Windows machine it is `V:/Development/go-oracle-info/artifacts/oracle-support`. On another machine, use the authorized local copy. Read `import-manifest.json` for the retained captures, archived HTML, hashes, and replay inputs. Check article identity and scope using the dated capture manifest and original archive provenance. Save any fresh MOS captures in a separate dated directory. If an article is inaccessible, continue with applicable evidence and identify the exact missing claim.
- Record title, stable Oracle URL, current article ID, old Doc ID when known, Oracle revision, original capture time, current check time, and SHA-256 of any retained evidence. Match the exact product, numeric release or RU, component/home, platform, and documented restrictions. For query claims, check the required privileges, database/container state, and instance scope where applicable. Preserve distinctions between historical observations, current applicability, and live package availability.
- Keep full articles, private inputs, and normalized private feeds outside Git and published modules. Put retained evidence in the project's approved artifact storage. Carry only the scoped, attributable facts permitted in the requested output.

For a signed-in MOS refresh of all seven JSON files, copy the [reusable refresh prompt](refresh-mos-prompt.md).

For the remaining Database catalog gaps, copy the [issue #174 MOS capture prompt](capture-issue-174-prompt.md).

## Choose the repository workflow

| Requested output | Start here |
| --- | --- |
| README, guide, API examples, or reference corrections | Inspect the existing doc and its Go API or SQL source. Use `docs/developer-guide.md` for fact APIs and `docs/fact-api-compatibility.md` for query qualification. Preserve the existing wording and structure outside the requested change. Separate documented applicability, static tests, native SQL execution, and driver qualification. |
| Bundled Database, Exadata, ODA, or SuperCluster metadata | Follow `docs/release-catalog.md`, especially **Refresh before publish** and **Direct Oracle review**. Use `scripts/refresh_release_catalog.py` to build a preview from the exact `history_base`, private review, and verified ODA input. Compare all four generated catalogs and preserve historical records and original capture dates. Record a scoped no-change finding when the evidence supports it. |
| Private recommendations feed by release train and home | Follow **Prepare the private snapshot** in `docs/release-catalog.md`. Verify KB188772 (old Doc ID 555.1), then run `scripts/refresh_patch_recommendations.py` with the local review, using `capture` for MHTML and `archive.file` for archived HTML. Preserve explicit empty sections and missing coverage, kept separate from live platform eligibility. |
| Private Gold Image or AutoUpgrade archive feed | Follow `docs/patch-tools.md` and `scripts/refresh_patch_tools.py --help`. Use `--oracle-docs` for the dated local captures and `--archive-file` for the reviewed historical AutoUpgrade HTML. Review KB153394 (3053981.1) and KB123450 (2485457.1) as applicable. Preserve Gold Image home, release train, and deployment restrictions. Bind an AutoUpgrade digest only to its exact file or attachment. The public AutoUpgrade JSON manifest is separate from archived MOS metadata. |

Use source previews and the owning validation commands before integrating repo changes. For facts in a private feed, verify representative lookups against the reviewed input and record coverage gaps. Report changed documents or generated files, exact source scope, validation passed, and unresolved evidence.

If the user requests publication, also follow `docs/mirroring.md` and the `save-publish` skill. The module requires a fresh, version-bound applicability review. Reuse applicable verified Oracle web or local MOS evidence with the original capture dates. Obtain a new capture when a required claim needs it. Use the project's `scripts/publish_module.ps1 -Action Check` before tag or release actions. Treat publication as a separate requested outcome.
