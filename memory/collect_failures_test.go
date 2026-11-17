package memory_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/marcuoli/go-oracle-info/memory"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestCollectorsPreserveIndependentNoRowFailures(t *testing.T) {
	terminal := errors.New("terminal failure")
	for _, tc := range []struct {
		name    string
		failure error
		want    error
	}{
		{"no rows", sql.ErrNoRows, nil},
		{"wrapped", fmt.Errorf("adapter: %w", sql.ErrNoRows), nil},
		{"duplicate no rows", errors.Join(sql.ErrNoRows, fmt.Errorf("adapter: %w", sql.ErrNoRows)), nil},
		{"joined terminal", errors.Join(sql.ErrNoRows, terminal), terminal},
		{"nested terminal", fmt.Errorf("adapter: %w", errors.Join(fmt.Errorf("empty: %w", sql.ErrNoRows), terminal)), terminal},
		{"joined cancellation", errors.Join(sql.ErrNoRows, context.Canceled), context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := readerFunc(func(context.Context, session.Statement) (session.Result, error) { return session.Result{}, tc.failure })
			t.Run("primary", func(t *testing.T) {
				got, err := memory.Collect(t.Context(), reader, memory.Request{Parameter: memory.SGA, Options: memory.Options{Scope: memory.CDBRoot, OracleMajor: 19}, PrimaryReference: testRef(t, 1)})
				if tc.want != nil {
					if !errors.Is(err, tc.want) || got.State == memory.Available || got.State == memory.NoRows {
						t.Fatalf("failure hidden: %+v %v", got, err)
					}
				} else if err != nil || got.State != memory.NoRows {
					t.Fatalf("no-row compatibility lost: %+v %v", got, err)
				}
			})
			t.Run("RAC", func(t *testing.T) {
				got, err := memory.CollectRAC(t.Context(), reader, memory.Options{Scope: memory.CDBRoot, OracleMajor: 19}, testRef(t, 1))
				if tc.want != nil {
					if !errors.Is(err, tc.want) || got.SGA.State != memory.Unavailable || got.PGA.State != memory.Unavailable {
						t.Fatalf("failure hidden: %+v %v", got, err)
					}
				} else if err != nil || got.SGA.State != memory.NoRows || got.PGA.State != memory.NoRows {
					t.Fatalf("no-row compatibility lost: %+v %v", got, err)
				}
			})
			t.Run("effective", func(t *testing.T) {
				primary := sample("", "3", "sga_target")
				primary.Rows[0][0].Null = true
				r := &sequenceReader{results: []session.Result{primary, {}}, failures: []error{nil, tc.failure}}
				got, err := memory.Collect(t.Context(), r, memory.Request{Parameter: memory.SGA, Options: memory.Options{Scope: memory.ConnectedPDB, OracleMajor: 19}, PrimaryReference: testRef(t, 1), EffectiveReference: testRef(t, 2)})
				if got.State != memory.Null || got.SourceContainerID != 3 || got.InstanceID != 2 {
					t.Fatalf("valid primary lost: %+v %v", got, err)
				}
				if tc.want != nil {
					if !errors.Is(err, tc.want) {
						t.Fatalf("effective failure hidden: %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestCollectorsRejectCancellationAfterCompleteQuery(t *testing.T) {
	t.Run("known primary", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		reader := readerFunc(func(context.Context, session.Statement) (session.Result, error) {
			cancel()
			return sample("2", "3", "sga_target"), nil
		})
		got, err := memory.Collect(ctx, reader, memory.Request{Parameter: memory.SGA, Options: memory.Options{Scope: memory.ConnectedPDB, OracleMajor: 19}, PrimaryReference: testRef(t, 1), EffectiveReference: testRef(t, 2)})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled query succeeded: %+v %v", got, err)
		}
	})
	t.Run("effective", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		calls := 0
		reader := readerFunc(func(context.Context, session.Statement) (session.Result, error) {
			calls++
			if calls == 1 {
				r := sample("", "3", "sga_target")
				r.Rows[0][0].Null = true
				return r, nil
			}
			cancel()
			return sample("7.813", "3", "sga_target"), nil
		})
		got, err := memory.Collect(ctx, reader, memory.Request{Parameter: memory.SGA, Options: memory.Options{Scope: memory.ConnectedPDB, OracleMajor: 19}, PrimaryReference: testRef(t, 1), EffectiveReference: testRef(t, 2)})
		if !errors.Is(err, context.Canceled) || calls != 2 || got.State != memory.Null || got.SourceContainerID != 3 || got.InstanceID != 2 {
			t.Fatalf("cancelled replacement or lost primary: %+v %v calls=%d", got, err, calls)
		}
	})
	t.Run("RAC", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		reader := readerFunc(func(context.Context, session.Statement) (session.Result, error) {
			cancel()
			return racResult(racRow("sga_max_size", "1073741824", "1", "0", "1"), racRow("pga_aggregate_target", "0", "1", "0", "1")), nil
		})
		got, err := memory.CollectRAC(ctx, reader, memory.Options{Scope: memory.CDBRoot, OracleMajor: 19}, testRef(t, 1))
		if !errors.Is(err, context.Canceled) || got.SGA.State != memory.Unavailable || got.PGA.State != memory.Unavailable {
			t.Fatalf("cancelled RAC succeeded: %+v %v", got, err)
		}
	})
}
