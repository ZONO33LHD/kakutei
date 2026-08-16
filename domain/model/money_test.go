package model

import (
	"math"
	"testing"
)

func TestMoneyRoundDownTo(t *testing.T) {
	tests := []struct {
		name string
		m    Money
		unit int64
		want Money
	}{
		{"1000円未満切捨て", 1_234_567, 1000, 1_234_000},
		{"ちょうど単位", 1_234_000, 1000, 1_234_000},
		{"100円未満切捨て", 45_678, 100, 45_600},
		{"ゼロ", 0, 1000, 0},
		{"単位未満", 999, 1000, 0},
		{"負値は負の無限大方向", -150, 100, -200},
		{"負値ちょうど", -200, 100, -200},
		{"不正な単位はそのまま", 123, 0, 123},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.RoundDownTo(tt.unit); got != tt.want {
				t.Errorf("RoundDownTo(%d) = %d, want %d", tt.unit, got, tt.want)
			}
		})
	}
}

func TestMoneyMulDiv(t *testing.T) {
	tests := []struct {
		name     string
		m        Money
		num, den int64
		want     Money
	}{
		{"7.8%適用", 1_000_000, 78, 1000, 78_000},
		{"切り捨て", 999, 78, 1000, 77}, // 77.922 → 77
		{"負値は床関数", -999, 78, 1000, -78},
		{"等倍", 12345, 1, 1, 12345},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.MulDiv(tt.num, tt.den); got != tt.want {
				t.Errorf("MulDiv(%d, %d) = %d, want %d", tt.num, tt.den, got, tt.want)
			}
		})
	}
}

// 分母 0 は税率定数の設定バグなので panic する (黙って 0 を返さない)。
func TestMoneyMulDivZeroDenominatorPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("分母 0 は panic すべき")
		}
	}()
	_ = Money(100).MulDiv(1, 0)
}

func TestMoneyValidateAmountRange(t *testing.T) {
	if !Money(0).ValidateAmountRange() || !MaxAmount.ValidateAmountRange() || !(-MaxAmount).ValidateAmountRange() {
		t.Error("範囲内の金額が拒否された")
	}
	if (MaxAmount + 1).ValidateAmountRange() || (-MaxAmount - 1).ValidateAmountRange() {
		t.Error("範囲外の金額が受理された")
	}
}

func TestMoneyRoundDownToExtremes(t *testing.T) {
	// int64 下限近傍でもオーバーフロー (正値への回り込み) せず、単位の倍数を返すこと。
	// 実運用の金額は ValidateAmountRange で遮断されるため、ここでは安全性のみ検証する。
	m := Money(math.MinInt64 + 1)
	got := m.RoundDownTo(100)
	if got > 0 {
		t.Errorf("オーバーフローで正値になった: %d → %d", m, got)
	}
	if int64(got)%100 != 0 {
		t.Errorf("丸め結果が単位の倍数でない: %d", got)
	}

	// 通常範囲の負値は負の無限大方向へ丸める
	if got := Money(-MaxAmount - 50).RoundDownTo(100); got != -MaxAmount-100 {
		t.Errorf("RoundDownTo = %d, want %d", got, -MaxAmount-100)
	}
}

func TestMoneyMinMaxClamp(t *testing.T) {
	if got := Money(100).Min(200); got != 100 {
		t.Errorf("Min = %d, want 100", got)
	}
	if got := Money(100).Max(200); got != 200 {
		t.Errorf("Max = %d, want 200", got)
	}
	if got := Money(-5).ClampNonNegative(); got != 0 {
		t.Errorf("ClampNonNegative = %d, want 0", got)
	}
	if got := Money(5).ClampNonNegative(); got != 5 {
		t.Errorf("ClampNonNegative = %d, want 5", got)
	}
	if !Money(-1).IsNegative() || Money(0).IsNegative() {
		t.Error("IsNegative の判定が不正")
	}
	if Money(42).Yen() != 42 {
		t.Error("Yen() が不正")
	}
}
