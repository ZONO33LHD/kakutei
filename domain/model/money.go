package model

import "math"

// Money は円単位の金額。税計算はすべて整数演算で行い、浮動小数点は使わない。
//
// 符号は文脈で意味を持つ (例: 事業所得の赤字、還付額は負)。
// 端数処理は RoundDownTo / 各ドメインサービスの規定に従う。
type Money int64

// MaxAmount は入力として受け付ける金額の上限 (1兆円)。
//
// 個人の確定申告で現実にあり得ない桁の入力を境界で遮断することで、
// 集計 (最大100明細×上限) や税率乗算 (最大×10000) が int64 で
// オーバーフローしないことを保証する。
const MaxAmount Money = 1_000_000_000_000

func (m Money) ValidateAmountRange() bool {
	return m >= -MaxAmount && m <= MaxAmount
}

func (m Money) Yen() int64 { return int64(m) }

func (m Money) IsNegative() bool { return m < 0 }

// RoundDownTo は unit 円未満を切り捨てる (国税通則法118条・119条の端数処理)。
// 負の金額に対しては 0 方向ではなく負の無限大方向へ丸める
// (例: -150 を 100 円単位 → -200)。ただし税額計算では負値に適用しない前提。
func (m Money) RoundDownTo(unit int64) Money {
	if unit <= 0 {
		return m
	}
	v := int64(m)
	if v >= 0 {
		return Money(v / unit * unit)
	}
	// 負値: 剰余を引いて負の無限大方向へ丸める。
	r := v % unit // Go の % は被除数の符号を保つため r ≤ 0
	if r == 0 {
		return Money(v)
	}
	// v-r は unit の倍数かつ v より大きい。さらに unit を引くと int64 下限を
	// 割り込む場合は下限側の倍数 (v-r) で打ち止めにしてオーバーフローを防ぐ。
	if v-r < math.MinInt64+unit {
		return Money(v - r)
	}
	return Money(v - r - unit)
}

// MulDiv は m × numerator ÷ denominator を整数演算 (切り捨て) で計算する。
// 税率適用 (例: ×7.8% = ×78÷1000) に使う。
// 負の被乗数には床関数 (負の無限大方向への切り捨て) を適用する。
//
// denominator は税制定数 (コンパイル時定数) を渡す前提であり、0 は設定バグとして
// panic する (黙って 0 円を返すと税額誤りが検出不能になるため)。
func (m Money) MulDiv(numerator, denominator int64) Money {
	if denominator == 0 {
		panic("model.Money.MulDiv: 分母が 0 です (税率定数の設定ミス)")
	}
	v := int64(m) * numerator
	q := v / denominator
	if v%denominator != 0 && (v < 0) != (denominator < 0) {
		q--
	}
	return Money(q)
}

func (m Money) Min(other Money) Money {
	if m < other {
		return m
	}
	return other
}

func (m Money) Max(other Money) Money {
	if m > other {
		return m
	}
	return other
}

func (m Money) ClampNonNegative() Money {
	if m < 0 {
		return 0
	}
	return m
}
