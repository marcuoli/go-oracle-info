// Package database selects read-only connected database/environment facts.
// Callers admit dictionary privileges and root/PDB/instance scope; no scope changes occur here.
package database

// ConfigurationSQL reads the cluster configuration parameter, not proven RAC membership.
const ConfigurationSQL = `SELECT CASE WHEN VALUE='TRUE' THEN 'RAC' ELSE 'Single Instance' END AS VALUE FROM V$PARAMETER WHERE NAME='cluster_database'`

// EndianSQL reads the database platform's byte order.
const EndianSQL = `SELECT ENDIAN_FORMAT AS VALUE FROM V$TRANSPORTABLE_PLATFORM WHERE PLATFORM_ID=(SELECT PLATFORM_ID FROM V$DATABASE)`

// CharacterSetSQL reads the database character set.
const CharacterSetSQL = `SELECT VALUE FROM NLS_DATABASE_PARAMETERS WHERE PARAMETER='NLS_CHARACTERSET'`

// CPUCountSQL reads the configured CPU parameter, not host capacity.
const CPUCountSQL = `SELECT VALUE FROM V$PARAMETER WHERE NAME='cpu_count'`

// ProductsSQL lists selected registered components; registration does not prove licensing.
const ProductsSQL = `SELECT COMP_ID AS VALUE FROM DBA_REGISTRY WHERE COMP_ID IN ('CONTEXT','SDO','XOQ','DV','OLS','APEX') ORDER BY COMP_ID`

// DatafileSizeSQL sums visible datafile bytes in rounded GiB; an empty source remains NULL.
const DatafileSizeSQL = `SELECT CASE WHEN COUNT(*)>0 THEN TO_CHAR(ROUND(SUM(BYTES)/POWER(1024,3),3), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') END AS VALUE FROM DBA_DATA_FILES`

// StorageSQL classifies visible data/temp file names; missing file names remain unknown.
const StorageSQL = `SELECT CASE WHEN COUNT(*)=0 OR COUNT(FILE_NAME)<>COUNT(*) THEN NULL WHEN SUM(CASE WHEN FILE_NAME LIKE '+%' THEN 1 ELSE 0 END)=COUNT(*) THEN 'ASM' WHEN SUM(CASE WHEN FILE_NAME LIKE '+%' THEN 1 ELSE 0 END)=0 THEN 'FileSystem' ELSE 'Mixed (ASM+FileSystem)' END AS VALUE FROM (SELECT FILE_NAME FROM DBA_DATA_FILES UNION ALL SELECT FILE_NAME FROM DBA_TEMP_FILES)`

// EncryptionSQL counts encrypted tablespaces and columns independently.
// Separate scalar aggregates avoid ORA-00937, including on empty sources.
const EncryptionSQL = `SELECT (SELECT TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') FROM DBA_TABLESPACES WHERE ENCRYPTED='YES') AS ENCRYPTED_TABLESPACES, (SELECT TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') FROM DBA_ENCRYPTED_COLUMNS) AS ENCRYPTED_COLUMNS FROM DUAL`

// SGAAllocatedSQL reads observed instance SGA allocation in rounded GiB, not configured memory.
const SGAAllocatedSQL = `SELECT TO_CHAR(ROUND(SUM(VALUE)/POWER(1024,3),3), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS VALUE FROM V$SGA`

// PGAAllocatedSQL reads observed instance PGA allocation in rounded GiB, not its target.
const PGAAllocatedSQL = `SELECT TO_CHAR(ROUND(VALUE/POWER(1024,3),3), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS VALUE FROM V$PGASTAT WHERE NAME='total PGA allocated'`

// PDBCountSQL counts visible non-seed PDBs in root scope.
const PDBCountSQL = `SELECT TO_CHAR(COUNT(*), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS VALUE FROM V$PDBS WHERE NAME<>'PDB$SEED'`

// ParametersSQL reads display/raw initialization values without choosing comparison policy.
const ParametersSQL = `SELECT NAME, COALESCE(DISPLAY_VALUE,VALUE) AS VALUE FROM V$PARAMETER ORDER BY NAME`

// NLSDatabaseSQL reads all database NLS values without normalizing their text.
const NLSDatabaseSQL = `SELECT PARAMETER AS NAME, VALUE FROM NLS_DATABASE_PARAMETERS ORDER BY PARAMETER`

// NLSInstanceSQL reads all instance NLS values without normalizing their text.
const NLSInstanceSQL = `SELECT PARAMETER AS NAME, VALUE FROM NLS_INSTANCE_PARAMETERS ORDER BY PARAMETER`
