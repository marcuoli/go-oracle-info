// Package instances observes qualified instance visibility without admitting or
// connecting to members, or claiming complete physical RAC membership.
package instances

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/marcuoli/go-oracle-info/internal/legacyversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// Instance retains one source row without collapsing scopes or normalizing flags.
// InstanceID is a validated positive reporting ID. Numeric fields keep exact text;
// ContainerID nil means the column is absent on11g, not a projected SQL NULL.
type Instance struct {
	InstanceID     session.Value
	ContainerID    *session.Value
	InstanceNumber session.Value
	Name           session.Value
	HostName       session.Value
	Status         session.Value
	DatabaseStatus session.Value
	Parallel       session.Value
	Blocked        session.Value
}

// Result is visibility evidence only. Neither Available nor NoRows certifies
// complete physical membership, actual network reachability or parameter coverage.
type Result struct {
	State      State
	Provenance Provenance
	Rows       []Instance
}

// JSONEnvelope keeps source/version/scope apart from ordered native row data.
type JSONEnvelope struct {
	Provenance Provenance      `json:"provenance"`
	Data       json.RawMessage `json:"data"`
}

// Decode validates the whole shape/values before returning any member rows.
// Repeated instance observations are preserved, never silently deduplicated.
func Decode(result session.Result, major int) (Result, error) {
	plan, err := instancePlan(major)
	if err != nil {
		return Result{State: Unavailable}, err
	}
	return decode(result, plan)
}

// Read executes one global visibility source. An error discards every returned
// prefix; callers must not substitute the connected instance as complete coverage.
func Read(ctx context.Context, reader Reader, request Request) (Result, error) {
	plan, err := instancePlan(request.OracleMajor)
	if err != nil {
		return Result{State: Unavailable}, err
	}
	unavailable := Result{State: Unavailable, Provenance: plan.provenance}
	result, err := queryInventory(ctx, reader, request.Reference, plan)
	if err != nil {
		return unavailable, err
	}
	facts, decodeErr := decode(result, plan)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return unavailable, &Error{Operation: "read", cause: err}
	}
	return facts, nil
}

