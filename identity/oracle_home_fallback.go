package identity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"

	"github.com/marcuoli/go-oracle-session/session"
)

// OracleHomeSource identifies the actual selected source, independently of release.
type OracleHomeSource string

const (
	OracleHomeContext     OracleHomeSource = "SYS_CONTEXT(USERENV.ORACLE_HOME)"
	OracleHomeEnvironment OracleHomeSource = "DBMS_SYSTEM.GET_ENV(ORACLE_HOME)"
	OracleHomeLibrary     OracleHomeSource = "DBA_LIBRARIES(SYS.DBMS_SUMADV_LIB)"
)

// OracleHomeAttempt retains safe state/code facts for every source considered.
// Err preserves a cause for private inspection and is never serialized.
type OracleHomeAttempt struct {
	Source OracleHomeSource
	State  State
	Code   string
	Err    error `json:"-"`
}

// OracleHomeExecutor is the existing non-owning session execution contract.
// GET_ENV uses OUT binds; no DBMS_OUTPUT state or text resultset is introduced.
type OracleHomeExecutor interface {
	Exec(context.Context, session.Statement, session.ExecOptions) (sql.Result, error)
}

// OracleHomeEnvironmentStatement reads the process environment and executing
// instance atomically. Only the fixed ORACLE_HOME key is permitted by this API.
func OracleHomeEnvironmentStatement() string {
	return "BEGIN SYS.DBMS_SYSTEM.GET_ENV('ORACLE_HOME', :oracle_home); :instance_id := SYS_CONTEXT('USERENV','INSTANCE'); END;"
}

// DecodeOracleHome decodes the release-selected raw SQL dataset, not a fallback.
func DecodeOracleHome(result session.Result, major int) (OracleHomeObservation, error) {
	source := OracleHomeContext
	if major < 12 {
		source = OracleHomeLibrary
	}
	return DecodeOracleHomeSource(result, major, source)
}

// DecodeOracleHomeSource validates raw source/candidate coherence for a known
// selected source. Callers may reuse it when admitting saved observations.
func DecodeOracleHomeSource(result session.Result, major int, source OracleHomeSource) (OracleHomeObservation, error) {
	if major < 9 {
		return OracleHomeObservation{}, ErrUnsupportedVersion
	}
	selected := 12
	switch source {
	case OracleHomeContext:
		if major < 12 {
			return OracleHomeObservation{}, ErrUnsupportedVersion
		}
	case OracleHomeEnvironment:
	case OracleHomeLibrary:
		selected = 11
	default:
		return OracleHomeObservation{}, ErrInvalidEvidence
	}
	fact, err := decodeOracleHomeSQL(result, selected)
	if err != nil {
		return OracleHomeObservation{}, err
	}
	fact.Provenance.OracleMajor = major
	fact.Provenance.Source = string(source)
	return fact, nil
}

// ErrOracleHomeExecutorUnavailable reports a caller without admitted OUT-bind execution.
var ErrOracleHomeExecutorUnavailable = errors.New("Oracle home environment executor unavailable")

var oracleHomeCode = regexp.MustCompile("ORA-[0-9]{5}")

func homeErrorCode(err error) string {
	if errors.Is(err, ErrOracleHomeExecutorUnavailable) {
		return "executor_unavailable"
	}
	// Native rowset wrappers keep public messages static. Inspect bounded causes,
	// retaining only the Oracle code; never expose driver text or OUT values.
	pending := []error{err}
	for visited := 0; len(pending) > 0 && visited < 64; visited++ {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current == nil {
			continue
		}
		if owned, ok := current.(*session.Error); ok {
			if code := oracleHomeCode.FindString(owned.Code); code != "" {
				return code
			}
		}
		if code := oracleHomeCode.FindString(current.Error()); code != "" {
			return code
		}
		switch cause := current.(type) {
		case interface{ Unwrap() []error }:
			pending = append(pending, cause.Unwrap()...)
		case interface{ Unwrap() error }:
			pending = append(pending, cause.Unwrap())
		}
	}
	return ""
}
func homeFallbackAllowed(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, session.ErrUnavailable) || errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		return false
	}
	pending := []error{err}
	leaves := 0
	for visited := 0; len(pending) > 0 && visited < 64; visited++ {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current == nil {
			continue
		}
		switch cause := current.(type) {
		case interface{ Unwrap() []error }:
			children := cause.Unwrap()
			if len(children) > 0 {
				pending = append(pending, children...)
				continue
			}
		case interface{ Unwrap() error }:
			if child := cause.Unwrap(); child != nil {
				pending = append(pending, child)
				continue
			}
		}
		leaves++
		// Every terminal cause must describe an unavailable source. A recognized
		// Oracle code must not hide a simultaneous close, capacity or decode error.
		if current == ErrOracleHomeExecutorUnavailable {
			continue
		}
		switch homeErrorCode(current) {
		case "ORA-02003", "ORA-00942", "ORA-01031", "ORA-06550", "ORA-04043":
		default:
			return false
		}
	}
	return len(pending) == 0 && leaves > 0
}

