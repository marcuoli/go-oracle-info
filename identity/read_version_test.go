package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestReadVersionSelectsActualNumericPrecision(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		legacy    identity.Text
		full      *identity.Text
		want      string
		state     identity.State
		precision identity.VersionPrecision
		fallback  bool
	}{
		{"9i", available("9.2.0.8.0"), nil, "9.2.0.8.0", identity.Available, identity.LegacyPrecision, false},
		{"10g", available("10.2.0.1.0"), nil, "10.2.0.1.0", identity.Available, identity.LegacyPrecision, false},
		{"11g", available("11.2.0.4.0"), nil, "11.2.0.4.0", identity.Available, identity.LegacyPrecision, false},
		{"12c", available("12.2.0.1.0"), nil, "12.2.0.1.0", identity.Available, identity.LegacyPrecision, false},
		{"18-full", available("18.0.0.0.0"), textPointer(available("18.3.0.0.0")), "18.3.0.0.0", identity.Available, identity.FullPrecision, false},
		{"19-full", available("19.0.0.0.0"), textPointer(available("19.32.0.0.0")), "19.32.0.0.0", identity.Available, identity.FullPrecision, false},
		{"26ai-stays-23", available("23.0.0.0.0"), textPointer(available("23.26.3.0.0")), "23.26.3.0.0", identity.Available, identity.FullPrecision, false},
		{"full-null", available("19.0.0.0.0"), textPointer(identity.Text{State: identity.Null}), "19.0.0.0.0", identity.Available, identity.LegacyPrecision, true},
		{"full-no-row", available("19.0.0.0.0"), textPointer(identity.Text{State: identity.NoRows}), "19.0.0.0.0", identity.Available, identity.LegacyPrecision, true},
		{"bootstrap-null", identity.Text{State: identity.Null}, nil, "", identity.Null, "", false},
		{"bootstrap-no-row", identity.Text{State: identity.NoRows}, nil, "", identity.NoRows, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			refs := versionRefs(t)
			steps := []propertyStep{bootstrapStep(refs.Legacy, tc.legacy)}
			if tc.full != nil {
				steps = append(steps, propertyStep{ref: refs.Full, statement: fullVersionSQL, result: scalarFact("VERSION", *tc.full)})
			}
			reader := &propertyReader{t: t, ctx: ctx, steps: steps}
			got, err := identity.ReadVersion(ctx, reader, refs)
			reader.done()
			if err != nil || got.State != tc.state || got.Value != tc.want || got.Precision != tc.precision || got.Fallback != tc.fallback || got.Legacy != tc.legacy {
				t.Fatalf("version=%#v err=%v", got, err)
			}
			if (got.Full == nil) != (tc.full == nil) || (got.Full != nil && *got.Full != *tc.full) {
				t.Fatal("full NULL/unselected provenance lost")
			}
			source := identity.InstanceVersionSource
			if tc.precision == identity.FullPrecision {
				source = identity.InstanceFullVersionSource
			}
			if got.Source != source {
				t.Fatal("wrong selected source")
			}
			if len(got.Attempts) != len(steps) {
				t.Fatal("attempts lost")
			}
		})
	}
}
func textPointer(text identity.Text) *identity.Text { return &text }

func TestReadVersionRetainsLegacyAlongsideFullFailure(t *testing.T) {
	t.Parallel()
	failure := errors.New("private full-column failure")
	for _, tc := range []struct {
		name   string
		result session.Result
		err    error
		cancel bool
	}{
		{"query", session.Result{}, failure, false},
		{"projection", session.Result{Columns: []string{"WRONG"}}, nil, false},
		{"canceled-payload", scalarFact("VERSION", available("19.32.0.0.0")), nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			refs := versionRefs(t)
			second := propertyStep{ref: refs.Full, statement: fullVersionSQL, result: tc.result, err: tc.err}
			if tc.cancel {
				second.after = cancel
			}
			reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{bootstrapStep(refs.Legacy, available("19.0.0.0.0")), second}}
			got, err := identity.ReadVersion(ctx, reader, refs)
			reader.done()
			if err == nil || got.State != identity.Available || got.Value != "19.0.0.0.0" || got.Precision != identity.LegacyPrecision || !got.Fallback || got.Full != nil || got.Source != identity.InstanceVersionSource {
				t.Fatalf("legacy/failure boundary lost: %#v %v", got, err)
			}
			if tc.err != nil && !errors.Is(err, failure) {
				t.Fatal("query cause lost")
			}
			if tc.cancel && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost")
			}
			checkAttempts(t, got.Attempts, []identity.PropertySource{identity.InstanceVersionSource, identity.InstanceFullVersionSource}, []identity.State{identity.Available, identity.Unavailable})
			if got.Attempts[1].Err == nil {
				t.Fatal("failed attempt became successful fallback")
			}
		})
	}
}
func TestReadVersionRejectsInvalidOrContradictoryFullValue(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"23.26.3.0.0", "19c", "19.32.0", "19.32.0.0.0\n"} {
		t.Run(value, func(t *testing.T) {
			ctx := context.Background()
			refs := versionRefs(t)
			reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{bootstrapStep(refs.Legacy, available("19.0.0.0.0")), {ref: refs.Full, statement: fullVersionSQL, result: scalarFact("VERSION", available(value))}}}
			got, err := identity.ReadVersion(ctx, reader, refs)
			reader.done()
			if !errors.Is(err, identity.ErrInvalidEvidence) || got.State != identity.Unavailable || got.Value != "" || got.Precision != "" || got.Legacy.Value != "19.0.0.0.0" || got.Full == nil || got.Full.Value != value {
				t.Fatalf("contradiction hidden: %#v %v", got, err)
			}
		})
	}
}
