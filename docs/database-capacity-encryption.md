# Database capacity storage and encryption facts

These APIs read admitted Oracle metadata. They never connect, switch containers,
open files, change keys or infer migration readiness. Existing DBA storage APIs
and legacy database SQL constants remain unchanged.

## File allocation and storage

storage.DatabaseFilesStatement, DecodeDatabaseFiles, ReadDatabaseFiles,
DatabaseFilesRows and DatabaseFilesJSON use V$DATAFILE and V$TEMPFILE. Rows retain
file kind, identity, exact numeric bytes, literal path and actual CON_ID on 12+.
On 11g the container field is absent rather than a fabricated SQL NULL.
The connected Oracle view determines root/PDB/non-CDB visibility. Successful
transport does not prove that all physical containers/files are visible.

DatabaseFilesResult.DatafileBytes sums datafile allocation only, excluding temp,
redo and control files. It uses exact integers. Empty/incomplete sources, NULL
sizes and zero datafile BYTES remain unknown: Oracle documents zero as inaccessible.
CREATE_BYTES is historical allocation and is not a fallback for current size.
V$ controlfile metadata can be queried without an open dictionary when Oracle
and the caller's privileges permit; successful collection is still required.

DatabaseFilesResult.StorageType returns ASM, FileSystem or ASM+FileSystem from
all selected data/temp paths. Missing names, incomplete sources or unsupported Exascale @vault paths remain unknown.
It does not classify hardware. The separate catalog.VisibleCellCountSQL returns
global visible cell presence; a positive count identifies visible cell
infrastructure, zero only describes this admitted source and failure is unknown.
Neither source establishes physical RAC completeness or per-file backing media.

The root Client facade supplies DatabaseFiles and DatabaseFilesJSON after
observing the source release. Native rows are the actual database/sql cursor;
the caller owns iteration, Err, Close and finite budgets. JSON independently
owns a bounded cursor and returns no partial output on any failure.

## Independent encryption observations

database.EncryptionKind selects TablespaceEncryption or ColumnEncryption.
EncryptionStatement, DecodeEncryption, ReadEncryption, EncryptionRows and
EncryptionJSON expose one independent source. EncryptedCount retains the exact
count; UnknownCount retains unrecognized/null tablespace-encryption flags.

Non-CDB/direct-PDB sources are DBA_TABLESPACES and DBA_ENCRYPTED_COLUMNS.
An explicitly admitted 12+ CDB root selects CDB_TABLESPACES and CDB_ENCRYPTED_COLUMNS.
CDB_ visibility is restricted by open state, privileges and container visibility;
closed/restricted containers can be absent. Zero is a count of the selected
visible dictionary, not proof that the whole CDB contains no encrypted data.
These counts cover tablespace/column encryption, not network/backup/external-file
encryption or successful key access.

The root facade EncryptionCounts and EncryptionCountsJSON observe the connected
architecture and select the appropriate source automatically. Callers still admit
OPEN state and privileges. No V$ENCRYPTED_TABLESPACES fallback is attempted:
Oracle documents its information as meaningful only for open/online datafiles
and open containers, with UNKNOWN status possible while mounted. Source failure
must not erase an independently available peer observation.

JSON uses the established provenance/data envelope and numeric columns/rows.
All rows, cardinality and domain values pass validation before any JSON is
returned. The byte budget includes provenance, and native cursor finalization
belongs exclusively to session/rowset.

## Qualification and sources

Go 1.27.4 on Windows/amd64: full module tests and vet passed. Tests cover exact
integers beyond floating-point precision, excluded temp allocation, inaccessible/
NULL/empty sources, duplicate/malformed late rows, post-query cancellation, actual
native cursor ownership, CDB/PDB selection and JSON terminal/close/budget failures.
Native Oracle qualification is recorded separately; synthetic fixtures establish
API behavior rather than all-version/platform support.

Oracle sources checked 2026-11-15:

- [V$DATAFILE](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-DATAFILE.html): controlfile origin; BYTES zero when inaccessible; actual container identifiers.
- [V$TEMPFILE](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-TEMPFILE.html): separate temporary-file metadata.
- [DBA_TABLESPACES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_TABLESPACES.html): tablespace encryption definition.
- [DBA_ENCRYPTED_COLUMNS](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/DBA_ENCRYPTED_COLUMNS.html): encrypted-column inventory.
- [Monitoring CDBs and PDBs](https://docs.oracle.com/en/database/oracle/oracle-database/19/multi/viewing-information-about-cdbs-and-pdbs-with-sql-plus.html): container visibility and restricted/closed scope limitations.
- [V$ENCRYPTED_TABLESPACES](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-ENCRYPTED_TABLESPACES.html): OPEN/online validity and mounted UNKNOWN status.

Source remains queued under issue #49. Final review, configured release checks,
privacy/metadata audit, integration, canonical synchronization and exact source
cleanup precede end-of-queue build/publication.


Oracle Exascale path review (2026-11-15): [Vaults](https://docs.oracle.com/en/engineered-systems/exadata-database-machine/exscl/vaults.html) and [Database file storage attributes](https://docs.oracle.com/en/engineered-systems/exadata-database-machine/exscl/customizing-storage-attributes-database-files.html). Exascale uses @vault paths; these remain unclassified under the three-value property rather than becoming FileSystem. Raw file facts and allocation remain available. No physical Exascale qualification is claimed.

Native consumer qualification on 2026-11-15 used an existing 26ai Enterprise
(23.26.1.0.0) primary: OPEN CDB-root allocation/encryption and direct-PDB
allocation/encryption matched independent SELECT aggregates. The existing
MOUNTED standby's allocation matched V$DATAFILE; encryption was deliberately
not admitted as an OPEN dictionary observation. This does not qualify every
release/platform or physical RAC/ASM/Exadata/Exascale configuration.

Independent review found and resolved two storage cases before queue delivery:
Exascale paths stay unknown, and numeric-equivalent file/container IDs are
deduplicated by canonical comparison keys without changing raw returned fields.
Focused red/green regressions and full module tests/vet passed after correction.