// ReadOracleHome prefers documented context on 12c+, then GET_ENV when the
// admitted reader supports Exec, then the inferred dictionary library candidate.
// Empty/NULL/no-row or specifically unavailable sources may advance the chain;
// cancellation, invalid evidence, unknown transport failures and retired sessions
// stop it. Attempts retain failures even when a later source supplies a value.
func ReadOracleHome(ctx context.Context, reader Reader, request OracleHomeRequest) (OracleHomeObservation, error) {
	if err := validateFactRequest(reader, request.Reference); err != nil {
		return OracleHomeObservation{}, err
	}
	if request.OracleMajor < 9 {
		return OracleHomeObservation{}, ErrUnsupportedVersion
	}
	sources := []OracleHomeSource{OracleHomeEnvironment, OracleHomeLibrary}
	if request.OracleMajor >= 12 {
		sources = append([]OracleHomeSource{OracleHomeContext}, sources...)
	}
	var attempts []OracleHomeAttempt
	for _, source := range sources {
		if err := factReadError(ctx, nil); err != nil {
			return OracleHomeObservation{Attempts: attempts}, err
		}
		var result session.Result
		var err error
		if source == OracleHomeEnvironment {
			executor, ok := reader.(OracleHomeExecutor)
			if !ok {
				attempts = append(attempts, OracleHomeAttempt{Source: source, State: Unavailable, Code: "executor_unavailable"})
				continue
			}
			// sql.Out has no explicit string-capacity field. The qualified driver
			// infers it from destination storage even for OUT-only binds. NUL-filled
			// storage is never evidence: failed/unchanged outputs cannot pass decoding.
			home, instance := strings.Repeat("\x00", 4000), strings.Repeat("\x00", 32)
			_, err = executor.Exec(ctx, session.Statement{Reference: request.Reference, Text: OracleHomeEnvironmentStatement(), Args: []any{sql.Out{Dest: &home}, sql.Out{Dest: &instance}}}, session.ExecOptions{})
			if err == nil {
				result = session.Result{Columns: []string{"INSTANCE_ID", "ORACLE_HOME", "LIBRARY_FILE_SPEC"}, Rows: [][]session.Value{{{Text: instance}, {Text: home, Null: home == ""}, {Null: true}}}}
			}
		} else {
			major := request.OracleMajor
			if source == OracleHomeLibrary {
				major = 11
			}
			statement, selectErr := OracleHomeStatement(major)
			if selectErr != nil {
				return OracleHomeObservation{Attempts: attempts}, selectErr
			}
			result, err = reader.Query(ctx, session.Statement{Reference: request.Reference, Text: statement})
		}
		if joined := factReadError(ctx, err); joined != nil {
			code := homeErrorCode(joined)
			attempts = append(attempts, OracleHomeAttempt{Source: source, State: Unavailable, Code: code, Err: joined})
			if !homeFallbackAllowed(ctx, joined) {
				return OracleHomeObservation{Attempts: attempts}, joined
			}
			continue
		}
		fact, decodeErr := DecodeOracleHomeSource(result, request.OracleMajor, source)
		if decodeErr = factReadError(ctx, decodeErr); decodeErr != nil {
			attempts = append(attempts, OracleHomeAttempt{Source: source, State: Unavailable, Code: "invalid_evidence", Err: decodeErr})
			return OracleHomeObservation{Attempts: attempts}, decodeErr
		}
		attempts = append(attempts, OracleHomeAttempt{Source: source, State: fact.Home.State})
		fact.Attempts = attempts
		if fact.Home.State == Available && fact.Home.Value != "" {
			return fact, nil
		}
		if source == OracleHomeLibrary {
			return fact, nil
		}
	}
	var failures []error
	for _, attempt := range attempts {
		if attempt.Err != nil {
			failures = append(failures, attempt.Err)
		}
	}
	if len(failures) == 0 {
		failures = append(failures, errors.New("Oracle home sources unavailable"))
	}
	return OracleHomeObservation{Attempts: attempts}, &ReadError{cause: errors.Join(failures...)}
}
