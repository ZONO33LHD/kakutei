package policy

import "testing"

// lookupPension は速算表を適用して控除額を返すテスト用ヘルパ。
// 実装 (domain/service/taxation) と同様、控除額は年金収入以下にキャップする。
func lookupPension(table []PensionDeductionBracket, maxDeduction, pension int64) int64 {
	deduction := maxDeduction
	for _, b := range table {
		if pension <= b.Threshold {
			if b.RatePct == 0 {
				deduction = b.Fixed
			} else {
				deduction = pension*b.RatePct/100 + b.Fixed
			}
			break
		}
	}
	if deduction > pension {
		return pension
	}
	return deduction
}

// 国税庁 令和7年分 公的年金等に係る雑所得の速算表との照合 (65歳未満)。
func TestPensionDeductionUnder65LegalValues(t *testing.T) {
	tests := []struct {
		pension int64
		want    int64
	}{
		{500_000, 500_000},      // ≤60万: 全額控除
		{600_000, 600_000},      // 境界
		{1_000_000, 600_000},    // 60万超〜130万: 一律60万
		{3_000_000, 1_025_000},  // 300万×25%+27.5万 = 102.5万 (雑所得197.5万)
		{4_100_000, 1_300_000},  // 境界: 410万×25%+27.5万 = 130万
		{5_000_000, 1_435_000},  // 500万×15%+68.5万
		{8_000_000, 1_855_000},  // 800万×5%+145.5万
		{10_000_001, 1_955_000}, // 1,000万超: 上限195.5万
	}
	for _, tt := range tests {
		got := lookupPension(PensionDeductionUnder65, PensionDeductionUnder65Max, tt.pension)
		if got != tt.want {
			t.Errorf("65歳未満 年金%d円: 控除 = %d, want %d", tt.pension, got, tt.want)
		}
	}
}

// 国税庁 令和7年分 公的年金等に係る雑所得の速算表との照合 (65歳以上)。
func TestPensionDeductionOver65LegalValues(t *testing.T) {
	tests := []struct {
		pension int64
		want    int64
	}{
		{1_000_000, 1_000_000},  // ≤110万: 全額控除
		{2_000_000, 1_100_000},  // 110万超〜330万: 一律110万
		{3_300_000, 1_100_000},  // 境界: 330万×25%+27.5万 = 110万 と連続
		{4_000_000, 1_275_000},  // 400万×25%+27.5万
		{10_000_001, 1_955_000}, // 上限
	}
	for _, tt := range tests {
		got := lookupPension(PensionDeductionOver65, PensionDeductionOver65Max, tt.pension)
		if got != tt.want {
			t.Errorf("65歳以上 年金%d円: 控除 = %d, want %d", tt.pension, got, tt.want)
		}
	}
}

// 速算表が区分境界で連続していること (境界前後で控除額が逆行しない)。
func TestPensionDeductionContinuity(t *testing.T) {
	for _, table := range [][]PensionDeductionBracket{PensionDeductionUnder65, PensionDeductionOver65} {
		for _, b := range table {
			at := lookupPension(table, 1_955_000, b.Threshold)
			next := lookupPension(table, 1_955_000, b.Threshold+1)
			if next < at {
				t.Errorf("速算表が境界 %d で逆行: %d → %d", b.Threshold, at, next)
			}
		}
	}
}
