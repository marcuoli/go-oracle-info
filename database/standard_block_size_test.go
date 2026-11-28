package database

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
)

const blockSizeParameterSQL = "SELECT p.VALUE AS BLOCK_SIZE_BYTES FROM V$PARAMETER p WHERE p.NAME = 'db_block_size'"
const blockSizeSystemSQL = "SELECT t.BLOCK_SIZE AS BLOCK_SIZE_BYTES FROM DBA_TABLESPACES t WHERE t.TABLESPACE_NAME = 'SYSTEM'"

func TestStandardBlockSizeSelectsBothSourcesThrough26ai(t *testing.T) {
	for _, version := range []string{
		"9.2.0.8.0", "10.1.0.5.0", "10.2.0.5.0", "11.1.0.7.0", "11.2.0.4.0",
		"12.1.0.1.0", "12.1.0.2.0", "12.2.0.1.0", "18.4.0.0.0", "19.33.0.1.0",
		"21.21.0.0.0", "23.2.0.0.0", "23.9.0.0.0", "23.26.1.0.0", "23.26.4.0.0",
	} {
		opts := StandardBlockSizeOptions{OracleVersion: version}
		got, err := StandardBlockSizeStatement(opts)
		if err != nil || got != blockSizeParameterSQL {
			t.Fatalf("%s primary selected %q: %v", version, got, err)
		}
		opts.Source = StandardBlockSizeSystemTablespace
		got, err = StandardBlockSizeStatement(opts)
		if err != nil || got != blockSizeSystemSQL {
			t.Fatalf("%s fallback selected %q: %v", version, got, err)
		}
	}
	for _, version := range []string{"", "9.1.0.0.0", "10.3.0.0.0", "26ai", "26.1.0.0.0"} {
		opts := StandardBlockSizeOptions{OracleVersion: version}
		result, err := ReadStandardBlockSize(t.Context(), datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
			t.Fatal("unsupported version reached SQL")
			return session.Result{}, nil
		}), StandardBlockSizeRequest{Options: opts, Reference: coreReference(t)})
		if !errors.Is(err, ErrUnsupportedVersion) || result.Rows != nil {
			t.Fatalf("unexpected result for %q: %+v %v", version, result, err)
		}
	}
}

func TestStandardBlockSizeFallbackPreservesEvidence(t *testing.T) {
	good := coreResult("BLOCK_SIZE_BYTES", session.Value{Text: "8192"})
	null := coreResult("BLOCK_SIZE_BYTES", session.Value{Null: true})
	empty := session.Result{Columns: good.Columns}
	denied := errors.New("ORA-01031: private diagnostics")
	unavailable := errors.New("ORA-00942: private diagnostics")
	cleanup := errors.New("cleanup cause")
	for _, tc := range []struct {
		name         string
		primary      session.Result
		primaryErr   error
		fallback     session.Result
		fallbackErr  error
		allow        bool
		wantSources  []string
		wantAttempts []StandardBlockSizeAttempt
		wantSource   StandardBlockSizeSource
		wantRows     []StandardBlockSizeRow
		wantErr      error
	}{
		{"positive stops", good, nil, session.Result{}, nil, true, []string{blockSizeParameterSQL}, []StandardBlockSizeAttempt{{StandardBlockSizeParameter, "available"}}, StandardBlockSizeParameter, []StandardBlockSizeRow{{Bytes: session.Value{Text: "8192"}}}, nil},
		{"null falls back", null, nil, good, nil, true, []string{blockSizeParameterSQL, blockSizeSystemSQL}, []StandardBlockSizeAttempt{{StandardBlockSizeParameter, "null"}, {StandardBlockSizeSystemTablespace, "available"}}, StandardBlockSizeSystemTablespace, []StandardBlockSizeRow{{Bytes: session.Value{Text: "8192"}}}, nil},
		{"null remains when system invisible", null, nil, empty, nil, true, []string{blockSizeParameterSQL, blockSizeSystemSQL}, []StandardBlockSizeAttempt{{StandardBlockSizeParameter, "null"}, {StandardBlockSizeSystemTablespace, "no_rows"}}, StandardBlockSizeParameter, []StandardBlockSizeRow{{Bytes: session.Value{Null: true}}}, nil},
		{"denied falls back", session.Result{}, denied, good, nil, true, []string{blockSizeParameterSQL, blockSizeSystemSQL}, []StandardBlockSizeAttempt{{StandardBlockSizeParameter, "inaccessible"}, {StandardBlockSizeSystemTablespace, "available"}}, StandardBlockSizeSystemTablespace, []StandardBlockSizeRow{{Bytes: session.Value{Text: "8192"}}}, nil},
		{"empty doesn't hide a denial", session.Result{}, denied, empty, nil, true, []string{blockSizeParameterSQL, blockSizeSystemSQL}, []StandardBlockSizeAttempt{{StandardBlockSizeParameter, "inaccessible"}, {StandardBlockSizeSystemTablespace, "no_rows"}}, "", nil, denied},
		{"unavailable without opt-in", session.Result{}, unavailable, good, nil, false, []string{blockSizeParameterSQL}, []StandardBlockSizeAttempt{{StandardBlockSizeParameter, "unavailable"}}, "", nil, unavailable},
		{"mixed cleanup does not retry", session.Result{}, errors.Join(denied, cleanup), good, nil, true, []string{blockSizeParameterSQL}, []StandardBlockSizeAttempt{{StandardBlockSizeParameter, "failed"}}, "", nil, cleanup},
		{"failed fallback doesn't emit a result", null, nil, session.Result{}, cleanup, true, []string{blockSizeParameterSQL, blockSizeSystemSQL}, []StandardBlockSizeAttempt{{StandardBlockSizeParameter, "null"}, {StandardBlockSizeSystemTablespace, "failed"}}, "", nil, cleanup},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := StandardBlockSizeRequest{Options: StandardBlockSizeOptions{OracleVersion: "19.33.0.1.0"}, Reference: coreReference(t), AllowSystemTablespaceFallback: tc.allow}
			var seen []string
			reader := datasetTestReader(func(ctx context.Context, s session.Statement) (session.Result, error) {
				if ctx != t.Context() || s.Reference != req.Reference || len(s.Args) != 0 {
					t.Fatal("caller reference, context or statement arguments changed")
				}
				seen = append(seen, s.Text)
				if len(seen) == 1 {
					return tc.primary, tc.primaryErr
				}
				return tc.fallback, tc.fallbackErr
			})
			result, err := ReadStandardBlockSize(t.Context(), reader, req)
			if !errors.Is(err, tc.wantErr) || (err != nil && strings.Contains(err.Error(), "private diagnostics")) {
				t.Fatalf("read error %v, expected %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(seen, tc.wantSources) || !reflect.DeepEqual(result.Attempts, tc.wantAttempts) ||
				!reflect.DeepEqual(result.Rows, tc.wantRows) || result.Provenance.Source != string(tc.wantSource) ||
				result.Provenance.Complete != (tc.wantErr == nil) {
				t.Fatalf("unexpected reads or result: %v %+v", seen, result)
			}
		})
	}
}

