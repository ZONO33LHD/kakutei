package model

import "testing"

func TestFiscalYearValidate(t *testing.T) {
	if err := FiscalYear(2025).Validate(); err != nil {
		t.Errorf("2025 は valid: %v", err)
	}
	for _, y := range []FiscalYear{7, 1999, 2101, 20250} {
		if err := y.Validate(); err == nil {
			t.Errorf("Validate(%d) はエラーになるべき", y)
		}
	}
}

func TestFiscalYearRange(t *testing.T) {
	y := FiscalYear(2025)
	if y.Start().String() != "2025-01-01" {
		t.Errorf("Start = %s", y.Start())
	}
	if y.End().String() != "2025-12-31" {
		t.Errorf("End = %s", y.End())
	}
	in, _ := ParseDate("2025-06-30")
	out, _ := ParseDate("2026-01-01")
	if !y.Contains(in) || y.Contains(out) {
		t.Error("Contains の判定が不正")
	}
}
