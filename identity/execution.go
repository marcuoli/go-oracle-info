package identity

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/marcuoli/go-oracle-session/session"
)

// Execution preserves independent connected database, instance and session facts.
// Startup is source-local civil time without an inferred timezone. These facts
// do not admit an expected target or establish a cluster-wide identity.
type Execution struct {
	Database  Text
	DBID      Text
	Version   Text
	Instance  Text
	Container Text
	Service   Text
	Startup   Text

	// Parsed IDs are usable only when decoding succeeds and the corresponding
	// raw Text is Available. Null and NoRows leave these fields zero; zero is
	// also a valid available container ID, so it cannot represent absence.
	InstanceID  int
	ContainerID int
}

// ExecutionStatement preserves the seven-column connected identity projection.
// Callers must establish VERSION_FULL and view support before execution; this
// selector neither discovers capabilities nor changes the current container.
func ExecutionStatement() string {
	return `SELECT d.name AS database_name, TO_CHAR(d.dbid,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS dbid, i.version_full AS oracle_version, TO_CHAR(i.instance_number,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') AS instance_id, SYS_CONTEXT('USERENV','CON_ID') AS container_id, SYS_CONTEXT('USERENV','SERVICE_NAME') AS service_name, TO_CHAR(i.startup_time,'YYYY-MM-DD HH24:MI:SS') AS startup_key FROM v$database d CROSS JOIN v$instance i`
}

// DecodeExecution requires a complete seven-column scalar result. It preserves
// exact text, SQL NULL and no-row states without treating version text as a
// compatibility gate. Numeric-ID errors retain raw and independently valid facts
// alongside ErrInvalidEvidence; target requirements and comparisons stay outside.
func DecodeExecution(result session.Result) (Execution, error) {
	r, absent, err := row(result, "DATABASE_NAME", "DBID", "ORACLE_VERSION", "INSTANCE_ID", "CONTAINER_ID", "SERVICE_NAME", "STARTUP_KEY")
	if err != nil {
		return Execution{}, err
	}
	if absent {
		return Execution{
			Database: Text{State: NoRows}, DBID: Text{State: NoRows}, Version: Text{State: NoRows},
			Instance: Text{State: NoRows}, Container: Text{State: NoRows},
			Service: Text{State: NoRows}, Startup: Text{State: NoRows},
		}, nil
	}
	facts := Execution{
		Database: text(r["DATABASE_NAME"]), DBID: text(r["DBID"]), Version: text(r["ORACLE_VERSION"]),
		Instance: text(r["INSTANCE_ID"]), Container: text(r["CONTAINER_ID"]),
		Service: text(r["SERVICE_NAME"]), Startup: text(r["STARTUP_KEY"]),
	}
	var failures []error
	for _, id := range []struct {
		name    string
		raw     Text
		minimum int
		parsed  *int
	}{
		{"instance", facts.Instance, 1, &facts.InstanceID},
		{"container", facts.Container, 0, &facts.ContainerID},
	} {
		if id.raw.State != Available {
			continue
		}
		value, err := strconv.Atoi(id.raw.Value)
		if err != nil || value < id.minimum {
			failures = append(failures, fmt.Errorf("execution %s ID: %w", id.name, ErrInvalidEvidence))
			continue
		}
		*id.parsed = value
	}
	return facts, errors.Join(failures...)
}
