package memory_test

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/memory"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

func TestScopedSelectionsPreserveVersionAndContainerBoundaries(t *testing.T) {
	for _, tc := range []struct {
		options memory.Options
		want    string
	}{
		{memory.Options{Scope: memory.NonCDB, OracleMajor: 9}, "0 AS SOURCE_CONTAINER_ID"},
		{memory.Options{Scope: memory.CDBRoot, OracleMajor: 12}, "S.CON_ID IN (0,1)"},
		{memory.Options{Scope: memory.ConnectedPDB, OracleMajor: 19, PDB: memory.PDBOptions{AllowInherited: true}}, "SOURCE_ROWS<>1"},
		{memory.Options{Scope: memory.DiscoveredPDB, OracleMajor: 12, TargetContainerID: 4}, "SYS.GV_$SYSTEM_PARAMETER"},
	} {
		sql, err := memory.Statement(memory.SGA, tc.options)
		if err != nil || !strings.Contains(sql, tc.want) {
			t.Fatalf("%+v: %v\n%s", tc.options, err, sql)
		}
	}
	for _, options := range []memory.Options{
		{Scope: memory.NonCDB, OracleMajor: 8},
		{Scope: memory.CDBRoot, OracleMajor: 11},
		{Scope: memory.DiscoveredPDB, OracleMajor: 19, TargetContainerID: 1},
		{Scope: memory.ConnectedPDB, OracleMajor: 12, PDB: memory.PDBOptions{EffectiveADB: true}},
	} {
		if _, err := memory.Statement(memory.SGA, options); err == nil {
			t.Fatalf("invalid scope accepted: %+v", options)
		}
	}
}

