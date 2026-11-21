package parameters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ContainerSystemSource selects one admitted scalar initialization parameter view.
type ContainerSystemSource string

const (
	// ContainerSystemLocal selects V$SYSTEM_PARAMETER on the executing instance.
	ContainerSystemLocal ContainerSystemSource = "V$SYSTEM_PARAMETER"
	// ContainerSystemGlobal selects GV$SYSTEM_PARAMETER across qualified visible instances.
	ContainerSystemGlobal ContainerSystemSource = "GV$SYSTEM_PARAMETER"
)

var (
	// ErrContainerSystemUnsupportedScope reports a known non-CDB connection.
	ErrContainerSystemUnsupportedScope = errors.New("scoped system parameters require a CDB connection")
	// ErrContainerSystemInaccessible marks an explicit ORA-01031 source failure.
	ErrContainerSystemInaccessible = errors.New("scoped system parameter source is inaccessible")
)

// ContainerSystemOptions supplies the observed numeric release and the executing
// container ID from the caller's admitted handle. In CDB$ROOT (ID 1), an empty
// ContainerIDs filter admits every visible row. Explicit IDs can include 0 for
// whole-CDB data. Outside CDB$ROOT, only the known current container is admitted.
// ID 0 as CurrentContainerID describes a non-CDB.
type ContainerSystemOptions struct {
	OracleVersion      string
	CurrentContainerID int64
	Source             ContainerSystemSource
	Filter             Filter
	ContainerIDs       []int64
}

// ContainerSystemRequest supplies the options and a stable caller SQL reference.
type ContainerSystemRequest struct {
	Options   ContainerSystemOptions
	Reference reference.Reference
}

// ContainerSystemParameter preserves the scalar parameter text and SQL NULLs.
// InstanceID is the executing instance for V$ and the row's reporting instance
// for GV$. ContainerID is the source row's CON_ID, independent of the connection.
type ContainerSystemParameter struct {
	InstanceID   session.Value
	ContainerID  session.Value
	Name         session.Value
	Value        session.Value
	DisplayValue session.Value
}

// ContainerSystemProvenance describes the selected source and filters. Visibility
// and transport completion do not establish coverage of all containers or RAC members.
type ContainerSystemProvenance struct {
	Provenance
	OracleVersion      string  `json:"oracle_version"`
	CurrentContainerID int64   `json:"current_container_id"`
	ContainerIDs       []int64 `json:"container_ids,omitempty"`
}

// ContainerSystemResult retains source order and duplicates. A nonnil empty Rows
// slice means the admitted selection returned no rows.
type ContainerSystemResult struct {
	Provenance ContainerSystemProvenance
	Rows       []ContainerSystemParameter
}

type containerSystemSelection struct {
	selection
	evidence ContainerSystemProvenance
}

// ContainerSystemStatement selects one documented scoped scalar projection with
// exact filters. It does not query session parameters, merge sources or infer limits.
func ContainerSystemStatement(options ContainerSystemOptions) (string, []any, error) {
	p, err := containerSystemPlan(options)
	return p.sql, p.args, err
}

