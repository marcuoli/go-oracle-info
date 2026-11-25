Use $go-oracle-info-docs to capture evidence for all 41 remaining historical
package gaps in go-oracle-info issue #174 and independently verify the
release date of Oracle Database 21.4 Windows patch 33322894.

Issue
https://git.lan.orcl.com.br/marcuoli/go-oracle-info/issues/174

Use the existing signed-in My Oracle Support browser session. If MOS
requires authentication, let me complete it and then resume. Use only
read-only MOS actions. Save Patch Details and READMEs, without downloading
patch binaries or installing anything.

Use the existing go-oracle-info checkout at
I:\SynologyDrive\Trabalhos\Desenv\go\go-oracle-info if it is accessible.
Verify it with git rev-parse --show-toplevel and confirm that go.mod
declares github.com/marcuoli/go-oracle-info. Read AGENTS.md and the
repository's .agents\skills\go-oracle-info-docs\SKILL.md if the skill
isn't installed on this machine. If the checkout is unavailable, continue
with the explicit checklist below. This is a capture and verification
task. Leave the checkout, catalog JSON, issue, and release refs unchanged.

Save everything in a new dated subfolder of
I:\QnapQSync\Docs\refresh
named issue-174-YYYYMMDD-HHmmss. Verify the destination is outside every
Git checkout. Preserve earlier captures and archived HTML. Keep all raw
captures, manifests, and review files out of Git, both forges, and release
artifacts. Keep credentials, account details, tokens, and cookies out
of the saved files and reporting.

Start with patch 33322894
https://support.oracle.com/support/?patchId=33322894

Save Patch Details and the linked README for the exact 21.4 Windows
Database Bundle Patch. Record the full version, exact platform, filename,
and any explicitly stated original release or first-availability date.
Record any reissue date, README revision date, or last-updated date
separately. Compare them with the 19 October 2022 date shown for 33322894
in both saved copies of KB117472. The same table places the subsequent
21.5 entry in January 2022. Use only evidence tied to the exact Windows
package to settle the date. If MOS shows only a build or update date,
or the available evidence still conflicts, leave the release date
unverified and record what is missing.

MOS article references
- KB111315 / 2285559.1 — 11.2.0.4 Proactive Patch Information
- KB106910 / 2285558.1 — 12.1.0.2 Proactive Patch Information
- KB150224 / 2285557.1 — 12.2.0.1 Proactive Patch Information
- KB910370 / 2369376.1 — 18c Proactive Patch Information
- KB111276 / 2521164.1 — 19c Proactive Patch Information
- KB117472 / 2796590.1 — 21c Proactive Patch Information
- KB153930 / 888828.1 — Exadata supported versions and patch history
- KB117594 / 1569461.1 — SuperCluster QFSDP contents and model scope

Open documents using https://support.oracle.com/support/?documentId=<KB_ID>
and known patches using https://support.oracle.com/support/?patchId=<NUMBER>.
For a candidate package, save the Patch Details and README together. Keep
the exact product, version, patch family, DB or GI home, platform or
hardware models, and package number. Preserve each platform package
separately. Capture relevant footnotes and replacement notices. A
successful search with no matching result is unresolved. An explicit
Oracle statement that a patch was skipped, suspended, replaced, or never
issued is evidence of that status.

41-group checklist

11.2.0.4 — 20 groups
1. Windows DB Bundle Patches (11). Check the Quarterly Database Patch for
   Windows table in KB111315 and its footnotes for suffixes 230117, 230418,
   230718, 231017, 240116, 240416, 240716, 241015, 250121, 250415, 250715.
   The saved 230117 row indicates no patch was issued. Capture exact
   Windows package evidence for any of the later versions MOS identifies.
2. GI Combo (1), 11.2.0.4.260120. Check KB111315's GI COMBO section and
   patch 38658548. The saved row identifies it as an OJVM component PSU.
   Look for an exact README listing both the GI and OJVM components.
3. Exadata Database SPU (1), 11.2.0.4.210119EXADBBP. Check KB111315's
   Exadata DB SPU section for a standalone SPU tied to the January 2021
   Exadata DB BP. Preserve the distinction from the QFSDP and the combo.
4. Exadata QFSDPs (6). Check KB111315 and KB153930 for suffixes 200714,
   210420, 210720, 211019, 220118, 220419. Save the original bundle README
   and exact included Database component where available. The saved July
   2020 row says replaced, and the other five lack a package number.