// Rows returns the exact caller-owned native cursor. Its owner supplies streaming
// deadlines/budgets, Next/Scan/Err checks and Close on every path.
func Rows(ctx context.Context, q rowset.Queryer, major int) (*sql.Rows, error) {
	plan, err := instancePlan(major)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// JSON executes the same global source independently and delegates complete
// bounded cursor consumption to session/rowset. Any error returns nil JSON.
func JSON(ctx context.Context, q rowset.Queryer, major int, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := instancePlan(major)
	if err != nil {
		return nil, err
	}
	return inventoryJSON(ctx, q, plan, plan.provenance, limits)
}

func inventoryJSON(ctx context.Context, q rowset.Queryer, plan selection, provenance any, limits rowset.Limits) (json.RawMessage, error) {
	payload, err := rowset.QueryEnvelopeJSON(ctx, rowset.OpenQuery(q, plan.sql), plan.schema, limits, rowset.EnvelopeOptions{
		Provenance: provenance,
		Validate: projection.ValidateRowset(&Error{Operation: "decode", cause: ErrInvalidEvidence}, func(result session.Result) error {
			_, err := columns(result, plan.schema)
			return err
		}),
	})
	return payload, projection.WrapEnvelopeError(err, func(stage string, cause error) error {
		switch stage {
		case "header", "encode":
			return &Error{Operation: "encode", cause: cause}
		case "query":
			failure := &Error{Operation: stage, cause: cause}
			if plan.classifyAccess {
				return projection.ClassifySourceError(failure)
			}
			return failure
		case "budget", "consume":
			return &Error{Operation: stage, cause: cause}
		}
		return cause
	})
}

func decode(result session.Result, plan selection) (Result, error) {
	unavailable := Result{State: Unavailable, Provenance: plan.provenance}
	fail := func() (Result, error) { return unavailable, &Error{Operation: "decode", cause: ErrInvalidEvidence} }
	index, err := columns(result, plan.schema)
	if err != nil {
		return fail()
	}
	rows := make([]Instance, len(result.Rows))
	for i, row := range result.Rows {
		record := Instance{InstanceID: row[index["INST_ID"]], InstanceNumber: row[index["INSTANCE_NUMBER"]], Name: row[index["INSTANCE_NAME"]], HostName: row[index["HOST_NAME"]], Status: row[index["STATUS"]], DatabaseStatus: row[index["DATABASE_STATUS"]], Parallel: row[index["PARALLEL"]], Blocked: row[index["BLOCKED"]]}
		if position, present := index["CON_ID"]; present {
			value := row[position]
			record.ContainerID = &value
		}
		rows[i] = record
	}
	state := Available
	if len(rows) == 0 {
		state = NoRows
	}
	return Result{State: state, Provenance: plan.provenance, Rows: rows}, nil
}

// InventoryOptions admits the Oracle projection for visible instances.
type InventoryOptions struct {
	OracleMajor int
}

// InventoryRequest adds the caller's SQL reference to the source options.
type InventoryRequest struct {
	Options   InventoryOptions
	Reference reference.Reference
}

// InventoryStatement is the named form of Statement.
func InventoryStatement(options InventoryOptions) (string, error) {
	return Statement(options.OracleMajor)
}

// DecodeInventory is the named form of Decode.
func DecodeInventory(result session.Result, options InventoryOptions) (Result, error) {
	return Decode(result, options.OracleMajor)
}

// ReadInventory is the named form of Read.
func ReadInventory(ctx context.Context, reader Reader, request InventoryRequest) (Result, error) {
	return Read(ctx, reader, Request{OracleMajor: request.Options.OracleMajor, Reference: request.Reference})
}

// InventoryRows is the named form of Rows.
func InventoryRows(ctx context.Context, q rowset.Queryer, options InventoryOptions) (*sql.Rows, error) {
	return Rows(ctx, q, options.OracleMajor)
}

// InventoryJSON is the named form of JSON.
func InventoryJSON(ctx context.Context, q rowset.Queryer, options InventoryOptions, limits rowset.Limits) (json.RawMessage, error) {
	return JSON(ctx, q, options.OracleMajor, limits)
}

// Statement selects all qualified visible GV$INSTANCE rows in the admitted
// container scope, including raw mounted/blocked states. It never filters to the
// connected instance or only open PDB members; reachability policy belongs outside.
func Statement(major int) (string, error) {
	plan, err := instancePlan(major)
	return plan.sql, err
}

func instancePlan(major int) (selection, error) {
	if major < 11 {
		return selection{}, ErrUnsupportedVersion
	}
	return selectInventory(major), nil
}

func selectInventory(major int) selection {
	columns := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}}
	scope := "visible_qualified_instances"
	if major >= 12 {
		columns = append(columns, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
		scope = "current_container_and_whole_database_qualified_instances"
	}
	columns = append(columns, rowset.Column{Name: "INSTANCE_NUMBER", Kind: rowset.Integer})
	for _, name := range []string{"INSTANCE_NAME", "HOST_NAME", "STATUS", "DATABASE_STATUS", "PARALLEL"} {
		columns = append(columns, rowset.Column{Name: name, Kind: rowset.Text})
	}
	if major >= 11 {
		columns = append(columns, rowset.Column{Name: "BLOCKED", Kind: rowset.Text})
	}
	aliases := make([]string, len(columns))
	for i, column := range columns {
		aliases[i] = "i." + column.Name + " AS " + column.Name
	}
	text := "SELECT " + strings.Join(aliases, ", ") + " FROM GV$INSTANCE i"
	if major >= 12 {
		text += " WHERE i.CON_ID IN (0,TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')))"
	}
	text += " ORDER BY i.INST_ID"
	if major >= 12 {
		text += ", i.CON_ID"
	}
	text += ", i.INSTANCE_NUMBER"
	return selection{sql: text, schema: columns, provenance: Provenance{Source: "GV$INSTANCE", OracleMajor: major, Scope: scope}}
}

