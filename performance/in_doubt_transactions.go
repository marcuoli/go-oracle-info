package performance

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/marcuoli/go-oracle-info/internal/datetime"
	"github.com/marcuoli/go-oracle-info/internal/numeric"
	"github.com/marcuoli/go-oracle-info/internal/oracleversion"
	"github.com/marcuoli/go-oracle-info/internal/projection"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

// InDoubtTransactionsOptions admits the observed numeric version of the connected handle.
type InDoubtTransactionsOptions struct {
	OracleVersion string
}

// InDoubtTransactionsRequest supplies the caller reference and buffered bounds.
type InDoubtTransactionsRequest struct {
	Options   InDoubtTransactionsOptions
	Reference reference.Reference
	Limits    rowset.Limits
}

// InDoubtTransaction preserves visible distributed transaction state and Oracle's
// text commit number. Dates are Gregorian civil time with seconds and no zone.
// SourceContainerID is absent before 12.1 and describes the connected container.
type InDoubtTransaction struct {
	LocalTransactionID  session.Value
	GlobalTransactionID session.Value
	State               session.Value
	Mixed               session.Value
	Advice              session.Value
	FailTime            session.Value
	ForceTime           session.Value
	RetryTime           session.Value
	CommitNumber        session.Value
	SourceContainerID   *session.Value
}

// InDoubtTransactionsResult contains the selected rows and their connected-dictionary provenance.
type InDoubtTransactionsResult struct {
	Provenance Provenance
	Rows       []InDoubtTransaction
}

// InDoubtTransactionsStatement selects the reviewed DBA_2PC_PENDING projection.
func InDoubtTransactionsStatement(options InDoubtTransactionsOptions) (string, error) {
	p, err := inDoubtTransactionsPlan(options)
	return p.sql, err
}

func inDoubtTransactionsPlan(options InDoubtTransactionsOptions) (selection, error) {
	major, ok := oracleversion.LegacyFactMajor(options.OracleVersion)
	if !ok {
		return selection{}, ErrUnsupportedVersion
	}
	p := selection{
		sql: "SELECT v.LOCAL_TRAN_ID AS LOCAL_TRAN_ID" +
			", v.GLOBAL_TRAN_ID AS GLOBAL_TRAN_ID" +
			", v.STATE AS STATE" +
			", v.MIXED AS MIXED" +
			", v.ADVICE AS ADVICE" +
			", " + datetime.GregorianDate("v.FAIL_TIME") + " AS FAIL_TIME" +
			", " + datetime.GregorianDate("v.FORCE_TIME") + " AS FORCE_TIME" +
			", " + datetime.GregorianDate("v.RETRY_TIME") + " AS RETRY_TIME" +
			", v.COMMIT# AS COMMIT_NUMBER",
		schema: []rowset.Column{
			{Name: "LOCAL_TRAN_ID", Kind: rowset.Text},
			{Name: "GLOBAL_TRAN_ID", Kind: rowset.Text},
			{Name: "STATE", Kind: rowset.Text},
			{Name: "MIXED", Kind: rowset.Text},
			{Name: "ADVICE", Kind: rowset.Text},
			{Name: "FAIL_TIME", Kind: rowset.Text},
			{Name: "FORCE_TIME", Kind: rowset.Text},
			{Name: "RETRY_TIME", Kind: rowset.Text},
			{Name: "COMMIT_NUMBER", Kind: rowset.Text},
		},
		provenance: Provenance{Source: "DBA_2PC_PENDING", OracleMajor: major, Scope: "connected_dictionary"},
	}
	if major >= 12 {
		p.sql += ", TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')) AS SOURCE_CON_ID"
		p.schema = append(p.schema, rowset.Column{Name: "SOURCE_CON_ID", Kind: rowset.Integer})
	}
	p.sql += " FROM SYS.DBA_2PC_PENDING v"
	return p, nil
}

// DecodeInDoubtTransactions validates every row and preserves source text and SQL NULL.
func DecodeInDoubtTransactions(result session.Result, options InDoubtTransactionsOptions) (InDoubtTransactionsResult, error) {
	p, err := inDoubtTransactionsPlan(options)
	if err != nil {
		return InDoubtTransactionsResult{}, err
	}
	return decodeInDoubtTransactions(result, p)
}

