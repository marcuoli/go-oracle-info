package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/marcuoli/go-oracle-info/identity"
	"github.com/marcuoli/go-oracle-session/session"
)

func TestReadEditionPreservesExplicitCodesAndStandardAmbiguity(t *testing.T) {
	t.Parallel()
	for code, want := range map[string]identity.EditionType{
		"EE": identity.EnterpriseEdition, "SE": identity.StandardEdition, "CORE EE": identity.CoreEnterpriseEdition, "CORE SE": identity.CoreStandardEdition, "SE2": identity.StandardEditionFamily, "XE": identity.ExpressEdition, "FREE": identity.FreeEdition,
		"PO": identity.PersonalEdition, "HP": identity.HighPerformanceEdition, "XP": identity.ExtremePerformanceEdition,
	} {
		t.Run(code, func(t *testing.T) {
			ctx := context.Background()
			refs := editionRefs(t)
			reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{bootstrapStep(refs.Version, available("12.1.0.2.0")), {ref: refs.Code, statement: editionCodeSQL, result: scalarFact("EDITION", available(code))}}}
			got, err := identity.ReadEdition(ctx, reader, refs)
			reader.done()
			if err != nil || got.State != identity.Available || got.Value != want || got.Code != available(code) || got.Precision != identity.ObservedEditionCode || got.Fallback || got.Label.State != identity.Unavailable || got.Source != identity.InstanceEditionSource || got.Banners != nil {
				t.Fatalf("code precision inferred: %#v %v", got, err)
			}
		})
	}
}
func TestReadEditionDoesNotFallbackAfterMissingUnknownOrFailedCode(t *testing.T) {
	t.Parallel()
	failure := errors.New("private edition permission failure")
	for _, tc := range []struct {
		name    string
		code    identity.Text
		err     error
		want    identity.State
		unknown bool
	}{
		{"null", identity.Text{State: identity.Null}, nil, identity.Null, false},
		{"no-rows", identity.Text{State: identity.NoRows}, nil, identity.NoRows, false},
		{"unknown", available("FUTURE"), nil, identity.Unavailable, true},
		{"case-not-inferred", available("ee"), nil, identity.Unavailable, true},
		{"label-not-code", available("Enterprise Edition"), nil, identity.Unavailable, true},
		{"failed", identity.Text{State: identity.Unavailable}, failure, identity.Unavailable, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			refs := editionRefs(t)
			reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{bootstrapStep(refs.Version, available("19.0.0.0.0")), {ref: refs.Code, statement: editionCodeSQL, result: scalarFact("EDITION", tc.code), err: tc.err}}}
			got, err := identity.ReadEdition(ctx, reader, refs)
			reader.done()
			if got.State != tc.want || got.Value != "" || got.Code != tc.code || got.Banners != nil || got.Fallback {
				t.Fatalf("code changed or speculative read: %#v %v", got, err)
			}
			if tc.err != nil && !errors.Is(err, tc.err) || tc.unknown && !errors.Is(err, identity.ErrUnknownEdition) || tc.err == nil && !tc.unknown && err != nil {
				t.Fatal("code failure lost", err)
			}
		})
	}
}
func TestReadEditionLegacyUsesOnlyExplicitPreservedLabel(t *testing.T) {
	t.Parallel()
	for label, want := range map[string]identity.EditionType{"Enterprise Edition": identity.EnterpriseEdition, "Standard Edition": identity.StandardEdition, "Express Edition": identity.ExpressEdition} {
		t.Run(label, func(t *testing.T) {
			ctx := context.Background()
			refs := editionRefs(t)
			banner := "Oracle Database 11g " + label + " Release 11.2.0.4.0 - 64bit Production"
			result := session.Result{Columns: []string{"BANNER"}, Rows: [][]session.Value{{{Text: "TNS for Linux: Version 11.2.0.4.0 - Production"}}, {{Text: banner}}}}
			reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{bootstrapStep(refs.Version, available("11.2.0.4.0")), {ref: refs.Banners, statement: legacyBannersSQL, result: result}}}
			got, err := identity.ReadEdition(ctx, reader, refs)
			reader.done()
			if err != nil || got.State != identity.Available || got.Value != want || got.Label != available(label) || got.Code.State != identity.Unavailable || !got.Fallback || got.Precision != identity.ExplicitLegacyLabel || got.Source != identity.VersionBannersSource || len(got.Banners) != 2 || got.Banners[1].Banner.Value != banner {
				t.Fatalf("legacy code inferred/raw label lost: %#v %v", got, err)
			}
			result.Rows[1][0].Text = "changed"
			if got.Banners[1].Banner.Value != banner {
				t.Fatal("raw banner alias changed")
			}
		})
	}
}
func TestReadEditionLegacyRejectsAmbiguityAndIncompleteShapes(t *testing.T) {
	t.Parallel()
	banner := session.Value{Text: "Oracle Database 11g Enterprise Edition Release 11.2.0.4.0 - 64bit Production"}
	for _, tc := range []struct {
		name   string
		result session.Result
		want   error
		state  identity.State
	}{
		{"empty", session.Result{Columns: []string{"BANNER"}}, nil, identity.NoRows},
		{"null", session.Result{Columns: []string{"BANNER"}, Rows: [][]session.Value{{{Null: true}}}}, identity.ErrUnknownEdition, identity.Unavailable},
		{"duplicate", session.Result{Columns: []string{"BANNER"}, Rows: [][]session.Value{{banner}, {banner}}}, identity.ErrUnknownEdition, identity.Unavailable},
		{"unknown-label", session.Result{Columns: []string{"BANNER"}, Rows: [][]session.Value{{{Text: "Oracle Database 11g Unknown Edition Release 11.2.0.4.0 - Production"}}}}, identity.ErrUnknownEdition, identity.Unavailable},
		{"late-malformed", session.Result{Columns: []string{"BANNER"}, Rows: [][]session.Value{{banner}, {}}}, identity.ErrInvalidEvidence, identity.Unavailable},
		{"too-many", session.Result{Columns: []string{"BANNER"}, Rows: make([][]session.Value, 17)}, identity.ErrInvalidEvidence, identity.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			refs := editionRefs(t)
			reader := &propertyReader{t: t, ctx: ctx, steps: []propertyStep{bootstrapStep(refs.Version, available("11.2.0.4.0")), {ref: refs.Banners, statement: legacyBannersSQL, result: tc.result}}}
			got, err := identity.ReadEdition(ctx, reader, refs)
			reader.done()
			if got.State != tc.state || got.Value != "" || (tc.want == nil && err != nil) || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("ambiguous/incomplete source accepted: %#v %v", got, err)
			}
			if errors.Is(tc.want, identity.ErrInvalidEvidence) && got.Banners != nil {
				t.Fatal("malformed banner prefix returned")
			}
		})
	}
}
