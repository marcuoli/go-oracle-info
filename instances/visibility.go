package instances

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

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
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
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
