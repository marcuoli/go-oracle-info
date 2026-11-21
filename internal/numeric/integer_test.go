package numeric_test

import (
	"testing"

	"github.com/marcuoli/go-oracle-info/internal/numeric"
)

func TestIntegerText(t *testing.T) {
	tests := []struct {
		text             string
		unsigned, signed bool
	}{
		{"0", true, true},
		{"0001", true, true},
		{"184467440737095516160000", true, true},
		{"+001", false, true},
		{"-0", false, true},
		{"-184467440737095516160000", false, true},
		{"", false, false},
		{"+", false, false},
		{"-", false, false},
		{"--1", false, false},
		{" 1", false, false},
		{"1\n", false, false},
		{"1_000", false, false},
		{"0x10", false, false},
		{"1.0", false, false},
		{"1e3", false, false},
		{"\u0661", false, false},
		{"1\xff", false, false},
	}
	for _, test := range tests {
		if got := numeric.IsUnsignedInteger(test.text); got != test.unsigned {
			t.Errorf("IsUnsignedInteger(%q) = %v, want %v", test.text, got, test.unsigned)
		}
		if got := numeric.IsSignedInteger(test.text); got != test.signed {
			t.Errorf("IsSignedInteger(%q) = %v, want %v", test.text, got, test.signed)
		}
	}
}
