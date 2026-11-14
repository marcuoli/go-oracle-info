# Observed edition, versions and dictionary components

These APIs select read-only facts in the caller-admitted current session/container.
They do not connect, choose credentials/drivers, change containers, grant access,
set NLS values or decide whether a destination is acceptable. A registered component
or edition code is not a licensing assertion. Native Enterprise evidence must be
obtained on Enterprise; observations from Free do not qualify another edition.

## Identity

- Reuse `identity.VersionStatement(major)` with `DecodeVersion`: known 18+ selects
  `V$INSTANCE.VERSION_FULL`; 11g/12c use raw `VERSION`. Unknown release zero uses
  the existing bootstrap projection. No legacy value is padded into an RU.
- Existing `identity.FullVersionSQL` remains unchanged. `DecodeFullVersion` accepts
  its unaliased `VERSION_FULL` result. Do not use it before support is established.
- `identity.EditionStatement(major)` selects raw `V$INSTANCE.EDITION` on known 12+;
  `DecodeEdition` retains any code, SQL NULL or authoritative no row. The reviewed
  11g dictionary does not expose EDITION; the selector rejects that source instead
  of guessing a code from marketing/version text. Preserve its banner separately.
- `identity.BannersStatement(major)` selects `BANNER` on 11+ and additionally
  `BANNER_FULL` on 18+. `DecodeBanners` preserves every complete source row, including
  duplicates and multiline banners. `VersionBanner.Full == nil` means unselected;
  a non-nil `Text` with `State == Null` means selected SQL NULL.

Release arguments are database binary major versions, not marketing labels; for
example an observed `23.*` source can carry 26ai branding. Preserve both source
strings. Capability selection is separate from the native applicability of grants,
open state and the connected container. Choosing a legacy projection after missing
or inaccessible modern evidence is caller policy, not an automatic retry here.

## Registry

`registry.Statement(major)` selects COMP_ID, COMP_NAME, VERSION, STATUS and MODIFIED
from DBA_REGISTRY on 11+, plus VERSION_FULL on 18+. `registry.Decode` returns one
`Component` per row with raw nullable strings. A nil `VersionFull` pointer means the
column was deliberately not selected; a non-nil invalid `sql.NullString` means SQL
NULL. Empty non-NULL text remains valid empty text. VERSION and VERSION_FULL remain
separate; neither version is synthesized from the other. Unknown statuses and
repeated/conflicting IDs survive without filtering or winner selection.

MODIFIED is a VARCHAR2 source label, not a DATE/TIMESTAMP. It is retained byte for
byte, including source-local formatting; no timezone, precision, parse format or
UTC conversion is inferred. Query the admitted current container only; this is
not CDB-wide coverage or a proven RAC inventory.

All decoders require complete results after terminal cursor/close checks. The
multirow decoders validate the entire metadata/row shape before returning facts;
a malformed late row cannot become an apparently complete prefix. A successful
empty source has zero rows. Permission failures, unavailable dictionary access
while MOUNTED, canceled reads and capacity/truncation failures remain errors with
the caller; never turn them into a successful empty component table. Separate
queries let a failed registry read coexist with valid instance/banner observations.

## Oracle references

Checked 2026-11-14. These references qualify static column selection, not live
Oracle release/RU/platform, grant, open-state or driver qualification.

- [V$INSTANCE 11.2](https://docs.oracle.com/cd/E24693_01/server.11203/e24448/dynviews_2002.htm): legacy VERSION; no documented EDITION column.
- [V$INSTANCE 12.1](https://docs.oracle.com/database/121/REFRN/GUID-6A0C9B51-1714-4223-B166-9D54C4E65D67.htm): observed EDITION codes.
- [V$INSTANCE 18](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-INSTANCE.html) and [V$VERSION 18](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/V-VERSION.html): full-version/banner columns introduced in18.
- [DBA_REGISTRY 11.2](https://docs.oracle.com/cd/E24693_01/server.11203/e24448/statviews_4208.htm) and [DBA_REGISTRY 18](https://docs.oracle.com/en/database/oracle/oracle-database/18/refrn/DBA_REGISTRY.html): raw component fields, MODIFIED type and full-version boundary.
- Current target references: [V$INSTANCE 19](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-INSTANCE.html), [V$INSTANCE 26](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-INSTANCE.html), [V$VERSION 26](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-VERSION.html), [DBA_REGISTRY 19](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_REGISTRY.html), [DBA_REGISTRY 26](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/DBA_REGISTRY.html).
