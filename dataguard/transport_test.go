package dataguard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/testsql"
	"github.com/marcuoli/go-oracle-session/rowset"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

type readerFunc func(context.Context, session.Statement) (session.Result, error)

func (f readerFunc) Query(ctx context.Context, s session.Statement) (session.Result, error) {
	return f(ctx, s)
}
func TestCompleteReaderAndJSONCompanions(t *testing.T) {
	ref, err := reference.NewReference("test", "dataguard", "read", 1)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{OracleVersion: "19.32.0.0.0", Reference: ref}
	progress := ArchiveProgressOptions{ResetlogsChange: "18446744073709551616", ResetlogsTime: "2026-11-18T00:00:00"}
	cases := []struct {
		read func(context.Context, Reader) (Provenance, int, error)
		json func(context.Context, rowset.Queryer, rowset.Limits) (json.RawMessage, error)
	}{
		{func(c context.Context, r Reader) (Provenance, int, error) {
			f, e := ReadDatabaseContext(c, r, DatabaseContextRequest{OracleVersion: request.OracleVersion, Reference: ref})
			return f.Provenance, len(f.Rows), e
		}, func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
			return DatabaseContextJSON(c, q, request.OracleVersion, l)
		}},
		{func(c context.Context, r Reader) (Provenance, int, error) {
			f, e := ReadStandbyLogs(c, r, request)
			return f.Provenance, len(f.Rows), e
		}, func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
			return StandbyLogsJSON(c, q, request.OracleVersion, Filter{}, l)
		}},
		{func(c context.Context, r Reader) (Provenance, int, error) {
			f, e := ReadProcesses(c, r, request)
			return f.Provenance, len(f.Rows), e
		}, func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
			return ProcessesJSON(c, q, request.OracleVersion, Filter{}, l)
		}},
		{func(c context.Context, r Reader) (Provenance, int, error) {
			f, e := ReadRecoveryProgress(c, r, request)
			return f.Provenance, len(f.Rows), e
		}, func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
			return RecoveryProgressJSON(c, q, request.OracleVersion, Filter{}, l)
		}},
		{func(c context.Context, r Reader) (Provenance, int, error) {
			f, e := ReadArchiveGaps(c, r, request)
			return f.Provenance, len(f.Rows), e
		}, func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
			return ArchiveGapsJSON(c, q, request.OracleVersion, Filter{}, l)
		}},
		{func(c context.Context, r Reader) (Provenance, int, error) {
			f, e := ReadArchiveDestinations(c, r, request)
			return f.Provenance, len(f.Rows), e
		}, func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
			return ArchiveDestinationsJSON(c, q, request.OracleVersion, Filter{}, l)
		}},
		{func(c context.Context, r Reader) (Provenance, int, error) {
			f, e := ReadArchiveDestinationStatus(c, r, request)
			return f.Provenance, len(f.Rows), e
		}, func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
			return ArchiveDestinationStatusJSON(c, q, request.OracleVersion, Filter{}, l)
		}},
		{func(c context.Context, r Reader) (Provenance, int, error) {
			f, e := ReadArchiveProgress(c, r, ArchiveProgressRequest{OracleVersion: request.OracleVersion, Reference: ref, Filter: progress})
			return f.Provenance, len(f.Rows), e
		}, func(c context.Context, q rowset.Queryer, l rowset.Limits) (json.RawMessage, error) {
			return ArchiveProgressJSON(c, q, request.OracleVersion, progress, l)
		}},
	}
	for i, p := range allPlans(t) {
		t.Run(p.provenance.Source, func(t *testing.T) {
			tc := cases[i]
			source := validSource(p)
			reader := readerFunc(func(ctx context.Context, s session.Statement) (session.Result, error) {
				if s.Text != p.sql || s.Reference != ref {
					t.Fatalf("different admitted SQL/ref: %s", s.Text)
				}
				return source, nil
			})
			provenance, n, err := tc.read(t.Context(), reader)
			if err != nil || !provenance.Complete || n != 1 || provenance.Source != p.provenance.Source {
				t.Fatalf("lost complete typed read: %+v %d %v", provenance, n, err)
			}
			cause := io.ErrUnexpectedEOF
			provenance, n, err = tc.read(t.Context(), readerFunc(func(context.Context, session.Statement) (session.Result, error) { return source, cause }))
			if !errors.Is(err, cause) || provenance.Complete || n != 0 {
				t.Fatalf("published failed query prefix: %+v %d %v", provenance, n, err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			provenance, n, err = tc.read(ctx, readerFunc(func(context.Context, session.Statement) (session.Result, error) { cancel(); return source, nil }))
			if !errors.Is(err, context.Canceled) || provenance.Complete || n != 0 {
				t.Fatalf("published canceled read: %+v %d %v", provenance, n, err)
			}
			for _, failure := range []string{"valid", "late", "close", "cancel", "rows", "bytes", "invalid"} {
				t.Run(failure, func(t *testing.T) {
					step := nativeStep(p, source)
					limits := rowset.Limits{MaxRows: 1, MaxBytes: 65536}
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					switch failure {
					case "late":
						step.NextErr = cause
					case "close":
						step.CloseErr = cause
					case "cancel":
						step.OnNext = func(int) { cancel() }
					case "rows":
						step.Values = append(step.Values, step.Values[0])
					case "bytes":
						limits.MaxBytes = 1
					case "invalid":
						for i, name := range step.Columns {
							if name == "CON_ID" {
								step.Values[0][i] = "not-a-number"
							}
						}
					}
					db, state := testsql.New(t, step)
					payload, err := tc.json(ctx, db, limits)
					if failure == "valid" {
						if err != nil || !json.Valid(payload) || state.Closed() != 1 {
							t.Fatalf("valid companion rejected: %s %v", payload, err)
						}
						return
					}
					if err == nil || payload != nil {
						t.Fatalf("published %s prefix: %s %v", failure, payload, err)
					}
					if (failure == "late" || failure == "close") && !errors.Is(err, cause) {
						t.Fatalf("lost cause %v", err)
					}
				})
			}
		})
	}
}
func TestTypedFieldsPreserveExactNumbersNullAndSourceSemantics(t *testing.T) {
	p, _ := standbyLogsPlan("23.26.1.0.0", Filter{Instances: []int64{7}})
	r := validSource(p)
	for i, c := range r.Columns {
		switch c {
		case "DBID":
			r.Rows[0][i] = v("UNASSIGNED")
		case "STATUS":
			r.Rows[0][i] = v("ACTIVE")
		case "ARCHIVED":
			r.Rows[0][i] = v("YES")
		case "FIRST_CHANGE":
			r.Rows[0][i] = v("18446744073709551616")
		case "GROUP_NUMBER":
			r.Rows[0][i] = v("2")
		}
	}
	f, err := DecodeStandbyLogs(r, "23.26.1.0.0", Filter{Instances: []int64{7}})
	if err != nil || f.Provenance.Complete || len(f.Rows) != 1 || f.Rows[0].DBID.Text != "UNASSIGNED" || f.Rows[0].FirstChange.Text != "18446744073709551616" || f.Rows[0].GroupNumber.Text != "2" || !f.Rows[0].LastTime.Null || f.Rows[0].Archived.Text != "YES" {
		t.Fatalf("changed raw SRL source: %+v %v", f, err)
	}
	if _, err = DecodeStandbyLogs(r, "23.26.1.0.0", Filter{Instances: []int64{2}}); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("other reporter admitted: %v", err)
	}
	p, _ = recoveryProgressPlan("19.32.0.0.0", Filter{})
	r = validSource(p)
	for i, c := range r.Columns {
		if c == "SOFAR" {
			r.Rows[0][i] = v("0.125")
		}
		if c == "TOTAL" {
			r.Rows[0][i] = v("1.234567E+20")
		}
	}
	recovery, err := DecodeRecoveryProgress(r, "19.32.0.0.0", Filter{})
	if err != nil || recovery.Rows[0].Sofar.Text != "0.125" || recovery.Rows[0].Total.Text != "1.234567E+20" {
		t.Fatalf("lost numeric meaning: %+v %v", recovery, err)
	}
}