func containerSystemPlan(options ContainerSystemOptions) (containerSystemSelection, error) {
	major := containerSystemMajor(options.OracleVersion)
	if major == 0 {
		return containerSystemSelection{}, ErrUnsupportedVersion
	}
	if options.CurrentContainerID == 0 {
		return containerSystemSelection{}, ErrContainerSystemUnsupportedScope
	}
	instances := options.Filter.Instances
	names := options.Filter.Names
	ids := options.ContainerIDs
	if options.CurrentContainerID < 0 || len(ids) > MaxFilterValues {
		return containerSystemSelection{}, ErrInvalidFilter
	}
	if err := validate(names, instances, nil); err != nil {
		return containerSystemSelection{}, err
	}

	var instance string
	switch options.Source {
	case ContainerSystemLocal:
		// V$ does not project INST_ID. SYS_CONTEXT identifies its executing
		// member without a join that might change container visibility.
		instance = "TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE'))"
	case ContainerSystemGlobal:
		instance = "p.INST_ID"
	default:
		return containerSystemSelection{}, ErrInvalidFilter
	}
	scope := "root_visible_container_rows"
	if options.CurrentContainerID != 1 {
		scope = "current_container_rows"
		if len(ids) == 0 {
			ids = []int64{options.CurrentContainerID}
		}
	}
	for _, id := range ids {
		if id < 0 || (options.CurrentContainerID != 1 && id != options.CurrentContainerID) {
			return containerSystemSelection{}, ErrInvalidFilter
		}
	}

	provenance := source(string(options.Source), major, names, instances)
	provenance.Scope = scope
	schema := []rowset.Column{
		{Name: "INST_ID", Kind: rowset.Integer}, {Name: "CON_ID", Kind: rowset.Integer},
		{Name: "NAME", Kind: rowset.Text}, {Name: "VALUE", Kind: rowset.Text}, {Name: "DISPLAY_VALUE", Kind: rowset.Text},
	}
	var args []any
	var clauses []string
	for _, filter := range []struct {
		expression string
		ids        []int64
	}{{"p.CON_ID", ids}, {instance, instances}} {
		if len(filter.ids) == 0 {
			continue
		}
		binds := make([]string, 0, len(filter.ids))
		for _, id := range filter.ids {
			args = append(args, id)
			binds = append(binds, ":"+strconv.Itoa(len(args)))
		}
		clauses = append(clauses, filter.expression+" IN ("+strings.Join(binds, ",")+")")
	}
	if len(names) > 0 {
		binds := make([]string, 0, len(names))
		for _, name := range names {
			args = append(args, name)
			binds = append(binds, "NLSSORT(:"+strconv.Itoa(len(args))+", 'NLS_SORT=BINARY')")
		}
		clauses = append(clauses, "NLSSORT(p.NAME, 'NLS_SORT=BINARY') IN ("+strings.Join(binds, ",")+")")
	}

	text := "SELECT " + instance + " AS INST_ID, p.CON_ID AS CON_ID, p.NAME AS NAME, p.VALUE AS VALUE, p.DISPLAY_VALUE AS DISPLAY_VALUE FROM " + provenance.Source + " p"
	if len(clauses) > 0 {
		text += " WHERE " + strings.Join(clauses, " AND ")
	}
	text += " ORDER BY INST_ID, p.CON_ID, NLSSORT(p.NAME, 'NLS_SORT=BINARY')"
	return containerSystemSelection{
		selection: selection{sql: text, args: args, schema: schema, provenance: provenance},
		evidence: ContainerSystemProvenance{
			Provenance: provenance, OracleVersion: options.OracleVersion,
			CurrentContainerID: options.CurrentContainerID, ContainerIDs: slices.Clone(ids),
		},
	}, nil
}

func containerSystemMajor(version string) int {
	if len(version) > 128 {
		return 0
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 5 {
		return 0
	}
	var numbers [5]uint64
	for i, part := range parts {
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil || !numeric.IsUnsignedInteger(part) {
			return 0
		}
		numbers[i] = value
	}
	switch numbers[0] {
	case 12:
		if numbers[1] == 1 || numbers[1] == 2 {
			return 12
		}
	case 18, 19, 21, 23:
		// Oracle Database 26ai uses the numeric 23 release series.
		return int(numbers[0])
	}
	return 0
}

// DecodeContainerSystem validates the whole projection and scope. It preserves
// NULL, raw value and display text, row order and duplicate names.
func DecodeContainerSystem(result session.Result, options ContainerSystemOptions) (ContainerSystemResult, error) {
	p, err := containerSystemPlan(options)
	if err != nil {
		return ContainerSystemResult{}, err
	}
	return decodeContainerSystem(result, p)
}

func containerSystemColumns(result session.Result, p containerSystemSelection) (map[string]int, error) {
	index, err := parameterColumns(result, p.selection)
	if err != nil {
		return nil, err
	}
	local := p.provenance.Source == string(ContainerSystemLocal)
	fail := &Error{Source: p.provenance.Source, cause: ErrInvalidEvidence}
	var localInstance int64
	for _, row := range result.Rows {
		instance := row[index["INST_ID"]]
		container := row[index["CON_ID"]]
		name := row[index["NAME"]]
		i, _ := strconv.ParseInt(instance.Text, 10, 64)   // Validated by parameterColumns.
		id, _ := strconv.ParseInt(container.Text, 10, 64) // SQL NULL is handled below.
		if name.Null || name.Text == "" || (!instance.Null && i < 1) || (!container.Null && id < 0) {
			return nil, fail
		}
		if len(p.evidence.ContainerIDs) > 0 && (container.Null || !slices.Contains(p.evidence.ContainerIDs, id)) {
			return nil, fail
		}
		if local {
			if instance.Null || (localInstance != 0 && i != localInstance) {
				return nil, fail
			}
			localInstance = i
		}
	}
	return index, nil
}

func decodeContainerSystem(result session.Result, p containerSystemSelection) (ContainerSystemResult, error) {
	index, err := containerSystemColumns(result, p)
	if err != nil {
		return ContainerSystemResult{}, err
	}
	rows := make([]ContainerSystemParameter, len(result.Rows))
	for i, row := range result.Rows {
		rows[i] = ContainerSystemParameter{
			InstanceID: row[index["INST_ID"]], ContainerID: row[index["CON_ID"]], Name: row[index["NAME"]],
			Value: row[index["VALUE"]], DisplayValue: row[index["DISPLAY_VALUE"]],
		}
	}
	return ContainerSystemResult{Provenance: p.evidence, Rows: rows}, nil
}

// ReadContainerSystem executes once through the admitted complete bounded reader.
// Query, late decode and cancellation failures leave no successful row prefix.
func ReadContainerSystem(ctx context.Context, reader Reader, request ContainerSystemRequest) (ContainerSystemResult, error) {
	p, err := containerSystemPlan(request.Options)
	if err != nil {
		return ContainerSystemResult{}, err
	}
	result, err := queryParameters(ctx, reader, request.Reference, p.selection)
	if err != nil {
		return ContainerSystemResult{}, markContainerSystemAccess(err)
	}
	facts, decodeErr := decodeContainerSystem(result, p)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return ContainerSystemResult{}, &Error{Source: p.provenance.Source, cause: err}
	}
	return facts, nil
}