5. SuperCluster QFSDP (1), 11.2.0.4.180116. Capture KB111315 and KB117594
   plus both READMEs, 26635432 and 27011128. Resolve the competing numbers
   using the exact release, supported models, and component levels.

12.1.0.2 — 2 groups
1. Database Proactive BP (1), 12.1.0.2.200414. Capture the April 2020 row
   in KB106910 and any archived README for the original bundle. Verify
   patch 31307682's own version and replacement relationship.
2. Exadata QFSDP (1), 12.1.0.2.200714. Capture the replaced July 2020 row
   in KB106910 and the original README or KB153930 notice explicitly
   naming the 12.1.0.2 component.

12.2.0.1 — 11 groups
1. Database RURs (3), suffixes 210119, 210420, 210720. Capture the DB RUR
   rows and Limited Error Correction footnote in KB150224. Check
   KB852400 for each original RU and revision, and capture any exact
   issued RUR README.
2. GI RURs (4), suffixes 171017, 210119, 210420, 210720. Use the GI RUR
   section of KB150224 and the GI fixes list KB153321. Prioritize the
   July 2017 GI revision dated 17 October 2017, whose saved link is blank.
3. JDK Bundle Patch (1), 12.2.0.1.251021. Check the October 2025 JDK
   section and exact JDK README with platform. The saved October DB RU
   is separate.
4. Exadata QFSDP (1), 12.2.0.1.200714. Capture the replaced July 2020
   row and original README or replacement notice tying the original
   bundle to 12.2.0.1. Use KB150224 and KB153930.
5. SuperCluster QFSDPs (2), 12.2.0.1.190716 and 12.2.0.1.200814.
   For July 2019, capture patch 29699004 and KB117594 note 9 about the
   delayed Q2 bundle replacing Q3. For August 2020, look for an exact
   SuperCluster README. The saved 31754141 row is Exadata.

18c — 5 groups
1. Exadata QFSDP (1), 18.11.0.0.200714. Capture the replaced July
   2020 row in KB910370 and the original README or KB153930 notice.
2. SuperCluster QFSDP (1), (Q3.2019).0.190716. Capture the 18c row in
   KB910370, patch 29699004, and KB117594 note 9. Establish the status
   of Q3 and its relationship to replacement patch 29252451.
3. SuperCluster QFSDP (1), (Q4.2019).0.191015. Capture both READMEs,
   30133707 and 30133720, plus the KB910370 and KB117594 rows. Compare
   models, exact included GI level, and date.
4. SuperCluster QFSDPs (2), 18.2.0.0.0 and 18.2.0.0.180417. Capture
   the KB910370 April 2018 SuperCluster note stating no bundle was
   issued for this line. Tie that disposition to both keys.

19c — 2 groups
1. SuperCluster QFSDP (1), 19.3.1. Compare KB111276 and KB117594.
   Capture the 29865182 README if available to verify its Exadata scope.
   Look for a distinct SuperCluster package or an explicit association.
2. GI RU (1), 19.3.0.0.190416. Capture the KB111276 GI RU suspension
   row and any original GI README, withdrawal, or replacement notice.
   Record the exact platform and package status.

21c — 1 group
1. Windows Database Bundle Patch (1), 21.5.0. Capture the KB117472 row
   documenting skipped patch 33589769 and Oracle's direction to use
   33829143 for 21.6. Preserve that replacement's separate version.

Deliver a private capture manifest and a private gap review in the same
dated folder. The manifest should record the relative filename, SHA-256,
Oracle title and URL, current and old document IDs where available,
revision, and actual capture timestamp. Use the existing skill's
manifest format if available.

The gap review should have exactly 41 group records plus a separate
33322894 date record. For each group, retain the exact catalog key,
outcome, patch number when verified, exact scope, any dates or
restrictions, and the supporting capture filenames and hashes. Use
outcomes such as confirmed issued, documented not issued, skipped,
suspended, superseded, wrong product or platform, conflicting,
access blocked, or unresolved. Cite the specific row, footnote, or README
section. Reuse a capture across groups when it supports each one's scope.

Before reporting completion, open the saved files, verify their hashes,
confirm the counts 20 + 2 + 11 + 5 + 2 + 1 = 41 and one separate date
record, and confirm the captures are outside Git. Report the exact
output directory, totals by outcome, the verified release date or
remaining blocker for 33322894, and the exact evidence still needed.
