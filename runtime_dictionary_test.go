package oracleinfo_test

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-info/objects"
	"github.com/marcuoli/go-oracle-info/performance"
	"github.com/marcuoli/go-oracle-info/scheduler"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

type runtimeDictionaryReader func(context.Context, session.Statement) (session.Result, error)

func (read runtimeDictionaryReader) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	return read(ctx, s)
}

type runtimeObservation struct {
	complete, hasRows bool
	numRows           int
}

func TestRuntimeDictionaryAdmissionAndCompletion(t *testing.T) {
	ref, err := reference.NewReference("go-oracle-info", "tests", "runtime-dictionary", 1)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		source      string
		floor       int
		baseColumns []string
		subname     bool
		unsupported error
		statement   func(string) (string, error)
		nativeJSON  func(rowset.Queryer, string, rowset.Limits) (json.RawMessage, error)
		read        func(context.Context, runtimeDictionaryReader, string, reference.Reference, rowset.Limits) (runtimeObservation, error)
	}{
		{source: "DBA_2PC_PENDING", floor: 9, baseColumns: []string{"LOCAL_TRAN_ID", "GLOBAL_TRAN_ID", "STATE", "MIXED", "ADVICE", "FAIL_TIME", "FORCE_TIME", "RETRY_TIME", "COMMIT_NUMBER"},
			unsupported: performance.ErrUnsupportedVersion,
			statement: func(v string) (string, error) {
				return performance.InDoubtTransactionsStatement(performance.InDoubtTransactionsOptions{OracleVersion: v})
			},
			nativeJSON: func(q rowset.Queryer, v string, lim rowset.Limits) (json.RawMessage, error) {
				return performance.InDoubtTransactionsJSON(t.Context(), q, performance.InDoubtTransactionsOptions{OracleVersion: v}, lim)
			},
			read: func(ctx context.Context, r runtimeDictionaryReader, v string, ref reference.Reference, lim rowset.Limits) (runtimeObservation, error) {
				f, err := performance.ReadInDoubtTransactions(ctx, r, performance.InDoubtTransactionsRequest{Options: performance.InDoubtTransactionsOptions{OracleVersion: v}, Reference: ref, Limits: lim})
				return runtimeObservation{f.Provenance.Complete, f.Rows != nil, len(f.Rows)}, err
			},
		},
		{source: "DBA_SCHEDULER_RUNNING_JOBS", floor: 10, baseColumns: []string{"OWNER", "JOB_NAME", "SESSION_ID", "RUNNING_INSTANCE"}, subname: true,
			unsupported: scheduler.ErrUnsupportedVersion,
			statement: func(v string) (string, error) {
				return scheduler.RunningJobsStatement(scheduler.RunningJobsOptions{OracleVersion: v})
			},
			nativeJSON: func(q rowset.Queryer, v string, lim rowset.Limits) (json.RawMessage, error) {
				return scheduler.RunningJobsJSON(t.Context(), q, scheduler.RunningJobsOptions{OracleVersion: v}, lim)
			},
			read: func(ctx context.Context, r runtimeDictionaryReader, v string, ref reference.Reference, lim rowset.Limits) (runtimeObservation, error) {
				f, err := scheduler.ReadRunningJobs(ctx, r, scheduler.RunningJobsRequest{Options: scheduler.RunningJobsOptions{OracleVersion: v}, Reference: ref, Limits: lim})
				return runtimeObservation{f.Provenance.Complete, f.Rows != nil, len(f.Rows)}, err
			},
		},
		{source: "DBA_MVIEWS", floor: 9, baseColumns: []string{"OWNER", "MVIEW_NAME", "STALENESS", "COMPILE_STATE"},
			unsupported: objects.ErrUnsupportedVersion,
			statement: func(v string) (string, error) {
				return objects.MaterializedViewsStatement(objects.MaterializedViewsOptions{OracleVersion: v})
			},
			nativeJSON: func(q rowset.Queryer, v string, lim rowset.Limits) (json.RawMessage, error) {
				return objects.MaterializedViewsJSON(t.Context(), q, objects.MaterializedViewsOptions{OracleVersion: v}, lim)
			},
			read: func(ctx context.Context, r runtimeDictionaryReader, v string, ref reference.Reference, lim rowset.Limits) (runtimeObservation, error) {
				f, err := objects.ReadMaterializedViews(ctx, r, objects.MaterializedViewsRequest{Options: objects.MaterializedViewsOptions{OracleVersion: v}, Reference: ref, Limits: lim})
				return runtimeObservation{f.Provenance.Complete, f.Rows != nil, len(f.Rows)}, err
			},
		},
		{source: "DBA_RECYCLEBIN", floor: 10, baseColumns: []string{"OWNER", "OBJECT_COUNT"},
			unsupported: objects.ErrUnsupportedVersion,
			statement: func(v string) (string, error) {
				return objects.RecycleBinCountsStatement(objects.RecycleBinCountsOptions{OracleVersion: v})
			},
			nativeJSON: func(q rowset.Queryer, v string, lim rowset.Limits) (json.RawMessage, error) {
				return objects.RecycleBinCountsJSON(t.Context(), q, objects.RecycleBinCountsOptions{OracleVersion: v}, lim)
			},
			read: func(ctx context.Context, r runtimeDictionaryReader, v string, ref reference.Reference, lim rowset.Limits) (runtimeObservation, error) {
				f, err := objects.ReadRecycleBinCounts(ctx, r, objects.RecycleBinCountsRequest{Options: objects.RecycleBinCountsOptions{OracleVersion: v}, Reference: ref, Limits: lim})
				return runtimeObservation{f.Provenance.Complete, f.Rows != nil, len(f.Rows)}, err
			},
		},
	}
	releases := []struct {
		v                  string
		major              int
		subname, container bool
	}{
		{"8.1.7.4.0", 0, false, false},
		{"9.0.1.4.0", 0, false, false},
		{"9.2.0.8.0", 9, false, false},
		{"10.1.0.5.0", 10, false, false},
		{"10.2.0.5.0", 10, true, false},
		{"11.1.0.7.0", 11, true, false},
		{"11.2.0.4.0", 11, true, false},
		{"12.1.0.1.0", 12, true, true},
		{"12.1.0.2.0", 12, true, true},
		{"12.2.0.1.0", 12, true, true},
		{"18.1.0.0.0", 18, true, true},
		{"19.33.0.0.0", 19, true, true},
		{"21.21.0.0.0", 21, true, true},
		{"23.9.0.0.0", 23, true, true},
		{"23.26.1.0.0", 23, true, true},
		{"23.26.4.0.0", 23, true, true},
		{"26.0", 0, false, false},
		{"26ai", 0, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			for _, release := range releases {
				t.Run(release.v, func(t *testing.T) {
					stmt, err := tc.statement(release.v)
					if release.major < tc.floor {
						db, state := testsql.New(t)
						blob, jsonErr := tc.nativeJSON(db, release.v, rowset.Limits{})
						read, readErr := tc.read(t.Context(), nil, release.v, ref, rowset.Limits{})
						if stmt != "" || !errors.Is(err, tc.unsupported) || !errors.Is(jsonErr, tc.unsupported) || !errors.Is(readErr, tc.unsupported) || blob != nil || read.hasRows || len(state.Calls()) != 0 {
							t.Fatalf("unsupported release executed or returned evidence: stmt=%q errors=%v/%v/%v", stmt, err, jsonErr, readErr)
						}
						return
					}
					if err != nil || !strings.Contains(stmt, " FROM SYS."+tc.source+" v") {
						t.Fatalf("selected statement: %q, %v", stmt, err)
					}
					expected := slices.Clone(tc.baseColumns)
					if tc.subname && release.subname {
						expected = slices.Insert(expected, 2, "JOB_SUBNAME")
					}
					if release.container {
						expected = append(expected, "SOURCE_CON_ID")
					}
					if strings.Contains(stmt, " AS SOURCE_CON_ID") != release.container || strings.Contains(stmt, " AS JOB_SUBNAME") != (tc.subname && release.subname) {
						t.Fatalf("selected the wrong optional columns: %s", stmt)
					}

					db, state := testsql.New(t, testsql.Step{SQL: stmt, Columns: expected})
					blob, err := tc.nativeJSON(db, release.v, rowset.Limits{})
					var decoded struct {
						Provenance struct {
							Complete      bool
							Source, Scope string
						}
						Data struct {
							Columns []rowset.Column
							Rows    [][]json.RawMessage
						}
					}
					if err != nil {
						t.Fatalf("empty JSON: %v", err)
					}
					if err := json.Unmarshal(blob, &decoded); err != nil {
						t.Fatal(err)
					}
					var names []string
					for _, c := range decoded.Data.Columns {
						names = append(names, c.Name)
					}
					if !decoded.Provenance.Complete || decoded.Provenance.Source != tc.source || decoded.Provenance.Scope != "connected_dictionary" ||
						!slices.Equal(expected, names) || decoded.Data.Rows == nil || len(decoded.Data.Rows) != 0 || state.Closed() != 1 || len(state.Calls()) != 1 {
						t.Fatalf("empty evidence or lifecycle: %s", blob)
					}

					calls := 0
					reader := runtimeDictionaryReader(func(_ context.Context, s session.Statement) (session.Result, error) {
						calls++
						if s.Text != stmt || s.Reference != ref || len(s.Args) != 0 {
							t.Fatal("buffered path did not forward the admitted statement and reference")
						}
						return session.Result{Columns: expected}, nil
					})
					observation, err := tc.read(t.Context(), reader, release.v, ref, rowset.Limits{})
					if err != nil || observation != (runtimeObservation{complete: true, hasRows: true}) || calls != 1 {
						t.Fatalf("empty buffered observation=%+v calls=%d err=%v", observation, calls, err)
					}
				})
			}
			const modern = "23.26.1.0.0"
			stmt, err := tc.statement(modern)
			if err != nil {
				t.Fatal(err)
			}
			expected := slices.Clone(tc.baseColumns)
			if tc.subname {
				expected = slices.Insert(expected, 2, "JOB_SUBNAME")
			}
			expected = append(expected, "SOURCE_CON_ID")
			denied := errors.New("ORA-00942")
			reader := runtimeDictionaryReader(func(context.Context, session.Statement) (session.Result, error) {
				return session.Result{Columns: expected}, denied
			})
			if f, err := tc.read(t.Context(), reader, modern, ref, rowset.Limits{}); f.hasRows || !errors.Is(err, denied) {
				t.Fatalf("access failure became a successful read: %+v, %v", f, err)
			}
			terminal := errors.New("row transport stopped")
			db, state := testsql.New(t, testsql.Step{SQL: stmt, Columns: expected, NextErr: terminal})
			if blob, err := tc.nativeJSON(db, modern, rowset.Limits{}); blob != nil || !errors.Is(err, terminal) || state.Closed() != 1 {
				t.Fatalf("terminal failure became evidence: %s, %v", blob, err)
			}
		})
	}
}

