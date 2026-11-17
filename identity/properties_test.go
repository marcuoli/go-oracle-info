package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"strings"
	"testing"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/session"
	"github.com/marcuoli/go-sqltemplate/reference"
)

const legacyVersionSQL = "SELECT VERSION AS VERSION FROM V$INSTANCE"
const fullVersionSQL = "SELECT VERSION_FULL AS VERSION FROM V$INSTANCE"
const editionCodeSQL = "SELECT EDITION AS EDITION FROM V$INSTANCE"
const legacyBannersSQL = "SELECT BANNER AS BANNER FROM V$VERSION ORDER BY BANNER"

type propertyStep struct {
	ref       reference.Reference
	statement string
	result    session.Result
	err       error
	after     func()
}
type propertyReader struct {
	t     *testing.T
	ctx   context.Context
	steps []propertyStep
	calls []session.Statement
}

func (r *propertyReader) Query(ctx context.Context, statement session.Statement) (session.Result, error) {
	r.t.Helper()
	index := len(r.calls)
	r.calls = append(r.calls, statement)
	if ctx != r.ctx || index >= len(r.steps) {
		r.t.Fatal("unexpected read or replaced context", index, statement.Text)
	}
	step := r.steps[index]
	if statement.Reference != step.ref || statement.Text != step.statement || len(statement.Args) != 0 {
		r.t.Fatalf("source/reference contract changed: %#v, want %#v", statement, step)
	}
	if step.after != nil {
		step.after()
	}
	return step.result, step.err
}
func (r *propertyReader) done() {
	r.t.Helper()
	if len(r.calls) != len(r.steps) {
		r.t.Fatalf("calls=%d want=%d", len(r.calls), len(r.steps))
	}
}
func propertyRef(t *testing.T, number uint32) reference.Reference {
	t.Helper()
	ref, err := reference.NewReference("test", "identity", "properties", number)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
func versionRefs(t *testing.T) identity.VersionReferences {
	return identity.VersionReferences{Legacy: propertyRef(t, 1), Full: propertyRef(t, 2)}
}
func architectureRefs(t *testing.T) identity.ArchitectureReferences {
	return identity.ArchitectureReferences{Version: propertyRef(t, 1), Container: propertyRef(t, 2)}
}
func editionRefs(t *testing.T) identity.EditionReferences {
	return identity.EditionReferences{Version: propertyRef(t, 1), Code: propertyRef(t, 2), Banners: propertyRef(t, 3)}
}
func scalarFact(column string, value identity.Text) session.Result {
	result := session.Result{Columns: []string{column}}
	if value.State == identity.NoRows {
		return result
	}
	result.Rows = [][]session.Value{{{Text: value.Value, Null: value.State == identity.Null}}}
	return result
}
func available(value string) identity.Text {
	return identity.Text{State: identity.Available, Value: value}
}
func bootstrapStep(ref reference.Reference, value identity.Text) propertyStep {
	return propertyStep{ref: ref, statement: legacyVersionSQL, result: scalarFact("VERSION", value)}
}
func checkAttempts(t *testing.T, got []identity.PropertyAttempt, sources []identity.PropertySource, states []identity.State) {
	t.Helper()
	if len(got) != len(sources) {
		t.Fatalf("attempts=%#v want sources=%v", got, sources)
	}
	for i := range sources {
		if got[i].Source != sources[i] || got[i].State != states[i] {
			t.Fatalf("attempt[%d]=%#v", i, got[i])
		}
	}
}

type propertyCall func(context.Context, identity.Reader) (identity.PropertyEvidence, error)

func propertyCalls(t *testing.T) map[string]propertyCall {
	t.Helper()
	return map[string]propertyCall{
		"version": func(ctx context.Context, r identity.Reader) (identity.PropertyEvidence, error) {
			v, e := identity.ReadVersion(ctx, r, versionRefs(t))
			return v.PropertyEvidence, e
		},
		"architecture": func(ctx context.Context, r identity.Reader) (identity.PropertyEvidence, error) {
			v, e := identity.ReadArchitecture(ctx, r, architectureRefs(t))
			return v.PropertyEvidence, e
		},
		"edition": func(ctx context.Context, r identity.Reader) (identity.PropertyEvidence, error) {
			v, e := identity.ReadEdition(ctx, r, editionRefs(t))
			return v.PropertyEvidence, e
		},
	}
}
func TestPropertyFailuresRetainCauseWithoutEchoingDriverText(t *testing.T) {
	t.Parallel()
	secret := errors.New("driver password=private-sentinel host=private-host")
	cause := errors.Join(secret, sql.ErrNoRows, errors.New("independent terminal failure"))
	for name, call := range propertyCalls(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{{ref: propertyRef(t, 1), statement: legacyVersionSQL, err: cause}}}
			got, err := call(ctx, reader)
			reader.done()
			if got.State != identity.Unavailable || !errors.Is(err, secret) || !errors.Is(err, sql.ErrNoRows) || len(got.Attempts) != 1 || !errors.Is(got.Attempts[0].Err, secret) {
				t.Fatalf("failure provenance lost: %#v %v", got, err)
			}
			var failure *identity.PropertyError
			if !errors.As(err, &failure) || failure.Source != identity.InstanceVersionSource || failure.Operation != "query" {
				t.Fatal("source operation lost")
			}
			encoded, marshalErr := json.Marshal(got)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if strings.Contains(err.Error(), "private-") || strings.Contains(string(encoded), "private-") || strings.Contains(got.Attempts[0].Err.Error(), "private-") {
				t.Fatal("driver details echoed")
			}
		})
	}
}
func TestPropertyCancellationRejectsPayloadAndStopsSequence(t *testing.T) {
	t.Parallel()
	for name, call := range propertyCalls(t) {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			step := bootstrapStep(propertyRef(t, 1), available("19.0.0.0.0"))
			step.after = cancel
			reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{step}}
			got, err := call(ctx, reader)
			reader.done()
			if got.State != identity.Unavailable || !errors.Is(err, context.Canceled) || len(got.Attempts) != 1 || got.Attempts[0].State != identity.Unavailable {
				t.Fatalf("canceled payload accepted: %#v %v", got, err)
			}
			preCanceled := &propertyReader{t: t, ctx: ctx}
			got, err = call(ctx, preCanceled)
			preCanceled.done()
			if !errors.Is(err, context.Canceled) || len(got.Attempts) != 0 {
				t.Fatal("query started on canceled context")
			}
		})
	}
}
func TestPropertyReferenceAdmissionPrecedesQueries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reader := &propertyReader{t: t, ctx: ctx}
	cases := map[string]propertyCall{
		"version-missing": func(c context.Context, r identity.Reader) (identity.PropertyEvidence, error) {
			v, e := identity.ReadVersion(c, r, identity.VersionReferences{Legacy: propertyRef(t, 1)})
			return v.PropertyEvidence, e
		},
		"version-duplicate": func(c context.Context, r identity.Reader) (identity.PropertyEvidence, error) {
			v, e := identity.ReadVersion(c, r, identity.VersionReferences{Legacy: propertyRef(t, 1), Full: propertyRef(t, 1)})
			return v.PropertyEvidence, e
		},
		"architecture-missing": func(c context.Context, r identity.Reader) (identity.PropertyEvidence, error) {
			v, e := identity.ReadArchitecture(c, r, identity.ArchitectureReferences{Version: propertyRef(t, 1)})
			return v.PropertyEvidence, e
		},
		"edition-duplicate-unused": func(c context.Context, r identity.Reader) (identity.PropertyEvidence, error) {
			v, e := identity.ReadEdition(c, r, identity.EditionReferences{Version: propertyRef(t, 1), Code: propertyRef(t, 2), Banners: propertyRef(t, 2)})
			return v.PropertyEvidence, e
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := call(ctx, reader)
			if !errors.Is(err, identity.ErrInvalidPropertyRequest) || got.State != identity.Unavailable || len(got.Attempts) != 0 {
				t.Fatalf("invalid request executed: %#v %v", got, err)
			}
		})
	}
	for name, call := range propertyCalls(t) {
		t.Run(name+"-nil", func(t *testing.T) {
			got, err := call(ctx, nil)
			if !errors.Is(err, identity.ErrInvalidPropertyRequest) || got.State != identity.Unavailable {
				t.Fatal("nil reader accepted")
			}
		})
	}
	reader.done()
}
func TestInvalidBootstrapNeverSelectsModernSource(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"26ai", "19.0.0.0", "19.0.0.0.0.0", "19.0.0.0.0 ", "+19.0.0.0.0", "１９.0.0.0.0", "19.0.0.0.4294967296", "8.1.7.4.0"} {
		t.Run(value, func(t *testing.T) {
			ctx := context.Background()
			for name, call := range propertyCalls(t) {
				t.Run(name, func(t *testing.T) {
					reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{bootstrapStep(propertyRef(t, 1), available(value))}}
					got, err := call(ctx, reader)
					reader.done()
					if err == nil || got.State != identity.Unavailable || len(got.Attempts) != 1 || got.Attempts[0].Err == nil {
						t.Fatalf("invalid bootstrap selected capability: %#v %v", got, err)
					}
				})
			}
		})
	}
}
func TestPropertyRejectsMalformedBootstrapProjection(t *testing.T) {
	t.Parallel()
	for _, result := range []session.Result{
		{Columns: []string{"VERSION", "VERSION"}, Rows: [][]session.Value{{{Text: "19.0.0.0.0"}, {Text: "19.0.0.0.0"}}}},
		{Columns: []string{"VERSION"}, Rows: [][]session.Value{{{Text: "19.0.0.0.0"}}, {{Text: "19.0.0.0.0"}}}},
		{Columns: []string{"VERSION"}, Rows: [][]session.Value{{}}},
	} {
		for name, call := range propertyCalls(t) {
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{{ref: propertyRef(t, 1), statement: legacyVersionSQL, result: result}}}
				got, err := call(ctx, reader)
				reader.done()
				if !errors.Is(err, identity.ErrInvalidEvidence) || got.State != identity.Unavailable {
					t.Fatal("malformed bootstrap accepted")
				}
			})
		}
	}
}
