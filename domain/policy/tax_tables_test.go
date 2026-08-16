package policy

import "testing"

// 税制定数は令和7年分 (2025) のみ対応。他年度への誤適用を防ぐ。
func TestSupportsFiscalYear(t *testing.T) {
	if !SupportsFiscalYear(2025) {
		t.Error("2025 はサポートされるべき")
	}
	if SupportsFiscalYear(2024) || SupportsFiscalYear(2026) {
		t.Error("未対応年度がサポート扱いになっている")
	}
}

func TestLookupBracket(t *testing.T) {
	tests := []struct {
		name   string
		income int64
		want   int64
	}{
		{"最小区分", 1_000_000, 950_000},
		{"境界ちょうど", 1_320_000, 950_000},
		{"境界+1", 1_320_001, 880_000},
		{"中間区分", 5_000_000, 630_000},
		{"最終区分", 25_000_000, 160_000},
		{"全区分超はフォールバック", 25_000_001, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LookupBracket(BasicDeductionTable, tt.income, 0); got != tt.want {
				t.Errorf("LookupBracket(%d) = %d, want %d", tt.income, got, tt.want)
			}
		})
	}
}

// 速算表の整合性: 区分境界で税額が連続していること (境界の前後で逆転しない)。
func TestIncomeTaxTableContinuity(t *testing.T) {
	taxAt := func(income int64) int64 {
		for _, b := range IncomeTaxTable {
			if income <= b.Threshold {
				return income*b.RatePct/100 - b.Deduction
			}
		}
		return income*IncomeTaxTopRatePct/100 - IncomeTaxTopDeduction
	}
	for _, b := range IncomeTaxTable {
		at := taxAt(b.Threshold)
		next := taxAt(b.Threshold + 1000)
		if next < at {
			t.Errorf("速算表が境界 %d で不連続: %d → %d", b.Threshold, at, next)
		}
	}
}

// 配偶者控除の法定値スポットチェック (国税庁タックスアンサー No.1191/1195)。
func TestSpouseDeductionTableLegalValues(t *testing.T) {
	tests := []struct {
		name         string
		table        []BracketEntry
		spouseIncome int64
		want         int64
	}{
		{"≤900万/配偶者控除", SpouseDeductionTable, 580_000, 380_000},
		{"≤900万/特別控除満額", SpouseDeductionTable, 950_000, 380_000},
		{"≤900万/特別控除最小", SpouseDeductionTable, 1_330_000, 30_000},
		{"900-950万/配偶者控除", SpouseDeductionTable9M, 580_000, 260_000},
		{"950-1000万/配偶者控除", SpouseDeductionTable10M, 580_000, 130_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LookupBracket(tt.table, tt.spouseIncome, 0); got != tt.want {
				t.Errorf("LookupBracket(%d) = %d, want %d", tt.spouseIncome, got, tt.want)
			}
		})
	}
}

// 老人控除対象配偶者の控除額 (48万/32万/16万)。
func TestSpouseElderlyDeductionLegalValues(t *testing.T) {
	if SpouseElderlyDeduction != 480_000 || SpouseElderlyDeduction9M != 320_000 || SpouseElderlyDeduction10M != 160_000 {
		t.Error("老人控除対象配偶者の控除額が法定値と不一致")
	}
}

// 住宅性能区分の型検証。
func TestHousingCategoryValid(t *testing.T) {
	for _, c := range []HousingCategory{HousingGeneral, HousingCertified, HousingZEH, HousingEnergyEfficient} {
		if !c.Valid() {
			t.Errorf("%q は valid であるべき", c)
		}
	}
	if HousingCategory("mansion").Valid() || HousingCategory("").Valid() {
		t.Error("未定義の住宅区分が valid になっている")
	}
}

// 簡易課税のみなし仕入率の法定値 (消費税法第37条)。
func TestSimplifiedDeemedRatiosLegalValues(t *testing.T) {
	want := map[SimplifiedBusinessType]int64{
		SimplifiedWholesale:     90,
		SimplifiedRetail:        80,
		SimplifiedManufacturing: 70,
		SimplifiedOther:         60,
		SimplifiedService:       50,
		SimplifiedRealEstate:    40,
	}
	for bt, ratio := range want {
		if got := SimplifiedDeemedRatios[bt]; got != ratio {
			t.Errorf("事業区分 %d のみなし仕入率 = %d%%, want %d%%", bt, got, ratio)
		}
	}
	if SimplifiedBusinessType(0).Valid() || SimplifiedBusinessType(7).Valid() {
		t.Error("範囲外の事業区分が valid になっている")
	}
	if !SimplifiedWholesale.Valid() || !SimplifiedRealEstate.Valid() {
		t.Error("正当な事業区分が invalid になっている")
	}
}