// ContainerSystemRows returns the actual caller-native cursor. Callers own
// Next, Scan, Err, Close, handle affinity and finite streaming budgets.
func ContainerSystemRows(ctx context.Context, q rowset.Queryer, options ContainerSystemOptions) (*sql.Rows, error) {
	p, err := containerSystemPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := open(ctx, q, p.selection)
	return rows, markContainerSystemAccess(err)
}

// ContainerSystemJSON independently consumes the same projection through the
// shared rowset helper. Failures return nil JSON, including final byte-budget checks.
func ContainerSystemJSON(ctx context.Context, q rowset.Queryer, options ContainerSystemOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := containerSystemPlan(options)
	if err != nil {
		return nil, err
	}
	invalid := &Error{Source: p.provenance.Source, cause: ErrInvalidEvidence}
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, p.sql, p.args...), p.schema, limits, rowset.EnvelopeOptions{
		Provenance: p.evidence,
		Validate: projection.ValidateRowset(invalid, func(result session.Result) error {
			_, err := containerSystemColumns(result, p)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		switch stage {
		case "limits", "validate":
			return cause
		case "query":
			return markContainerSystemAccess(&Error{Source: p.provenance.Source, cause: cause})
		default:
			return &Error{Source: p.provenance.Source, cause: cause}
		}
	})
}

func markContainerSystemAccess(err error) error {
	failure, ok := err.(*Error)
	if !ok || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		!isContainerSystemPrivilegeError(failure.cause) {
		return err
	}
	return &Error{Source: failure.Source, cause: errors.Join(ErrContainerSystemInaccessible, failure.cause)}
}

func isContainerSystemPrivilegeError(err error) bool {
	switch value := err.(type) {
	case nil:
		return false
	case *session.Error:
		return value.Code == "ORA-01031" || isContainerSystemPrivilegeError(value.Unwrap())
	case interface{ Unwrap() []error }:
		return slices.ContainsFunc(value.Unwrap(), isContainerSystemPrivilegeError)
	case interface{ Unwrap() error }:
		return isContainerSystemPrivilegeError(value.Unwrap())
	default:
		// go-ora can render the Oracle code without its zero padding.
		// Check only the leading code so a diagnostic mentioning a second
		// error does not turn an ambiguous ORA-00942 into a privilege denial.
		message := err.Error()
		if i := strings.IndexAny(message, ": \t\r\n"); i >= 0 {
			message = message[:i]
		}
		return message == "ORA-01031" || message == "ORA-1031"
	}
}
