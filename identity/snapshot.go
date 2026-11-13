// Package identity selects and decodes exact Oracle identity facts.
package identity

// SnapshotSQL reads connected database/instance identity using Oracle 11g+ columns.
// It does not admit an expected destination or prove a PDB's open mode.
const SnapshotSQL = `SELECT d.DB_UNIQUE_NAME, d.NAME AS DATABASE_NAME, i.VERSION, i.HOST_NAME, d.PLATFORM_NAME, d.OPEN_MODE, d.DATABASE_ROLE, d.LOG_MODE, i.STATUS AS INSTANCE_STATUS FROM V$DATABASE d CROSS JOIN V$INSTANCE i`

// ContainerSQL reads current container and CDB status on Oracle 12c+.
// Callers must establish the release before selecting this projection.
const ContainerSQL = `SELECT SYS_CONTEXT('USERENV','CON_NAME') AS CONTAINER, CDB FROM V$DATABASE`

// FullVersionSQL reads the unaliased full version on a known Oracle 18+ instance.
// VersionStatement remains available for the existing normalized projection.
const FullVersionSQL = `SELECT VERSION_FULL FROM V$INSTANCE`

// PDBOpenModeSQL reads the connected PDB's open mode on Oracle 12c+.
// Empty or inaccessible results do not inherit the root's open mode.
const PDBOpenModeSQL = `SELECT OPEN_MODE FROM V$PDBS WHERE CON_ID=TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))`
