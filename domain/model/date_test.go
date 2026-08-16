package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	d, err := ParseDate("2025-03-15")
	if err != nil {
		t.Fatalf("ParseDate error: %v", err)
	}
	if d.Year() != 2025 || d.Month() != time.March || d.Day() != 15 {
		t.Errorf("got %v", d)
	}
	if d.String() != "2025-03-15" {
		t.Errorf("String() = %q", d.String())
	}

	invalid := []string{"2025/03/15", "2025-13-01", "2025-02-30", "abc", ""}
	for _, s := range invalid {
		if _, err := ParseDate(s); err == nil {
			t.Errorf("ParseDate(%q) はエラーになるべき", s)
		}
	}
}

func TestNewDate(t *testing.T) {
	if _, err := NewDate(2025, time.February, 29); err == nil {
		t.Error("2025-02-29 は実在しないためエラーになるべき")
	}
	if _, err := NewDate(2024, time.February, 29); err != nil {
		t.Errorf("2024-02-29 (閏年) はエラーであってはならない: %v", err)
	}
}

func TestDateComparison(t *testing.T) {
	a, _ := ParseDate("2025-01-01")
	b, _ := ParseDate("2025-12-31")
	if !a.Before(b) || b.Before(a) {
		t.Error("Before の判定が不正")
	}
	if !b.After(a) || a.After(b) {
		t.Error("After の判定が不正")
	}
	if a.Before(a) || a.After(a) {
		t.Error("同一日付の比較が不正")
	}
}

func TestDateAgeAt(t *testing.T) {
	birth, _ := ParseDate("2000-06-15")
	tests := []struct {
		at   string
		want int
	}{
		{"2025-12-31", 25},
		{"2025-06-15", 25}, // 誕生日当日
		{"2025-06-14", 24}, // 誕生日前日
	}
	for _, tt := range tests {
		at, _ := ParseDate(tt.at)
		if got := birth.AgeAt(at); got != tt.want {
			t.Errorf("AgeAt(%s) = %d, want %d", tt.at, got, tt.want)
		}
	}
}

// 税法上の年齢は誕生日の前日に加齢する (年齢計算ニ関スル法律・民法143条)。
// 国税庁の令和7年分年齢区分: 1961-01-01 以前生まれ = 65歳以上、2010-01-01 以前生まれ = 16歳以上。
func TestDateTaxAgeAt(t *testing.T) {
	yearEnd, _ := ParseDate("2025-12-31")
	tests := []struct {
		birth string
		want  int
	}{
		{"1961-01-01", 65}, // 前日 (12/31) に65歳到達
		{"1961-01-02", 64},
		{"1960-12-31", 65},
		{"2010-01-01", 16}, // 扶養控除の対象になる境界
		{"2010-01-02", 15},
		{"1956-01-01", 70}, // 老人扶養・老人配偶者の境界
	}
	for _, tt := range tests {
		birth, _ := ParseDate(tt.birth)
		if got := birth.TaxAgeAt(yearEnd); got != tt.want {
			t.Errorf("TaxAgeAt(%s → 2025-12-31) = %d, want %d", tt.birth, got, tt.want)
		}
	}
}

func TestDateJSON(t *testing.T) {
	d, _ := ParseDate("2025-03-15")
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if string(b) != `"2025-03-15"` {
		t.Errorf("Marshal = %s", b)
	}

	var restored Date
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if restored != d {
		t.Errorf("round trip mismatch: %v != %v", restored, d)
	}

	var zero Date
	if err := json.Unmarshal([]byte(`""`), &zero); err != nil {
		t.Fatalf("空文字列は zero 値として受理すべき: %v", err)
	}
	if !zero.IsZero() {
		t.Error("空文字列は IsZero になるべき")
	}

	if err := json.Unmarshal([]byte(`123`), &zero); err == nil {
		t.Error("数値はエラーになるべき")
	}
}
