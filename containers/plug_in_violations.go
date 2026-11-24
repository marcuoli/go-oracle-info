package containers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// PlugInViolationsOptions admits a numeric release and caller-established scope.
// Root includes visible prospective PDBs. CurrentContainer selects only the
// connected container's ID and requires Oracle 12.2 or later.
type PlugInViolationsOptions struct {
	OracleVersion string
	Scope         Scope
}

// PlugInViolationsRequest adds a caller-owned SQL reference and buffered bounds.
type PlugInViolationsRequest struct {
	Options   PlugInViolationsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// PlugInViolation preserves one recorded finding, including raw status, type
// and text. Time is Gregorian civil text with six fractional digits and no
// timezone. A nil ContainerID denotes Oracle 12.1's unprojected column.
// On newer releases a nonnil ID can still carry SQL NULL or exact zero.
type PlugInViolation struct {
	Time        session.Value
	Name        session.Value
	Cause       session.Value
	Type        session.Value
	ErrorNumber session.Value
	Line        session.Value
	Message     session.Value
	Status      session.Value
	Action      session.Value
	ContainerID *session.Value
}

// PlugInViolationsResult contains the recorded findings visible to this read.
// Empty Rows describes this observation and cannot attest plug compatibility.
type PlugInViolationsResult struct {
	Provenance Provenance
	Rows       []PlugInViolation
}

// PlugInViolationsStatement selects recorded findings without running a plug check.
func PlugInViolationsStatement(options PlugInViolationsOptions) (string, error) {
	p, err := plugInViolationsPlan(options)
	return p.sql, err
}

func plugInViolationsPlan(options PlugInViolationsOptions) (selection, error) {
	r, err := profile(options.OracleVersion, options.Scope)
	if err != nil {
		return selection{}, err
	}
	predicate := connectedContainer + " = 1"
	if options.Scope == CurrentContainer {
		if r < release122 {
			return selection{}, ErrUnsupportedScope
		}
		predicate = "v.CON_ID >= 1 AND v.CON_ID = " + connectedContainer
	}

	fields := []field{
		projected("TIME", datetime.GregorianTimestamp6("v.TIME"), rowset.Text),
		raw("NAME", rowset.Text), raw("CAUSE", rowset.Text), raw("TYPE", rowset.Text),
		exact("ERROR_NUMBER"), exact("LINE"), raw("MESSAGE", rowset.Text),
		raw("STATUS", rowset.Text), raw("ACTION", rowset.Text),
	}
	var extras []field
	order := "v.TIME, v.NAME, v.LINE, v.CAUSE, v.TYPE, v.ERROR_NUMBER, v.STATUS"
	if r >= release122 {
		extras = []field{exact("CON_ID")}
		order = "v.CON_ID, " + order
	}
	provenance := Provenance{Source: "PDB_PLUG_IN_VIOLATIONS", OracleVersion: options.OracleVersion, Scope: options.Scope}
	return selectFields(provenance.Source, provenance, fields, extras, predicate, order), nil
}

// DecodePlugInViolations validates the selected projection and retains raw findings.
func DecodePlugInViolations(result session.Result, options PlugInViolationsOptions) (PlugInViolationsResult, error) {
	p, err := plugInViolationsPlan(options)
	if err != nil {
		return PlugInViolationsResult{}, err
	}
	return decodePlugInViolations(result, p)
}

func decodePlugInViolations(result session.Result, p selection) (PlugInViolationsResult, error) {
	index, err := resultIndex(result, p)
	if err != nil {
		return PlugInViolationsResult{}, err
	}
	rows := make([]PlugInViolation, len(result.Rows))
	for i, row := range result.Rows {
		v := PlugInViolation{
			Time: row[index["TIME"]], Name: row[index["NAME"]], Cause: row[index["CAUSE"]],
			Type: row[index["TYPE"]], ErrorNumber: row[index["ERROR_NUMBER"]], Line: row[index["LINE"]],
			Message: row[index["MESSAGE"]], Status: row[index["STATUS"]], Action: row[index["ACTION"]],
			ContainerID: optional(row, index, "CON_ID"),
		}
		const timestamp = "2006-01-02T15:04:05.000000"
		_, timeErr := time.Parse(timestamp, v.Time.Text)
		if v.Time.Null || len(v.Time.Text) != len(timestamp) || timeErr != nil || v.Time.Text[19] != '.' ||
			!requiredText(v.Name) || !requiredText(v.Cause) || !requiredText(v.Type) ||
			!requiredText(v.Message) || v.Line.Null || !numeric.IsSignedInteger(v.Line.Text) ||
			(!v.ErrorNumber.Null && !numeric.IsSignedInteger(v.ErrorNumber.Text)) {
			return PlugInViolationsResult{}, ErrInvalidEvidence
		}
		if v.ContainerID != nil && (!unsigned(*v.ContainerID) ||
			(p.provenance.Scope == CurrentContainer && id(*v.ContainerID) < 1)) {
			return PlugInViolationsResult{}, ErrInvalidEvidence
		}
		rows[i] = v
	}
	return PlugInViolationsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadPlugInViolations returns the complete bounded observation or an error.
func ReadPlugInViolations(ctx context.Context, reader Reader, request PlugInViolationsRequest) (PlugInViolationsResult, error) {
	p, err := plugInViolationsPlan(request.Options)
	if err != nil {
		return PlugInViolationsResult{}, err
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return PlugInViolationsResult{}, err
	}
	result, err := readSource(ctx, reader, p, request.Reference)
	if err != nil {
		return PlugInViolationsResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return PlugInViolationsResult{}, sourceFailure(p.provenance.Source, errors.Join(rowset.ErrCapacity, ctx.Err()), false)
	}
	facts, err := decodePlugInViolations(result, p)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return PlugInViolationsResult{}, sourceFailure(p.provenance.Source, err, false)
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// PlugInViolationsRows transfers the actual native cursor to its caller.
func PlugInViolationsRows(ctx context.Context, q rowset.Queryer, options PlugInViolationsOptions) (*sql.Rows, error) {
	p, err := plugInViolationsPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	return rows, sourceFailure(p.provenance.Source, err, true)
}

// PlugInViolationsJSON returns an independently consumed complete bounded dataset.
func PlugInViolationsJSON(ctx context.Context, q rowset.Queryer, options PlugInViolationsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := plugInViolationsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(result session.Result) error {
		_, err := decodePlugInViolations(result, p)
		return err
	})
}
