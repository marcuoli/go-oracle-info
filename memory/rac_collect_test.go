package memory_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/marcuoli/go-oracle-info/memory"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestRACCollectionBindsOnlyTheAdmittedTarget(t *testing.T) {
	reader := &sequenceReader{results: []session.Result{racResult(
		racRow("sga_target", "1073741824", "1", "4", "4"),
		racRow("pga_aggregate_target", "0", "1", "4", "4"),
	)}}
	ref := testRef(t, 4)
	got, err := memory.ReadRACWithOptions(t.Context(), reader, memory.RACRequest{
		Options:   memory.Options{Scope: memory.DiscoveredPDB, OracleMajor: 12, TargetContainerID: 4},
		Reference: ref,
	})
	if err != nil || got.SGA.ValueGiB != "1" || got.PGA.ValueGiB != "0" || len(reader.statements) != 1 {
		t.Fatalf("%+v calls=%d %v", got, len(reader.statements), err)
	}
	if reader.statements[0].Reference != ref {
		t.Fatalf("caller reference changed: %v", reader.statements[0].Reference)
	}
	args := reader.statements[0].Args
	if len(args) != 1 || args[0] != sql.Named("target_container_id", 4) {
		t.Fatalf("unexpected target binds: %#v", args)
	}
}

func TestRACCollectionKeepsAbsenceCancellationAndFailureDistinct(t *testing.T) {
	failure := errors.New("driver message must remain an inspectable cause")
	for _, tc := range []struct {
		name  string
		err   error
		state memory.State
	}{
		{"no row", sql.ErrNoRows, memory.NoRows},
		{"failure", failure, memory.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &sequenceReader{failures: []error{tc.err}}
			got, err := memory.CollectRAC(context.Background(), reader, memory.Options{Scope: memory.CDBRoot, OracleMajor: 19}, testRef(t, 1))
			if got.SGA.State != tc.state || got.PGA.State != tc.state || (tc.err == failure && !errors.Is(err, failure)) || (tc.err == sql.ErrNoRows && err != nil) || len(reader.statements) != 1 {
				t.Fatalf("%+v calls=%d %v", got, len(reader.statements), err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &sequenceReader{}
	got, err := memory.CollectRAC(ctx, reader, memory.Options{Scope: memory.CDBRoot, OracleMajor: 19}, testRef(t, 1))
	if !errors.Is(err, context.Canceled) || len(reader.statements) != 0 || got.SGA.State != memory.Unavailable {
		t.Fatalf("canceled read executed or became absent: %+v %v", got, err)
	}
}

func TestRACOverflowAndInvalidIdentityNeverBecomePartialTotals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result session.Result
	}{
		{"overflow", func() session.Result {
			r := racResult()
			for i := 0; i < 4097; i++ {
				r.Rows = append(r.Rows, racRow("sga_max_size", "1073741824", "1", "0", "1"))
			}
			return r
		}()},
		{"wrong container", racResult(racRow("sga_max_size", "1073741824", "1", "0", "3"))},
		{"missing instance", racResult(racRow("sga_max_size", "1073741824", "", "0", "1"))},
		{"malformed row", racResult([]session.Value{{Text: "sga_max_size"}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := memory.DecodeRAC(tc.result, memory.Options{Scope: memory.CDBRoot, OracleMajor: 19})
			if !errors.Is(err, memory.ErrInvalidEvidence) || got.SGA.State != memory.Unavailable || got.PGA.State != memory.Unavailable || got.SGA.ValueGiB != "" || got.PGA.ValueGiB != "" || len(got.Members) != 0 {
				t.Fatalf("invalid source became a prefix: %+v %v", got, err)
			}
		})
	}
}