func TestStandardBlockSizeRejectsMalformedBoundedOrCanceledResult(t *testing.T) {
	good := coreResult("BLOCK_SIZE_BYTES", session.Value{Text: "8192"})
	for _, value := range []string{"0", "-", "8.0", "1e3", " 8192", "18446744073709551616"} {
		result := coreResult("BLOCK_SIZE_BYTES", session.Value{Text: value})
		_, err := DecodeStandardBlockSize(result, StandardBlockSizeOptions{OracleVersion: "23.26.1.0.0"})
		if !errors.Is(err, ErrInvalidEvidence) {
			t.Fatalf("accepted invalid value %q", value)
		}
	}
	duplicate := session.Result{Columns: good.Columns, Rows: append(good.Rows, good.Rows[0])}
	for _, tc := range []struct {
		name string
		data session.Result
		lim  rowset.Limits
		want error
	}{
		{"duplicate", duplicate, rowset.Limits{}, ErrInvalidEvidence},
		{"budget", good, rowset.Limits{MaxRows: 1, MaxBytes: 3}, rowset.ErrCapacity},
		{"canceled", good, rowset.Limits{}, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			reader := datasetTestReader(func(context.Context, session.Statement) (session.Result, error) {
				calls++
				if tc.want == context.Canceled {
					cancel()
				}
				return tc.data, nil
			})
			result, err := ReadStandardBlockSize(ctx, reader, StandardBlockSizeRequest{
				Options: StandardBlockSizeOptions{OracleVersion: "19.33.0.1.0"}, Reference: coreReference(t),
				Limits: tc.lim, AllowSystemTablespaceFallback: true,
			})
			if !errors.Is(err, tc.want) || calls != 1 || result.Rows != nil || result.Provenance.Complete {
				t.Fatalf("failed read yielded %+v, %v, calls=%d", result, err, calls)
			}
		})
	}
}

func TestStandardBlockSizeJSONPreservesSourceAndCursorCompletion(t *testing.T) {
	options := StandardBlockSizeOptions{OracleVersion: "19.3.0.0.0", Source: StandardBlockSizeSystemTablespace}
	step := testsql.Step{SQL: blockSizeSystemSQL, Columns: []string{"BLOCK_SIZE_BYTES"}, Values: [][]driver.Value{{int64(8192)}}}
	db, state := testsql.New(t, step)
	payload, err := StandardBlockSizeJSON(t.Context(), db, options, rowset.Limits{})
	if err != nil || !strings.Contains(string(payload), "\"source\":\"DBA_TABLESPACES(SYSTEM)\"") ||
		!strings.Contains(string(payload), "\"rows\":[[8192]]") || state.Closed() != 1 {
		t.Fatalf("unexpected read: %s %v, closes=%d", payload, err, state.Closed())
	}
	late := errors.New("terminal cursor")
	step.NextErr = late
	db, state = testsql.New(t, step)
	payload, err = StandardBlockSizeJSON(t.Context(), db, options, rowset.Limits{})
	if payload != nil || !errors.Is(err, late) || state.Closed() != 1 {
		t.Fatalf("failed cursor emitted %s %v, closes=%d", payload, err, state.Closed())
	}
}
