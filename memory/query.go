// Package memory selects and decodes read-only configured-memory facts.
// It opens no sessions and leaves endpoint/container admission to callers.
package memory

import "errors"

// Parameter selects a fixed configured-memory family, never arbitrary SQL.
type Parameter string

const (
	SGA Parameter = "sga"
	PGA Parameter = "pga"
)

// ErrUnsupportedParameter rejects names outside the two configured families.
var ErrUnsupportedParameter = errors.New("unsupported configured-memory parameter")

// PDBOptions declares already-admitted inheritance and effective-value selection.
// EffectiveADB requires known Oracle 18+ and a primary NULL/no-row outcome.
// Use Collect to enforce that transition; PDBStatement alone executes nothing.
type PDBOptions struct {
	AllowInherited bool
	EffectiveADB   bool
}

// PDBStatement selects current-instance, connected-PDB configured settings.
// It returns exact three-decimal GiB text and source/current-container identity.
// An inherited setting is not a PDB allocation or a RAC total.
func PDBStatement(parameter Parameter, options PDBOptions) (string, error) {
	return pdbStatement(parameter, options, "TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))", "GV$SYSTEM_PARAMETER")
}

func pdbStatement(parameter Parameter, options PDBOptions, target, source string) (string, error) {
	child, parent := "pga_aggregate_target", "pga_aggregate_target"
	childPriority := "P.SOURCE_CONTAINER_ID=" + target
	selection := ""
	switch parameter {
	case SGA:
		child, parent = "sga_target", "sga_max_size"
		// Duplicate zero rows must win child priority and reach the decoder. Dropping
		// them before selection would silently promote a valid-looking parent value.
		childPriority += " AND (P.SOURCE_ROWS<>1 OR P.VALUE IS NULL OR TO_NUMBER(P.VALUE)<>0)"
		selection = " AND (SOURCE_ROWS<>1 OR SOURCE_CONTAINER_ID<>" + target + " OR VALUE IS NULL OR TO_NUMBER(VALUE)<>0)"
	case PGA:
	default:
		return "", ErrUnsupportedParameter
	}
	projection := "SELECT " + numeric("ROUND(TO_NUMBER(VALUE)/POWER(1024,3),3)") + " AS VALUE, " +
		numeric("SOURCE_CONTAINER_ID") + " AS SOURCE_CONTAINER_ID, " + numeric("TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID'))") + " AS CURRENT_CONTAINER_ID, " +
		"PARAMETER_NAME AS PARAMETER_NAME, " + numeric("SOURCE_ROWS") + " AS SOURCE_ROWS, " +
		numeric("TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE'))") + " AS INSTANCE_ID, " + numeric(target) + " AS TARGET_CONTAINER_ID FROM ("
	inherited := ""
	if options.AllowInherited {
		inherited = " OR (S.CON_ID IN (0,1) AND S.NAME='" + parent + "')"
	}
	query := projection + "SELECT P.VALUE AS VALUE, P.PARAMETER_NAME AS PARAMETER_NAME, P.SOURCE_CONTAINER_ID AS SOURCE_CONTAINER_ID, P.SOURCE_ROWS AS SOURCE_ROWS,\n" +
		"DENSE_RANK() OVER (ORDER BY CASE WHEN " + childPriority + " THEN 0 WHEN P.SOURCE_CONTAINER_ID=0 THEN 1 WHEN P.SOURCE_CONTAINER_ID=1 THEN 2 ELSE 3 END) AS SOURCE_PRIORITY\n" +
		"FROM (SELECT S.VALUE AS VALUE, S.NAME AS PARAMETER_NAME, S.CON_ID AS SOURCE_CONTAINER_ID,\n" +
		"COUNT(*) OVER (PARTITION BY S.CON_ID,S.NAME) AS SOURCE_ROWS FROM " + source + " S\n" +
		"WHERE S.INST_ID=TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AND ((S.CON_ID=" + target + " AND S.NAME='" + child + "')" + inherited + ")) P)\n" +
		"WHERE SOURCE_PRIORITY=1" + selection
	if options.EffectiveADB {
		query = projection + "SELECT P.VALUE AS VALUE, P.NAME AS PARAMETER_NAME, P.CON_ID AS SOURCE_CONTAINER_ID, COUNT(*) OVER () AS SOURCE_ROWS\n" +
			"FROM V$PARAMETER P WHERE P.CON_ID=" + target + " AND P.NAME='" + child + "'\n" +
			"AND P.ISSES_MODIFIABLE='FALSE' AND SYS_CONTEXT('USERENV','CLOUD_SERVICE') IN ('DWCS','OLTP','JDCS'))"
		if parameter == SGA {
			query += " WHERE SOURCE_ROWS<>1 OR VALUE IS NULL OR TO_NUMBER(VALUE)>0"
		}
	}
	return query, nil
}

func numeric(expression string) string {
	return "TO_CHAR(" + expression + ", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''')"
}