func TestDuplicateZeroRowsCannotFallThroughToAnInheritedSetting(t *testing.T) {
	sql, err := memory.PDBStatement(memory.SGA, memory.PDBOptions{AllowInherited: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(sql, "SOURCE_ROWS<>1") < 2 {
		t.Fatal("duplicate child evidence can be ranked/filtered away before decoding")
	}
}

type sequenceReader struct {
	results    []session.Result
	failures   []error
	statements []session.Statement
}

func (r *sequenceReader) Query(_ context.Context, s session.Statement) (session.Result, error) {
	i := len(r.statements)
	r.statements = append(r.statements, s)
	var err error
	if i < len(r.failures) {
		err = r.failures[i]
	}
	if i >= len(r.results) {
		return session.Result{}, err
	}
	return r.results[i], err
}
func testRef(t *testing.T, number int) reference.Reference {
	t.Helper()
	r, err := reference.NewReference("example", "facts", "configured-memory", uint32(number))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCollectionFallbackPreservesErrorsAndOnlyRetriesMissingPrimary(t *testing.T) {
	failure := errors.New("primary failed")
	for _, tc := range []struct {
		name      string
		primary   session.Result
		err       error
		wantCalls int
		wantErr   bool
	}{
		{"known", sample("2", "3", "sga_target"), nil, 1, false},
		{"no rows", session.Result{Columns: sample("2", "3", "sga_target").Columns}, nil, 2, false},
		{"NULL", func() session.Result { r := sample("", "3", "sga_target"); r.Rows[0][0].Null = true; return r }(), nil, 2, false},
		{"failure", session.Result{}, failure, 1, true},
		{"no-row error", session.Result{}, sql.ErrNoRows, 2, false},
		{"invalid", sample("-1", "3", "sga_target"), nil, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &sequenceReader{results: []session.Result{tc.primary, sample("7.813", "3", "sga_target")}, failures: []error{tc.err}}
			got, err := memory.Collect(context.Background(), reader, memory.Request{
				Parameter: memory.SGA, Options: memory.Options{Scope: memory.ConnectedPDB, OracleMajor: 19, PDB: memory.PDBOptions{AllowInherited: true}},
				PrimaryReference: testRef(t, 1), EffectiveReference: testRef(t, 2),
			})
			if (err != nil) != tc.wantErr || len(reader.statements) != tc.wantCalls {
				t.Fatalf("calls=%d err=%v", len(reader.statements), err)
			}
			if err == nil && tc.wantCalls == 2 {
				if got.State != memory.Available || got.ValueGiB != "7.813" || got.Origin != memory.EffectiveADB || got.InstanceID != 2 || got.SourceContainerID != 3 || got.CurrentContainerID != 3 || got.TargetContainerID != 3 {
					t.Fatalf("usable effective replacement lost value or provenance: %+v", got)
				}
			}
			if errors.Is(tc.err, failure) && !errors.Is(err, failure) {
				t.Fatal("primary cause lost")
			}
		})
	}
}

func TestFallbackWithoutUsableReplacementRetainsPrimaryProvenance(t *testing.T) {
	primary := sample("", "3", "sga_target")
	primary.Rows[0][0] = session.Value{Null: true}
	options := memory.Options{Scope: memory.ConnectedPDB, OracleMajor: 19}
	want, err := memory.Decode(primary, memory.SGA, options)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("effective query failed")
	for _, tc := range []struct {
		name      string
		result    session.Result
		failure   error
		wantError error
	}{
		{"no rows", session.Result{Columns: primary.Columns}, nil, nil},
		{"no-row error", session.Result{}, sql.ErrNoRows, nil},
		{"NULL", primary, nil, nil},
		{"invalid", sample("-1", "3", "sga_target"), nil, memory.ErrInvalidEvidence},
		{"query failure", session.Result{}, failure, failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &sequenceReader{results: []session.Result{primary, tc.result}, failures: []error{nil, tc.failure}}
			got, err := memory.Collect(context.Background(), reader, memory.Request{
				Parameter: memory.SGA, Options: options,
				PrimaryReference: testRef(t, 1), EffectiveReference: testRef(t, 2),
			})
			if !errors.Is(err, tc.wantError) || got != want || len(reader.statements) != 2 {
				t.Fatalf("got=%+v want=%+v calls=%d error=%v", got, want, len(reader.statements), err)
			}
		})
	}
}

func TestScalarScopeKeepsExecutionAndTargetContainersSeparate(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		options                            memory.Options
		current, target, source, parameter string
		wantError                          bool
	}{
		{"root", memory.Options{Scope: memory.CDBRoot, OracleMajor: 19}, "1", "1", "0", "sga_max_size", false},
		{"non-CDB", memory.Options{Scope: memory.NonCDB, OracleMajor: 9}, "0", "0", "0", "sga_max_size", false},
		{"discovered", memory.Options{Scope: memory.DiscoveredPDB, OracleMajor: 12, TargetContainerID: 4}, "1", "4", "4", "sga_target", false},
		{"wrong discovered target", memory.Options{Scope: memory.DiscoveredPDB, OracleMajor: 12, TargetContainerID: 4}, "1", "3", "3", "sga_target", true},
		{"discovered executed in PDB", memory.Options{Scope: memory.DiscoveredPDB, OracleMajor: 12, TargetContainerID: 4}, "4", "4", "4", "sga_target", true},
		{"direct executed in root", memory.Options{Scope: memory.ConnectedPDB, OracleMajor: 19}, "1", "4", "4", "sga_target", true},
		{"root wrong target", memory.Options{Scope: memory.CDBRoot, OracleMajor: 19}, "1", "4", "0", "sga_max_size", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := sample("2", tc.source, tc.parameter)
			r.Rows[0][2].Text = tc.current
			r.Rows[0][6].Text = tc.target
			got, err := memory.Decode(r, memory.SGA, tc.options)
			if (err != nil) != tc.wantError {
				t.Fatalf("%+v %v", got, err)
			}
			if !tc.wantError && (strconv.Itoa(got.CurrentContainerID) != tc.current || strconv.Itoa(got.TargetContainerID) != tc.target || strconv.Itoa(got.SourceContainerID) != tc.source) {
				t.Fatalf("provenance lost: %+v", got)
			}
		})
	}
	query, err := memory.Statement(memory.SGA, memory.Options{Scope: memory.DiscoveredPDB, OracleMajor: 12, TargetContainerID: 4})
	if err != nil || !strings.Contains(query, "TO_CHAR(TO_NUMBER(SYS_CONTEXT('USERENV','CON_ID')), 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS CURRENT_CONTAINER_ID") || !strings.Contains(query, "TO_CHAR(:target_container_id, 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''') AS TARGET_CONTAINER_ID") {
		t.Fatalf("target substituted for execution identity: %s %v", query, err)
	}
}

type readerFunc func(context.Context, session.Statement) (session.Result, error)

func (f readerFunc) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	return f(ctx, s)
}

func TestCollectionCancellationDoesNotStartFallbackOrErasePrimary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	primary := sample("", "3", "sga_target")
	primary.Rows[0][0].Null = true
	want, err := memory.DecodePDB(primary, memory.SGA, memory.PDBOptions{})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	reader := readerFunc(func(got context.Context, _ session.Statement) (session.Result, error) {
		calls++
		if got != ctx {
			t.Fatal("context was replaced")
		}
		cancel()
		return primary, nil
	})
	got, err := memory.Collect(ctx, reader, memory.Request{Parameter: memory.SGA, Options: memory.Options{Scope: memory.ConnectedPDB, OracleMajor: 19}, PrimaryReference: testRef(t, 1), EffectiveReference: testRef(t, 2)})
	if !errors.Is(err, context.Canceled) || calls != 1 || got != want {
		t.Fatalf("calls=%d got=%+v err=%v", calls, got, err)
	}
	_, err = memory.Collect(ctx, reader, memory.Request{})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("canceled collection queried source: %d %v", calls, err)
	}
}
