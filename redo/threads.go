package redo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// ErrUnsupportedScope reports a redo thread request outside the admitted primary role.
var ErrUnsupportedScope = errors.New("Oracle redo thread source requires a primary database")

// ThreadsOptions identifies the numeric release and the observed database role.
type ThreadsOptions struct {
	OracleVersion string
	DatabaseRole  string
}

// ThreadsRequest carries caller admission, reference, and buffered limits.
type ThreadsRequest struct {
	Options   ThreadsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// Thread preserves the redo control-file observation, including raw flags and
// Oracle's nullable instance name. SourceInstanceID identifies the observer.
type Thread struct {
	ThreadNumber, Status, Enabled, InstanceName, SourceInstanceID session.Value
	ContainerID, SourceContainerID                                *session.Value
}

// ThreadsResult retains selected rows and source provenance.
type ThreadsResult struct {
	Provenance Provenance
	Rows       []Thread
}

// ThreadsStatement selects the reviewed projection without truncation.
func ThreadsStatement(options ThreadsOptions) (string, error) {
	p, err := threadsPlan(options)
	return p.sql, err
}

func threadsPlan(options ThreadsOptions) (selection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return selection{}, ErrUnsupportedVersion
	}
	// Oracle documents that V$THREAD does not return meaningful physical standby data.
	if options.DatabaseRole != "PRIMARY" {
		return selection{}, ErrUnsupportedScope
	}
	p := selection{
		sql: "SELECT " + exactNumber("v.THREAD#") + " AS THREAD_NUMBER, v.STATUS AS STATUS, v.ENABLED AS ENABLED, " +
			"v.INSTANCE AS INSTANCE_NAME, TO_NUMBER(SYS_CONTEXT('USERENV','INSTANCE')) AS SOURCE_INST_ID",
		schema: []rowset.Column{
			{Name: "THREAD_NUMBER", Kind: rowset.Text},
			{Name: "STATUS", Kind: rowset.Text},
			{Name: "ENABLED", Kind: rowset.Text},
			{Name: "INSTANCE_NAME", Kind: rowset.Text},
			{Name: "SOURCE_INST_ID", Kind: rowset.Integer},
		},
		integerText: []string{"THREAD_NUMBER"},
		provenance:  Provenance{Source: "V$THREAD", OracleMajor: major, Scope: "primary_visible_redo_threads"},
	}
	if major >= 12 {
		p.sql += ", v.CON_ID AS CON_ID, TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer}, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
		p.provenance.OptionalColumns = []string{"CON_ID", "SOURCE_CON_ID"}
	}
	p.sql += " FROM V$THREAD v ORDER BY v.THREAD#"
	return p, nil
}

// DecodeThreads validates the complete supplied projection before returning any rows.
func DecodeThreads(result session.Result, options ThreadsOptions) (ThreadsResult, error) {
	p, err := threadsPlan(options)
	if err != nil {
		return ThreadsResult{}, err
	}
	return decodeThreads(result, p)
}

func decodeThreads(result session.Result, p selection) (ThreadsResult, error) {
	index, err := columns(result, p)
	if err != nil {
		return ThreadsResult{}, err
	}
	rows := make([]Thread, len(result.Rows))
	for i, row := range result.Rows {
		item := Thread{
			ThreadNumber: row[index["THREAD_NUMBER"]], Status: row[index["STATUS"]], Enabled: row[index["ENABLED"]],
			InstanceName: row[index["INSTANCE_NAME"]], SourceInstanceID: row[index["SOURCE_INST_ID"]],
			ContainerID: optional(row, index, "CON_ID"), SourceContainerID: optional(row, index, "SOURCE_CON_ID"),
		}
		if item.SourceContainerID != nil && item.SourceContainerID.Null {
			return ThreadsResult{}, &Error{p.provenance.Source, ErrInvalidEvidence}
		}
		rows[i] = item
	}
	return ThreadsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadThreads executes once through the caller's admitted bounded reader.
func ReadThreads(ctx context.Context, reader Reader, request ThreadsRequest) (ThreadsResult, error) {
	p, err := threadsPlan(request.Options)
	if err != nil {
		return ThreadsResult{}, err
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return ThreadsResult{}, err
	}
	result, err := query(ctx, reader, request.Reference, p)
	if err != nil {
		return ThreadsResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return ThreadsResult{}, &Error{p.provenance.Source, errors.Join(rowset.ErrCapacity, ctx.Err())}
	}
	facts, decodeErr := decodeThreads(result, p)
	if err = errors.Join(decodeErr, ctx.Err()); err != nil {
		return ThreadsResult{}, &Error{p.provenance.Source, err}
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// ThreadsRows transfers the caller-native cursor.
func ThreadsRows(ctx context.Context, q rowset.Queryer, options ThreadsOptions) (*sql.Rows, error) {
	p, err := threadsPlan(options)
	if err != nil {
		return nil, err
	}
	return open(ctx, q, p)
}

// ThreadsJSON consumes and validates a separate complete bounded cursor.
func ThreadsJSON(ctx context.Context, q rowset.Queryer, options ThreadsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := threadsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodeThreads(result, p)
		return err
	})
}