func queryInventory(ctx context.Context, reader Reader, ref reference.Reference, plan selection) (session.Result, error) {
	if reader == nil || (reflect.ValueOf(reader).Kind() == reflect.Pointer && reflect.ValueOf(reader).IsNil()) || ref.Validate() != nil {
		return session.Result{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return session.Result{}, &Error{Operation: "query", cause: err}
	}
	result, err := reader.Query(ctx, session.Statement{Reference: ref, Text: plan.sql})
	if err = errors.Join(err, ctx.Err()); err != nil {
		failure := &Error{Operation: "query", cause: err}
		if plan.classifyAccess {
			return session.Result{}, projection.ClassifySourceError(failure)
		}
		return session.Result{}, failure
	}
	return result, nil
}

// VisibilityOptions carries the complete numeric version observed on the admitted
// database. The reader or native handle already belongs to the intended container.
type VisibilityOptions struct {
	OracleVersion string
}

// VisibilityRequest adds the caller's SQL reference to the admitted options.
type VisibilityRequest struct {
	Options   VisibilityOptions
	Reference reference.Reference
}

// VisibilityProvenance retains the observed version as well as selected source scope.
type VisibilityProvenance struct {
	Provenance
	OracleVersion string `json:"oracle_version"`
	Complete      bool   `json:"complete"`
}

// VisibilityRow is one qualified GV$INSTANCE row. A nil Blocked or ContainerID
// means the source column is absent. A present value can be SQL NULL.
type VisibilityRow struct {
	InstanceID     session.Value
	ContainerID    *session.Value
	InstanceNumber session.Value
	Name           session.Value
	HostName       session.Value
	Status         session.Value
	DatabaseStatus session.Value
	Parallel       session.Value
	Blocked        *session.Value
}

// VisibilityResult distinguishes successful empty, unsupported, inaccessible,
// unavailable and failed reads. Rows describe only qualified visible instances.
type VisibilityResult struct {
	State      State
	Provenance VisibilityProvenance
	Rows       []VisibilityRow
}

// VisibilityStatement selects the reviewed GV$INSTANCE projection for the
// observed numeric release without a local, privilege or container fallback.
func VisibilityStatement(options VisibilityOptions) (string, error) {
	plan, err := visibilityPlan(options)
	return plan.sql, err
}

func visibilityPlan(options VisibilityOptions) (selection, error) {
	major, ok := legacyversion.Major(options.OracleVersion)
	if !ok {
		return selection{}, ErrUnsupportedVersion
	}
	plan := selectInventory(major)
	plan.classifyAccess = true
	return plan, nil
}

// DecodeVisibility validates the entire selected projection and preserves row
// ordering, duplicates and raw flags. It cannot establish transport completion.
func DecodeVisibility(result session.Result, options VisibilityOptions) (VisibilityResult, error) {
	plan, err := visibilityPlan(options)
	if err != nil {
		return VisibilityResult{State: Unsupported}, err
	}
	return decodeVisibility(result, plan, options.OracleVersion)
}

func decodeVisibility(result session.Result, plan selection, version string) (VisibilityResult, error) {
	facts := VisibilityResult{State: Failed, Provenance: VisibilityProvenance{Provenance: plan.provenance, OracleVersion: version}}
	index, err := columns(result, plan.schema)
	if err != nil {
		return facts, &Error{Operation: "decode", cause: err}
	}
	facts.Rows = make([]VisibilityRow, len(result.Rows))
	facts.State = NoRows
	if len(facts.Rows) > 0 {
		facts.State = Available
	}
	for i, row := range result.Rows {
		record := VisibilityRow{
			InstanceID: row[index["INST_ID"]], InstanceNumber: row[index["INSTANCE_NUMBER"]],
			Name: row[index["INSTANCE_NAME"]], HostName: row[index["HOST_NAME"]],
			Status: row[index["STATUS"]], DatabaseStatus: row[index["DATABASE_STATUS"]],
			Parallel: row[index["PARALLEL"]],
		}
		if p, ok := index["CON_ID"]; ok {
			value := row[p]
			record.ContainerID = &value
		}
		if p, ok := index["BLOCKED"]; ok {
			value := row[p]
			record.Blocked = &value
		}
		facts.Rows[i] = record
	}
	return facts, nil
}

// ReadVisibility executes the selected source once on the admitted reader.
// Query, decode and cancellation failures return no rows and retain their causes.
func ReadVisibility(ctx context.Context, reader Reader, request VisibilityRequest) (VisibilityResult, error) {
	plan, err := visibilityPlan(request.Options)
	if err != nil {
		return VisibilityResult{State: Unsupported}, err
	}
	failure := VisibilityResult{State: Failed, Provenance: VisibilityProvenance{Provenance: plan.provenance, OracleVersion: request.Options.OracleVersion}}
	result, err := queryInventory(ctx, reader, request.Reference, plan)
	if err != nil {
		switch {
		case errors.Is(err, ErrSourceInaccessible):
			failure.State = Inaccessible
		case errors.Is(err, ErrSourceUnavailable):
			failure.State = Unavailable
		}
		return failure, err
	}
	facts, decodeErr := decodeVisibility(result, plan, request.Options.OracleVersion)
	if err := errors.Join(decodeErr, ctx.Err()); err != nil {
		return failure, &Error{Operation: "read", cause: err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// VisibilityRows returns the actual caller-native cursor for the selected source.
// Its caller owns iteration, terminal checks, Close, deadlines and stream budgets.
func VisibilityRows(ctx context.Context, q rowset.Queryer, options VisibilityOptions) (*sql.Rows, error) {
	plan, err := visibilityPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, plan)
}

// VisibilityJSON independently reads the same projection and returns a complete
// bounded JSON envelope. An error returns nil JSON.
func VisibilityJSON(ctx context.Context, q rowset.Queryer, options VisibilityOptions, limits rowset.Limits) (json.RawMessage, error) {
	plan, err := visibilityPlan(options)
	if err != nil {
		return nil, err
	}
	provenance := VisibilityProvenance{Provenance: plan.provenance, OracleVersion: options.OracleVersion, Complete: true}
	return inventoryJSON(ctx, q, plan, provenance, limits)
}
