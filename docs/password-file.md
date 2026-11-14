# Password-file metadata

`passwordfile` reads `V$PASSWORDFILE_INFO` metadata from an already-admitted
connected instance. The common projection is `FILE_NAME`, `FORMAT`, `IS_ASM`,
`CON_ID`. `DefaultOptions(version)` admits this view from Oracle 12.2 and selects
`IS_EXC` from the documented Oracle 21c floor. Explicit `Options` remain available
for narrower or independently admitted capabilities. No probe, retry or fallback
is performed. All aliases are explicit; numeric `CON_ID` remains an integer in
native JSON, and format/flag/path text remains unchanged.

`Statement`, pure `Decode` and `Read` serve existing owned buffered readers with
caller-owned execution references. `Rows` and `JSON` serve caller-owned native
handles through the shared session rowset package. The root facade provides
`PasswordFileInfo` and `PasswordFileInfoJSON` with explicit `PasswordFileOptions`.
It reads version through existing identity properties, without a cache or new
connection. JSON has the same selected columns as native rows and adds domain
source/version/scope/optional-column provenance. Its finite byte limit includes
all escaping/schema/provenance; any failure returns nil JSON. More than one row
is not accepted as singular complete metadata.

A row containing SQL NULL differs from a successful empty source, an unavailable
read or unsupported version. Typed `Info` uses exact `session.Value` fields;
`IsExascale` nil means not selected, while a selected NULL has a nonnil `.Null`
value. An available row does not prove a file exists. Do not infer OS storage
from `IS_ASM=FALSE` alone on Exascale-capable sources, normalize unknown formats,
or infer credential policy, licensing, file contents, security or compatibility.

Oracle documents the view as singular and queryable from root/PDB, with CDB
`CON_ID=0` for common database-file metadata. This does not prove a distinct PDB
file or a complete RAC inventory. Source permissions and open/mounted/driver/
platform acceptance belong to the caller. A source error is unavailable evidence,
not an empty file. The view can retain cached metadata after a file name/location
change; these APIs never flush metadata, alter settings or read users, passwords,
verifiers or actual file bytes. Private paths and driver causes need deliberate
handling by the consumer.

Sources checked 2026-11-14:

- [Oracle 12.2 Database Reference](https://docs.oracle.com/en/database/oracle/oracle-database/12.2/refrn/V-PASSWORDFILE_INFO.html): introduced at 12.2.0.1, base projection and shared CDB row.
- [Oracle 19 Database Reference](https://docs.oracle.com/en/database/oracle/oracle-database/19/refrn/V-PASSWORDFILE_INFO.html): common projection and metadata-cache observation.
- [Oracle 21 Database Reference](https://docs.oracle.com/en/database/oracle/oracle-database/21/refrn/V-PASSWORDFILE_INFO.html): explicit footnote dates `IS_EXC` to Oracle 21c.
- [Oracle 26 Database Reference](https://docs.oracle.com/en/database/oracle/oracle-database/26/refrn/V-PASSWORDFILE_INFO.html): current Exascale/ASM distinctions and shared metadata.

See the [application developer guide](developer-guide.md) for reader choice,
source admission, errors and requests for additional punctual fact methods.