# Independent connected identity properties

These additive Oracle11+ APIs own small read sequences on an already-admitted
identity.Reader. They open no connection, change no session/container, choose no
driver/credentials, reconnect, cache, recover or acquire another scope. Each
property is independent; no whole snapshot is required. The caller owns target
admission, grants/open-state applicability, deadlines, complete cursor results,
row/byte budgets and application acceptance. Existing selectors/decoders and
their historical contracts are unchanged.

Run the connection-free example with:
    go run ./examples/identity-properties

It uses labeled fixtures, not a native database. It checks errors before State,
Precision and Fallback. In particular, a retained legacy value after a failed
full-version query must not be mistaken for successful full-version evidence.

## Calling contract and attempts

- ReadVersion(ctx, reader, VersionReferences{Legacy, Full})
- ReadArchitecture(ctx, reader, ArchitectureReferences{Version, Container})
- ReadEdition(ctx, reader, EditionReferences{Version, Code, Banners})

Every reference is caller-namespaced, valid and pairwise distinct within the
call, including unused compatibility roles. All are checked before any query.
No references are allocated by the library. Calls use the original context and
reader. A canceled context stops the next read; cancellation observed when a
query returns rejects its payload. Query/decoding failure never starts another
query on a reader that might have been retired. These properties do not make
separate calls an atomic database snapshot or prevent external session changes.

PropertyEvidence retains State, Source, Fallback and ordered PropertyAttempt
values. Available, SQL Null, successful NoRows and Unavailable stay distinct.
Actual query attempts retain their source, state and safe error. Scalar source
strings and independent raw architecture/edition fields remain available in the
typed observations. Successful Result shapes are validated before interpretation.
No Result can reconstruct a discarded truncation/finalization error: the reader
must never supply failed cursor prefixes as complete.

PropertyError.Error contains static source/operation text, not the driver message.
Unwrap intentionally preserves the original cause for errors.Is/As and explicit
private diagnostics. Structured source/operation remain inspectable. Treat every
non-nil returned error as unresolved, even when an independently valid legacy
version remains available. Reader errors (including joined no-row/terminal
errors) remain errors; successful zero-row Results represent NoRows. The library
does not log errors or echo private transport details.

## Numeric version

ReadVersion bootstraps V$INSTANCE.VERSION using the existing low-level projection.
It validates exactly five unsigned decimal segments without padding, trimming or
marketing-name conversion. On an established18+ source it reads VERSION_FULL
once and prefers that actual value, with FullPrecision and its actual source.
Oracle26ai branding can accompany numeric23.26.x; it never produces synthetic26.

A successful modern NULL/no-row reuses the validated bootstrap with
Fallback=true and LegacyPrecision. A modern query/projection failure also retains
the independent legacy value with that metadata, while returning the failure
and recording it in Attempts. No new query implements that fallback.
An invalid/contradictory decoded full numeric value leaves the property
Unavailable, preserving raw Legacy and Full evidence with an error. A differing
major is never silently selected. Full is nil without a decoded full observation;
Attempts distinguish a failed attempted read from an unselected legacy source.

LegacyPrecision describes the observed VERSION column, never an observed RU or
VERSION_FULL. It does not imply an installed-patch inventory or support decision.

## Connected architecture

ReadArchitecture bootstraps only the release capability. On12+,
ConnectedArchitectureSQL reads parent V$DATABASE.CDB and current-session
SYS_CONTEXT CON_ID/CON_NAME in one projection. Dictionary row CON_ID is not used
as a session-container identity. The parent CDB flag remains separate:
- YES, ID1, CDB$ROOT: connected CDB root.
- YES, ID2, PDB$SEED: connected seed PDB.
- YES, ID>2, nonroot/nonseed name: connected PDB.
- NO, ID0, nonroot/nonseed database name: NON-CDB.

All signals must be available/nonempty. Missing signals remain Unavailable,
with their exact NULL/no-row/raw fields preserved. Contradictory or invalid flags,
IDs or root/seed identities additionally return ErrInvalidEvidence. Never infer
a connected CDB merely from its parent CDB=YES or a missing PDB signal.

