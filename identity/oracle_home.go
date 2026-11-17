package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// OracleHomeObservation identifies only the executing instance. Inferred marks
// a dictionary library-path candidate, not proof of the running home.
// LibraryFileSpec retains that complete source; other sources select NULL.
type OracleHomeObservation struct {
	Attempts        []OracleHomeAttempt
	Provenance      Provenance
	InstanceID      int64
	Home            Text
	LibraryFileSpec Text
	Inferred        bool
}

// OracleHomeRequest admits a known release and caller-owned execution reference.
// It neither grants dictionary access nor discovers/reconnects to other members.
type OracleHomeRequest struct {
	OracleMajor int
	Reference   reference.Reference
}

// OracleHomeStatement uses documented USERENV metadata on 12c+. On 9i-11g it
// selects only SYS.DBMS_SUMADV_LIB and derives an absolute Unix /lib/ parent.
// Symbolic, Windows and unrecognized library paths remain NULL candidates.
// This release selection is never an error-triggered fallback or a host read.
func OracleHomeStatement(major int) (string, error) {
	if major < 9 {
		return "", ErrUnsupportedVersion
	}
	const member = "SYS_CONTEXT('USERENV','INSTANCE') AS INSTANCE_ID"
	if major >= 12 {
		return "SELECT " + member + ", SYS_CONTEXT('USERENV','ORACLE_HOME') AS ORACLE_HOME, CAST(NULL AS VARCHAR2(4000)) AS LIBRARY_FILE_SPEC FROM DUAL", nil
	}
	return "SELECT " + member + ", CASE WHEN SUBSTR(FILE_SPEC,1,1) = '/' AND INSTR(FILE_SPEC,'/lib/',-1) > 1 AND LENGTH(FILE_SPEC) > INSTR(FILE_SPEC,'/lib/',-1)+4 AND INSTR(FILE_SPEC,'/',INSTR(FILE_SPEC,'/lib/',-1)+5) = 0 THEN SUBSTR(FILE_SPEC,1,INSTR(FILE_SPEC,'/lib/',-1)-1) END AS ORACLE_HOME, FILE_SPEC AS LIBRARY_FILE_SPEC FROM SYS.DBA_LIBRARIES WHERE OWNER = 'SYS' AND LIBRARY_NAME = 'DBMS_SUMADV_LIB'", nil
}

func oracleHomeSchema() []rowset.Column {
	return []rowset.Column{{Name: "INSTANCE_ID", Kind: rowset.Integer}, {Name: "ORACLE_HOME", Kind: rowset.Text}, {Name: "LIBRARY_FILE_SPEC", Kind: rowset.Text}}
}
func oracleHomeProvenance(major int) Provenance {
	p := Provenance{Source: "SYS_CONTEXT(USERENV.ORACLE_HOME)", Scope: "executing_instance", OracleMajor: major}
	if major < 12 {
		p.Source = "DBA_LIBRARIES(SYS.DBMS_SUMADV_LIB)"
		p.Scope = "executing_instance_dictionary_library_candidate"
	}
	return p
}

// decodeOracleHomeSQL validates the selected SQL dataset.
// NULL, an empty value and no rows remain distinct. A decoder alone cannot prove
// transport completion, library existence, current binaries or RAC-wide coverage.
func decodeOracleHomeSQL(result session.Result, major int) (OracleHomeObservation, error) {
	if _, err := OracleHomeStatement(major); err != nil {
		return OracleHomeObservation{}, err
	}
	index, err := factColumns(result, oracleHomeSchema())
	if err != nil || len(result.Rows) > 1 {
		return OracleHomeObservation{}, ErrInvalidEvidence
	}
	fact := OracleHomeObservation{Provenance: oracleHomeProvenance(major), Inferred: major < 12}
	if len(result.Rows) == 0 {
		fact.Home = Text{State: NoRows}
		fact.LibraryFileSpec = Text{State: NoRows}
		return fact, nil
	}
	row := result.Rows[0]
	id := row[index["INSTANCE_ID"]]
	fact.InstanceID, err = strconv.ParseInt(id.Text, 10, 64)
	if err != nil || id.Null || fact.InstanceID <= 0 {
		return OracleHomeObservation{}, ErrInvalidEvidence
	}
	fact.Home = text(row[index["ORACLE_HOME"]])
	fact.LibraryFileSpec = text(row[index["LIBRARY_FILE_SPEC"]])
	for _, v := range []Text{fact.Home, fact.LibraryFileSpec} {
		if len(v.Value) > 4000 || strings.ContainsRune(v.Value, 0) {
			return OracleHomeObservation{}, ErrInvalidEvidence
		}
	}
	if major >= 12 {
		if fact.LibraryFileSpec.State != Null {
			return OracleHomeObservation{}, ErrInvalidEvidence
		}
	} else {
		candidate := Text{State: Null}
		raw := fact.LibraryFileSpec.Value
		if i := strings.LastIndex(raw, "/lib/"); fact.LibraryFileSpec.State == Available && strings.HasPrefix(raw, "/") && i > 0 && len(raw) > i+5 && !strings.Contains(raw[i+5:], "/") {
			candidate = Text{State: Available, Value: raw[:i]}
		}
		if candidate != fact.Home {
			return OracleHomeObservation{}, ErrInvalidEvidence
		}
	}
	return fact, nil
}

// OracleHomeRows returns the actual caller-native cursor. Callers own
// Next/Scan/Err/Close and finite budgets; this reads no remote RAC members.
func OracleHomeRows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	statement, err := OracleHomeStatement(major)
	if err != nil {
		return nil, err
	}
	return rowset.Query(ctx, q, statement)
}

// OracleHomeRowsJSON independently consumes the same selected source into bounded
// JSON, including provenance/inference. No prefix survives a domain, transport,
// close, cancellation or budget failure. Generic cursor mechanics stay in rowset.
func OracleHomeRowsJSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	if _, err := OracleHomeStatement(major); err != nil {
		return nil, err
	}
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	envelope := struct {
		Provenance Provenance      `json:"provenance"`
		Inferred   bool            `json:"inferred"`
		Data       json.RawMessage `json:"data"`
	}{Provenance: oracleHomeProvenance(major), Inferred: major < 12}
	metadata, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	budget := resolved
	budget.MaxBytes -= len(metadata) - 4
	if budget.MaxBytes < 1 {
		return nil, rowset.ErrCapacity
	}
	if budget.MaxRows > 1 {
		budget.MaxRows = 1
	}
	cursor, err := OracleHomeRows(ctx, q, major)
	if err != nil {
		return nil, err
	}
	table, err := rowset.Read(ctx, cursor, oracleHomeSchema(), budget)
	if err != nil {
		return nil, err
	}
	result, ok := projection.FromRowset(table)
	if !ok {
		return nil, ErrInvalidEvidence
	}
	if _, err = DecodeOracleHome(result, major); err != nil {
		return nil, err
	}
	envelope.Data, err = json.Marshal(table)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(envelope)
	if len(payload) > resolved.MaxBytes {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	if err = factReadError(ctx, err); err != nil {
		return nil, err
	}
	return payload, nil
}
