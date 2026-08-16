package taxation

import (
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/model"
)

// 給与所得の速算 (令和7年分、国税庁の速算表と照合)。
// 660万以下は A = 収入÷4 (千円未満切捨) を用いる端数規定がある。
func TestSalaryIncome(t *testing.T) {
	tests := []struct {
		name    string
		revenue model.Money
		want    model.Money
	}{
		{"ゼロ", 0, 0},
		{"控除額未満は所得0", 600_000, 0},
		{"最低保障ちょうど", 650_000, 0},
		{"190万以下は一律65万控除", 1_000_000, 350_000},
		{"190万境界", 1_900_000, 1_250_000},
		// 1,920,500 → A=480,000 → 480,000×2.8−80,000 = 1,264,000 (直接式なら1,264,350になる誤り)
		{"A方式の端数規定", 1_920_500, 1_264_000},
		{"190万超境界+1", 1_900_001, 1_250_000}, // A=475,000 → 1,250,000 (連続性)
		{"360万境界", 3_600_000, 2_440_000},    // A=900,000×2.8−8万
		{"360万超境界+1", 3_600_001, 2_440_000}, // A=900,000×3.2−44万
		{"660万境界", 6_600_000, 4_840_000},    // A=1,650,000×3.2−44万
		{"660万超境界+1", 6_600_001, 4_840_000}, // 6,600,001×0.9−110万 (床)
		{"850万境界", 8_500_000, 6_550_000},
		{"850万超は控除195万", 10_000_000, 8_050_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SalaryIncome(tt.revenue); got != tt.want {
				t.Errorf("SalaryIncome(%d) = %d, want %d", tt.revenue.Yen(), got.Yen(), tt.want.Yen())
			}
		})
	}
}

// 青色申告特別控除の利益上限キャップ (租税特別措置法第25条の2)。
func TestBusinessIncomeBlueReturnCap(t *testing.T) {
	tests := []struct {
		name              string
		revenue, expenses model.Money
		deduction         model.Money
		wantIncome        model.Money
		wantEffective     model.Money
		wantCapped        bool
	}{
		{"利益<控除は利益が上限", 500_000, 200_000, 650_000, 0, 300_000, true},
		{"赤字は控除0で赤字額を維持", 165_000, 350_779, 650_000, -185_779, 0, true},
		{"利益>控除は全額適用", 3_000_000, 1_000_000, 650_000, 1_350_000, 650_000, false},
		{"利益=控除で所得0", 1_000_000, 350_000, 650_000, 0, 650_000, false},
		{"収入0経費0", 0, 0, 650_000, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BusinessIncome(tt.revenue, tt.expenses, tt.deduction)
			if got.Income != tt.wantIncome {
				t.Errorf("Income = %d, want %d", got.Income.Yen(), tt.wantIncome.Yen())
			}
			if got.EffectiveBlueReturnDeduction != tt.wantEffective {
				t.Errorf("Effective = %d, want %d", got.EffectiveBlueReturnDeduction.Yen(), tt.wantEffective.Yen())
			}
			if got.Capped != tt.wantCapped {
				t.Errorf("Capped = %t, want %t", got.Capped, tt.wantCapped)
			}
		})
	}
}

func TestOneTimeIncome(t *testing.T) {
	tests := []struct {
		gross model.Money
		want  model.Money
	}{
		{0, 0},
		{500_000, 0},         // 特別控除以下
		{1_500_000, 500_000}, // (150万−50万)/2
		{500_001, 0},         // 1円/2 = 0
	}
	for _, tt := range tests {
		if got := OneTimeIncome(tt.gross); got != tt.want {
			t.Errorf("OneTimeIncome(%d) = %d, want %d", tt.gross.Yen(), got.Yen(), tt.want.Yen())
		}
	}
}

func TestValidateBlueReturnDeduction(t *testing.T) {
	for _, v := range []model.Money{0, 100_000, 550_000, 650_000} {
		if err := ValidateBlueReturnDeduction(v); err != nil {
			t.Errorf("%d は法定額なのにエラー: %v", v.Yen(), err)
		}
	}
	for _, v := range []model.Money{1, 500_000, 660_000, -1} {
		if err := ValidateBlueReturnDeduction(v); err == nil {
			t.Errorf("%d は法定外なのに受理された", v.Yen())
		}
	}
}