On11g, NON-CDB follows the documented pre-multitenant release constraint.
Method=LegacyReleaseArchitecture and Fallback=true make that basis explicit;
DatabaseCDB, ContainerID and ContainerName stay Unavailable, not fabricated
dictionary values. A missing/invalid bootstrap cannot establish architecture.

## Edition/product classification

ReadEdition uses raw V$INSTANCE.EDITION on12+. Documented explicit codes map to
EE, SE, XE, FREE, PO, HP, XP, CORE EE and CORE SE typed values. Historical12.1
CORE codes remain distinct. SE2 maps to StandardEditionFamily (SE_OR_SE2):
Oracle documents this code for Standard Edition or Standard Edition2, so it
cannot alone establish which of those products was observed. Code retains SE2
exactly. No inferred human-readable label replaces an absent observed label.

Unknown codes remain raw but Unavailable with ErrUnknownEdition. Modern NULL,
no-row, failure or unknown code never triggers speculative banner fallback.
An observed edition/product is not a licensing grant or feature entitlement.

On11g only, the existing conservative LegacyEditionDescription recognizes one
explicit Enterprise/Standard/Express label from complete raw database banners.
The property uses ExplicitLegacyLabel precision and Fallback=true, preserves the
label and all raw banners, and leaves Code Unavailable. No EDITION code is
fabricated. Duplicate/ambiguous/unknown labels stay unavailable. The complete
legacy source is limited to16 rows; malformed/oversized results return no prefix.

## Oracle sources and qualification

Checked2026-11-14:
- V$INSTANCE11.2, legacy VERSION and absence of documented EDITION:
  https://docs.oracle.com/cd/E24693_01/server.11203/e24448/dynviews_2002.htm
- V$INSTANCE12.1, historical observed EDITION codes:
  https://docs.oracle.com/database/121/REFRN/GUID-6A0C9B51-1714-4223-B166-9D54C4E65D67.htm
- V$INSTANCE18, VERSION_FULL boundary:
  https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-INSTANCE.html
- V$INSTANCE19/26, current codes and SE2 ambiguity:
  https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-INSTANCE.html
  https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-INSTANCE.html
- V$DATABASE19 and SYS_CONTEXT19, parent CDB and connected context semantics:
  https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-DATABASE.html
  https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/SYS_CONTEXT.html
- Oracle12.1 New Features, multitenant introduced12.1:
  https://docs.oracle.com/database/121/NEWFT/chapter12101.htm
- Oracle26ai Upgrade Guide, numeric release segments and23.26 examples:
  https://docs.oracle.com/en/database/oracle/oracle-database/26/upgrd/oracle-database-release-numbers.html

Explicit legacy banner example references remain in edition-version-registry.md.
Documentation/fixture checks do not establish native release, driver, grants,
container/open-state, operating-system or licensing qualification. Any native
observations must name their exact transport, release and scope separately.

## Bounded native projection and recorded-API check

On2026-11-14, the original SELECT projections succeeded through SQL*Plus client
CSV on existing Oracle26ai Enterprise23.26.1.0.0 primary root, its open PDB, and
mounted physical standby root. VERSION remained23.0.0.0.0, VERSION_FULL was
23.26.1.0.0, EDITION wasEE; parent CDB=YES coexisted with connected root1 or PDB3.
The stopped11g fixture was left stopped;11g paths retain fixture qualification.
No Free container, database state, apply state or grants were changed.

The private captures are replayed through all three Go property readers by
TestRecordedIdentityProperties under the integration build tag and explicit
ORACLE_INFO_IDENTITY_REPLAY path. This checks actual captured result shapes and
the API's values/provenance. It is connection-free replay, not native Go driver,
cursor-finalization or restricted-grant qualification. Full native reader
qualification remains with an admitted consumer adapter.

An initial XML serialization envelope failed on the mounted standby because
SYS.XMLTYPE was unavailable there. Replacing that probe envelope with client CSV
let the unchanged fixed-view SELECTs execute. This was instrumentation evidence,
not a property-source fallback or a reason to open the standby.
