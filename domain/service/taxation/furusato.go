package taxation

import (
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// EstimateFurusatoLimit はふるさと納税の控除上限額を推定する。
//
// 住民税所得割額の20%を全額控除できる寄附額の近似式:
//
//	上限 ≈ 住民税所得割額 × 20% ÷ (100% − 所得税率×1.021 − 10%) + 2,000円
//	住民税所得割額 = 課税所得 (1,000円未満切捨) × 10%
//
// 調整控除・住民税の課税所得差異は考慮しない推定値である。
// marginalTaxRatePct に 0 を渡すと課税所得から限界税率を自動判定する。
func EstimateFurusatoLimit(totalIncome, totalIncomeDeductions model.Money, marginalTaxRatePct int64) model.Money {
	taxable := (totalIncome - totalIncomeDeductions).ClampNonNegative().
		RoundDownTo(policy.TaxableIncomeRoundingUnit)
	if taxable <= 0 {
		return 0
	}
	if marginalTaxRatePct <= 0 {
		marginalTaxRatePct = marginalIncomeTaxRate(taxable)
	}

	// 住民税所得割額 = 課税所得 × 10%
	residentTax := taxable.MulDiv(policy.ResidentTaxRatePct, 100)
	if residentTax <= 0 {
		return 0
	}

	// 分母 (千分率): 1000 − 所得税率×1.021×10 − 100 (住民税基本分10%)
	denomPerMille := 1000 - marginalTaxRatePct*1021/100 - 100
	if denomPerMille <= 0 {
		// 所得税率が極端に高い場合の安全策: 住民税所得割の20%のみ
		return residentTax.MulDiv(policy.FurusatoResidentTaxRatioPct, 100) + policy.FurusatoSelfBurden
	}

	// 上限 = 住民税所得割額 × 20% ÷ (分母/1000) + 2,000
	return residentTax.MulDiv(policy.FurusatoResidentTaxRatioPct*10, denomPerMille) + policy.FurusatoSelfBurden
}

// marginalIncomeTaxRate は課税所得に対する所得税の限界税率 (%) を返す。
func marginalIncomeTaxRate(taxable model.Money) int64 {
	if taxable <= 0 {
		return 0
	}
	for _, b := range policy.IncomeTaxTable {
		if taxable.Yen() <= b.Threshold {
			return b.RatePct
		}
	}
	return policy.IncomeTaxTopRatePct
}
