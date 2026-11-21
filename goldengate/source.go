// Package goldengate exposes database-visible integrated replication facts.
// It owns no connection, process control, synchronization test or health policy.
package goldengate

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Reader returns complete bounded results, retaining terminal and close errors.
// Its owner admits the source and retains cancellation and connection lifetime.
type Reader interface {
	Query(context.Context, session.Statement) (session.Result, error)
}

// Request admits an observed numeric Oracle version and caller-owned reference.
// Capture/apply use a conservative 11.2.0.4 floor; administrator facts have their
// own explicit release admission. No source uses an error-triggered probe/fallback.
type Request struct {
	OracleVersion string
	Reference     reference.Reference
}

// Provenance identifies the selected source and admitted scope. Complete describes
// transport, not complete container, RAC, deployment or process coverage.
type Provenance struct {
	Source        string `json:"source"`
	ObjectOwner   string `json:"object_owner,omitempty"`
	ObjectName    string `json:"object_name,omitempty"`
	Scope         string `json:"scope"`
	OracleVersion string `json:"oracle_version"`
	Complete      bool   `json:"complete"`
}

var (
	// ErrUnsupportedVersion rejects unsupported or insufficient release evidence.
	ErrUnsupportedVersion = errors.New("unsupported Oracle GoldenGate source version")
	// ErrInvalidRequest rejects missing readers or invalid SQL references.
	ErrInvalidRequest = errors.New("invalid Oracle GoldenGate request")
	// ErrInvalidEvidence rejects malformed complete source results.
	ErrInvalidEvidence = errors.New("invalid Oracle GoldenGate evidence")
)

// Error formats only a static source, preserving private causes for errors.Is/As.
type Error struct {
	Source string
	cause  error
}

// Error excludes driver details which may contain private inputs.
func (e *Error) Error() string { return "GoldenGate fact read failed (" + e.Source + ")" }

// Unwrap retains the cause for deliberate diagnostics.
func (e *Error) Unwrap() error { return e.cause }

type selection struct {
	sql        string
	args       []any
	required   []string
	decimals   []string
	timestamps []string
	filters    []discoveryPredicate
	catalog    bool
	ggPurpose  bool
	schema     []rowset.Column
	provenance Provenance
}