func decodeInDoubtTransactions(result session.Result, p selection) (InDoubtTransactionsResult, error) {
	index, err := columns(result, p)
	if err != nil {
		return InDoubtTransactionsResult{}, err
	}
	rows := make([]InDoubtTransaction, len(result.Rows))
	for i, row := range result.Rows {
		v := InDoubtTransaction{
			LocalTransactionID:  row[index["LOCAL_TRAN_ID"]],
			GlobalTransactionID: row[index["GLOBAL_TRAN_ID"]],
			State:               row[index["STATE"]],
			Mixed:               row[index["MIXED"]],
			Advice:              row[index["ADVICE"]],
			FailTime:            row[index["FAIL_TIME"]],
			ForceTime:           row[index["FORCE_TIME"]],
			RetryTime:           row[index["RETRY_TIME"]],
			CommitNumber:        row[index["COMMIT_NUMBER"]],
		}
		// Oracle requires a local identity, state and both recovery timestamps.
		if v.LocalTransactionID.Text == "" || v.State.Text == "" || v.FailTime.Null || v.RetryTime.Null {
			return InDoubtTransactionsResult{}, ErrInvalidEvidence
		}
		for _, date := range []session.Value{v.FailTime, v.ForceTime, v.RetryTime} {
			if !date.Null {
				const civil = "2006-01-02T15:04:05"
				if _, err := time.Parse(civil, date.Text); len(date.Text) != len(civil) || err != nil {
					return InDoubtTransactionsResult{}, ErrInvalidEvidence
				}
			}
		}
		if j, ok := index["SOURCE_CON_ID"]; ok {
			source := row[j]
			if source.Null || !numeric.IsUnsignedInteger(source.Text) {
				return InDoubtTransactionsResult{}, ErrInvalidEvidence
			}
			v.SourceContainerID = &source
		}
		rows[i] = v
	}
	return InDoubtTransactionsResult{Provenance: p.provenance, Rows: rows}, nil
}

// ReadInDoubtTransactions executes once over an admitted complete reader.
func ReadInDoubtTransactions(ctx context.Context, reader Reader, request InDoubtTransactionsRequest) (InDoubtTransactionsResult, error) {
	p, err := inDoubtTransactionsPlan(request.Options)
	if err != nil {
		return InDoubtTransactionsResult{}, err
	}
	if !validReader(reader) || request.Reference.Validate() != nil {
		return InDoubtTransactionsResult{}, ErrInvalidRequest
	}
	limits, err := rowset.Resolve(request.Limits)
	if err != nil {
		return InDoubtTransactionsResult{}, err
	}
	if err := readError(ctx, p, nil); err != nil {
		return InDoubtTransactionsResult{}, err
	}
	result, err := reader.Query(ctx, session.Statement{Reference: request.Reference, Text: p.sql})
	if err := readError(ctx, p, err); err != nil {
		return InDoubtTransactionsResult{}, err
	}
	if !projection.FitsBufferedLimits(result, limits) {
		return InDoubtTransactionsResult{}, readError(ctx, p, rowset.ErrCapacity)
	}
	facts, err := decodeInDoubtTransactions(result, p)
	if err := readError(ctx, p, err); err != nil {
		return InDoubtTransactionsResult{}, err
	}
	facts.Provenance.Complete = true
	return facts, nil
}

// InDoubtTransactionsRows transfers the caller-native cursor.
func InDoubtTransactionsRows(ctx context.Context, q rowset.Queryer, options InDoubtTransactionsOptions) (*sql.Rows, error) {
	p, err := inDoubtTransactionsPlan(options)
	if err != nil {
		return nil, err
	}
	rows, err := rowset.Query(ctx, q, p.sql)
	if err != nil {
		return nil, &Error{p.provenance.Source, err}
	}
	return rows, nil
}

// InDoubtTransactionsJSON consumes a separate complete bounded cursor.
func InDoubtTransactionsJSON(ctx context.Context, q rowset.Queryer, options InDoubtTransactionsOptions, limits rowset.Limits) (json.RawMessage, error) {
	p, err := inDoubtTransactionsPlan(options)
	if err != nil {
		return nil, err
	}
	return readJSON(ctx, q, p, limits, func(r session.Result) error {
		_, err := decodeInDoubtTransactions(r, p)
		return err
	})
}