func TestRuntimeDictionaryTypedValuesAndBounds(t *testing.T) {
	const modern = "23.26.1.0.0"
	const exact = "987654321012345678901234567890"
	txt := func(value string) session.Value { return session.Value{Text: value} }
	null := session.Value{Null: true}

	txCols := []string{"LOCAL_TRAN_ID", "GLOBAL_TRAN_ID", "STATE", "MIXED", "ADVICE", "FAIL_TIME", "FORCE_TIME", "RETRY_TIME", "COMMIT_NUMBER", "SOURCE_CON_ID"}
	txRow := []session.Value{txt("1.2.3"), txt("remote.xid"), txt("forced commit"), txt("no"), null, txt("2026-11-24T03:04:05"), null, txt("2026-11-24T03:06:07"), txt("42.7"), txt("1")}
	txOptions := performance.InDoubtTransactionsOptions{OracleVersion: modern}
	tx, err := performance.DecodeInDoubtTransactions(session.Result{Columns: txCols, Rows: [][]session.Value{txRow}}, txOptions)
	if err != nil || tx.Provenance.Complete || len(tx.Rows) != 1 || tx.Rows[0].State.Text != "forced commit" || !tx.Rows[0].Advice.Null ||
		tx.Rows[0].FailTime.Text != "2026-11-24T03:04:05" || !tx.Rows[0].ForceTime.Null || tx.Rows[0].CommitNumber.Text != "42.7" {
		t.Fatalf("decoded transaction=%+v: %v", tx, err)
	}
	malformed := slices.Clone(txRow)
	malformed[5] = txt("2026-02-30T03:04:05")
	tx, err = performance.DecodeInDoubtTransactions(session.Result{Columns: txCols, Rows: [][]session.Value{txRow, malformed}}, txOptions)
	if tx.Rows != nil || !errors.Is(err, performance.ErrInvalidEvidence) {
		t.Fatalf("malformed late date leaked rows: %+v %v", tx, err)
	}

	jobCols := []string{"OWNER", "JOB_NAME", "JOB_SUBNAME", "SESSION_ID", "RUNNING_INSTANCE", "SOURCE_CON_ID"}
	jobRow := []session.Value{txt("App"), txt("J"), null, txt(exact), txt("7"), txt("3")}
	jobOptions := scheduler.RunningJobsOptions{OracleVersion: modern}
	jobs, err := scheduler.DecodeRunningJobs(session.Result{Columns: jobCols, Rows: [][]session.Value{jobRow, jobRow}}, jobOptions)
	if err != nil || len(jobs.Rows) != 2 || jobs.Rows[0].SessionID.Text != exact || jobs.Rows[0].RunningInstance.Text != "7" ||
		jobs.Rows[0].Subname == nil || !jobs.Rows[0].Subname.Null || jobs.Rows[0].SourceContainerID == nil || jobs.Rows[0].SourceContainerID.Text != "3" {
		t.Fatalf("modern running job observation=%+v: %v", jobs, err)
	}
	jobs, err = scheduler.DecodeRunningJobs(session.Result{
		Columns: []string{"OWNER", "JOB_NAME", "SESSION_ID", "RUNNING_INSTANCE"},
		Rows:    [][]session.Value{{txt("App"), txt("J"), txt(exact), null}},
	}, scheduler.RunningJobsOptions{OracleVersion: "10.1.0.5.0"})
	if err != nil || len(jobs.Rows) != 1 || jobs.Rows[0].Subname != nil || jobs.Rows[0].SourceContainerID != nil || !jobs.Rows[0].RunningInstance.Null {
		t.Fatalf("legacy running job observation=%+v: %v", jobs, err)
	}
	ref, err := reference.NewReference("go-oracle-info", "tests", "runtime-dictionary", 2)
	if err != nil {
		t.Fatal(err)
	}
	reader := runtimeDictionaryReader(func(context.Context, session.Statement) (session.Result, error) {
		return session.Result{Columns: jobCols, Rows: [][]session.Value{jobRow, jobRow}}, nil
	})
	if f, err := scheduler.ReadRunningJobs(t.Context(), reader, scheduler.RunningJobsRequest{Options: jobOptions, Reference: ref, Limits: rowset.Limits{MaxRows: 1}}); f.Rows != nil || !errors.Is(err, rowset.ErrCapacity) {
		t.Fatalf("over-budget reader leaked rows: %+v %v", f, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	canceledReader := runtimeDictionaryReader(func(context.Context, session.Statement) (session.Result, error) {
		cancel()
		return session.Result{Columns: jobCols, Rows: [][]session.Value{jobRow}}, nil
	})
	if f, err := scheduler.ReadRunningJobs(ctx, canceledReader, scheduler.RunningJobsRequest{Options: jobOptions, Reference: ref}); f.Rows != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reader leaked rows: %+v %v", f, err)
	}

	views, err := objects.DecodeMaterializedViews(session.Result{
		Columns: []string{"OWNER", "MVIEW_NAME", "STALENESS", "COMPILE_STATE", "SOURCE_CON_ID"},
		Rows:    [][]session.Value{{txt("CaseSensitive"), txt("M"), txt("IMPORT"), null, txt("3")}},
	}, objects.MaterializedViewsOptions{OracleVersion: modern})
	if err != nil || len(views.Rows) != 1 || views.Rows[0].Owner.Text != "CaseSensitive" || views.Rows[0].Staleness.Text != "IMPORT" || !views.Rows[0].CompileState.Null {
		t.Fatalf("materialized view observation=%+v: %v", views, err)
	}

	recycleOptions := objects.RecycleBinCountsOptions{OracleVersion: modern}
	recycleCols := []string{"OWNER", "OBJECT_COUNT", "SOURCE_CON_ID"}
	found, err := objects.DecodeRecycleBinCounts(session.Result{Columns: recycleCols, Rows: [][]session.Value{{txt("APP"), txt(exact), txt("1")}, {txt("app"), txt("2"), txt("1")}}}, recycleOptions)
	if err != nil || len(found.Rows) != 2 || found.Rows[0].Count.Text != exact || found.Rows[0].Owner.Text != "APP" || found.Rows[1].Owner.Text != "app" {
		t.Fatalf("recycle-bin counts=%+v: %v", found, err)
	}
	stmt, err := objects.RecycleBinCountsStatement(recycleOptions)
	if err != nil || !strings.Contains(stmt, "GROUP BY v.OWNER, NLSSORT(v.OWNER, 'NLS_SORT=BINARY')") {
		t.Fatalf("recycle-bin grouping: %q, %v", stmt, err)
	}
	db, state := testsql.New(t, testsql.Step{SQL: stmt, Columns: recycleCols, Values: [][]driver.Value{{"APP", "-1", int64(1)}}})
	if blob, err := objects.RecycleBinCountsJSON(t.Context(), db, recycleOptions, rowset.Limits{}); blob != nil || !errors.Is(err, objects.ErrInvalidEvidence) || state.Closed() != 1 {
		t.Fatalf("malformed count published evidence: %s %v", blob, err)
	}
}

func TestRuntimeDictionaryRejectsImpossibleEvidence(t *testing.T) {
	const version = "23.26.1.0.0"
	value := func(text string) session.Value { return session.Value{Text: text} }
	null := session.Value{Null: true}

	transactionColumns := []string{"LOCAL_TRAN_ID", "GLOBAL_TRAN_ID", "STATE", "MIXED", "ADVICE", "FAIL_TIME", "FORCE_TIME", "RETRY_TIME", "COMMIT_NUMBER", "SOURCE_CON_ID"}
	transaction := []session.Value{value("1.2.3"), null, value("prepared"), null, null,
		value("2026-11-24T03:04:05"), null, value("2026-11-24T03:05:06"), null, value("1")}
	for _, position := range []int{0, 2, 5, 7} {
		t.Run(transactionColumns[position], func(t *testing.T) {
			late := slices.Clone(transaction)
			late[position] = null
			result, err := performance.DecodeInDoubtTransactions(
				session.Result{Columns: transactionColumns, Rows: [][]session.Value{transaction, late}},
				performance.InDoubtTransactionsOptions{OracleVersion: version})
			if result.Rows != nil || !errors.Is(err, performance.ErrInvalidEvidence) {
				t.Fatalf("impossible required transaction value returned evidence: %+v %v", result, err)
			}
		})
	}
	views, err := objects.DecodeMaterializedViews(session.Result{
		Columns: []string{"OWNER", "MVIEW_NAME", "STALENESS", "COMPILE_STATE", "SOURCE_CON_ID"},
		Rows:    [][]session.Value{{value("APP"), null, value("STALE"), null, value("1")}},
	}, objects.MaterializedViewsOptions{OracleVersion: version})
	if views.Rows != nil || !errors.Is(err, objects.ErrInvalidEvidence) {
		t.Errorf("nameless materialized view returned evidence: %+v %v", views, err)
	}
	for _, tc := range []struct {
		name string
		row  []session.Value
	}{
		{"owner-null", []session.Value{null, value("5"), value("1")}},
		{"zero-count", []session.Value{value("OTHER"), value("000"), value("1")}},
		{"duplicate-owner", []session.Value{value("APP"), value("25"), value("1")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := objects.DecodeRecycleBinCounts(session.Result{
				Columns: []string{"OWNER", "OBJECT_COUNT", "SOURCE_CON_ID"},
				Rows:    [][]session.Value{{value("APP"), value("20"), value("1")}, tc.row},
			}, objects.RecycleBinCountsOptions{OracleVersion: version})
			if result.Rows != nil || !errors.Is(err, objects.ErrInvalidEvidence) {
				t.Fatalf("impossible grouped count returned evidence: %+v %v", result, err)
			}
		})
	}
}