func plan(version string, apply bool) (selection, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 5 {
		return selection{}, ErrUnsupportedVersion
	}
	var nums [5]int
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return selection{}, ErrUnsupportedVersion
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return selection{}, ErrUnsupportedVersion
		}
		nums[i] = n
	}
	if nums[0] < 11 || nums[0] == 11 && (nums[1] < 2 || nums[1] == 2 && nums[2] == 0 && nums[3] < 4) {
		return selection{}, ErrUnsupportedVersion
	}
	schema := []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}, {Name: "CAPTURE_NAME", Kind: rowset.Text}, {Name: "STATE", Kind: rowset.Text}, {Name: "CAPTURE_MESSAGE_NUMBER", Kind: rowset.Text}, {Name: "TOTAL_MESSAGES_CAPTURED", Kind: rowset.Integer}}
	source := "GV$GOLDENGATE_CAPTURE"
	if apply {
		source = "GV$GG_APPLY_COORDINATOR"
		schema = []rowset.Column{{Name: "INST_ID", Kind: rowset.Integer}, {Name: "APPLY_NAME", Kind: rowset.Text}, {Name: "STATE", Kind: rowset.Text}, {Name: "TOTAL_APPLIED", Kind: rowset.Integer}, {Name: "TOTAL_ERRORS", Kind: rowset.Integer}, {Name: "LWM_MESSAGE_NUMBER", Kind: rowset.Text}}
	}
	if nums[0] >= 12 {
		schema = append(schema, rowset.Column{Name: "CON_ID", Kind: rowset.Integer})
	}
	aliases := make([]string, len(schema))
	for i, c := range schema {
		expression := "v." + c.Name
		// Message positions are Oracle NUMBER, but their public contract is exact
		// text beyond int64. Project VARCHAR2 explicitly for native JSON too (#52).
		switch c.Name {
		case "CAPTURE_MESSAGE_NUMBER", "LWM_MESSAGE_NUMBER":
			expression = "TO_CHAR(" + expression + ",'TM9','NLS_NUMERIC_CHARACTERS=''.,''')"
		}
		aliases[i] = expression + " AS " + c.Name
	}
	order := "v.INST_ID"
	if nums[0] >= 12 {
		order += ", v.CON_ID"
	}
	order += ", v." + schema[1].Name
	return selection{sql: "SELECT " + strings.Join(aliases, ", ") + " FROM " + source + " v ORDER BY " + order, schema: schema, provenance: Provenance{Source: source, Scope: "qualified_visible_instances_current_container", OracleVersion: version}}, nil
}
func columns(result session.Result, p selection) (map[string]int, error) {
	names := make([]string, len(p.schema))
	for i, c := range p.schema {
		names[i] = c.Name
	}
	index, ok := projection.Columns(result, names...)
	if !ok {
		return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
	}
	for _, row := range result.Rows {
		if pos, exists := index["USERNAME"]; exists {
			if row[pos].Null || row[pos].Text == "" {
				return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
			}
		}
		for _, c := range p.schema {
			v := row[index[c.Name]]
			if !utf8.ValidString(v.Text) || v.Null && v.Text != "" {
				return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
			}
			integer := c.Kind == rowset.Integer || strings.HasSuffix(c.Name, "MESSAGE_NUMBER")
			if !v.Null && integer && (!numeric.IsUnsignedInteger(v.Text) || (len(v.Text) > 1 && v.Text[0] == '0')) {
				return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
			}
			if c.Name == "INST_ID" && (v.Null || v.Text == "0") {
				return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
			}
		}
	}
	if err := discoveryColumns(result, index, p); err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return index, nil
}
func optional(row []session.Value, index map[string]int) *session.Value {
	if pos, ok := index["CON_ID"]; ok {
		v := row[pos]
		return &v
	}
	return nil
}
func query(ctx context.Context, r Reader, request Request, p selection) (session.Result, error) {
	if r == nil || reflect.ValueOf(r).Kind() == reflect.Pointer && reflect.ValueOf(r).IsNil() || request.Reference.Validate() != nil {
		return session.Result{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return session.Result{}, &Error{p.provenance.Source, err}
	}
	result, err := r.Query(ctx, session.Statement{Text: p.sql, Args: p.args, Reference: request.Reference})
	if err = errors.Join(err, ctx.Err()); err != nil {
		return session.Result{}, &Error{p.provenance.Source, err}
	}
	return result, nil
}
func nativeJSON(ctx context.Context, q rowset.Queryer, p selection, limits rowset.Limits) (json.RawMessage, error) {
	resolved, err := rowset.Resolve(limits)
	if err != nil {
		return nil, err
	}
	p.provenance.Complete = true
	envelope := struct {
		Provenance Provenance      `json:"provenance"`
		Data       json.RawMessage `json:"data"`
	}{Provenance: p.provenance}
	metadata, _ := json.Marshal(envelope)
	resolved.MaxBytes -= len(metadata) - 4
	if resolved.MaxBytes < 1 {
		return nil, &Error{p.provenance.Source, rowset.ErrCapacity}
	}
	rows, err := rowset.Query(ctx, q, p.sql, p.args...)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	table, err := rowset.Read(ctx, rows, p.schema, resolved)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	result, ok := projection.FromRowset(table)
	if !ok {
		return nil, &Error{p.provenance.Source, ErrInvalidEvidence}
	}
	if _, err = columns(result, p); err != nil {
		return nil, err
	}
	envelope.Data, err = json.Marshal(table)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(envelope)
	original, _ := rowset.Resolve(limits)
	if len(payload) > original.MaxBytes {
		err = errors.Join(err, rowset.ErrCapacity)
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return payload, nil
}
