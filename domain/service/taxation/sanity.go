package taxation

import (
	"fmt"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// SanitySeverity はサニティチェック項目の重要度。
type SanitySeverity string

const (
	SanityError   SanitySeverity = "error"   // 申告不可レベルの異常
	SanityWarning SanitySeverity = "warning" // 確認が必要
	SanityInfo    SanitySeverity = "info"    // 参考情報
)

// SanityCheckItem はサニティチェックの1項目。
type SanityCheckItem struct {
	Severity SanitySeverity
	Code     string
	Message  string
}

// SanityCheckResult は申告前サニティチェックの結果。
type SanityCheckResult struct {
	Passed       bool // error が 0 件なら true
	Items        []SanityCheckItem
	ErrorCount   int
	WarningCount int
}

// largeBusinessLossThreshold は事業損失の警告閾値 (1,000万円)。
const largeBusinessLossThreshold = model.Money(-10_000_000)

// SanityCheckIncomeTax は所得税計算の入力と結果の整合性を検証する。
// 申告前の最終確認として、明らかな異常 (端数処理漏れ・計算不一致等) を検出する。
func SanityCheckIncomeTax(in *IncomeTaxInput, result *IncomeTaxResult) *SanityCheckResult {
	var items []SanityCheckItem
	add := func(severity SanitySeverity, code, message string) {
		items = append(items, SanityCheckItem{Severity: severity, Code: code, Message: message})
	}

	businessProfit := in.BusinessRevenue - in.BusinessExpenses

	// 1. 赤字なのに青色申告特別控除が適用されている (防御的チェック)
	if businessProfit < 0 && result.EffectiveBlueReturnDeduction > 0 {
		add(SanityError, "BLUE_DEDUCTION_ON_LOSS", fmt.Sprintf(
			"事業が赤字（%d円）にもかかわらず青色申告特別控除%d円が適用されています",
			businessProfit.Yen(), result.EffectiveBlueReturnDeduction.Yen()))
	}

	// 2. 控除が事業利益を超過
	if businessProfit > 0 && result.EffectiveBlueReturnDeduction > businessProfit {
		add(SanityError, "BLUE_DEDUCTION_EXCEEDS_PROFIT", fmt.Sprintf(
			"青色申告特別控除（%d円）が事業利益（%d円）を超過しています",
			result.EffectiveBlueReturnDeduction.Yen(), businessProfit.Yen()))
	}

	// 3. 事業損失が異常に大きい
	if result.BusinessIncome < largeBusinessLossThreshold {
		add(SanityWarning, "LARGE_BUSINESS_LOSS", fmt.Sprintf(
			"事業損失が%d円と大きい値です。入力を確認してください", result.BusinessIncome.Yen()))
	}

	// 4. 課税所得0なのに税額発生
	if result.TaxableIncome == 0 && result.IncomeTaxBase > 0 {
		add(SanityError, "TAX_ON_ZERO_INCOME", "課税所得が0円ですが算出税額が発生しています")
	}

	// 5. 損益通算後の合計所得が負
	if totalRaw := result.SalaryIncome + result.BusinessIncome; totalRaw < 0 {
		add(SanityInfo, "NEGATIVE_TOTAL_INCOME", fmt.Sprintf(
			"損益通算後の合計所得が負（%d円）です。純損失の繰越控除の適用を検討してください", totalRaw.Yen()))
	}

	// 6. 課税所得の1,000円未満切捨て漏れ
	if result.TaxableIncome > 0 && result.TaxableIncome.Yen()%policy.TaxableIncomeRoundingUnit != 0 {
		add(SanityError, "TAXABLE_INCOME_ROUNDING", fmt.Sprintf(
			"課税所得（%d円）が1,000円単位になっていません", result.TaxableIncome.Yen()))
	}

	// 7. 復興特別所得税の計算不一致
	expected := result.IncomeTaxAfterCredits.MulDiv(policy.ReconstructionTaxRateNum, policy.ReconstructionTaxRateDenom)
	if result.ReconstructionTax != expected {
		add(SanityError, "RECONSTRUCTION_TAX_MISMATCH", fmt.Sprintf(
			"復興特別所得税の計算が不一致です（実際: %d円、期待: %d円）",
			result.ReconstructionTax.Yen(), expected.Yen()))
	}

	// 8. 税額控除が算出税額を超過
	if result.IncomeTaxBase > 0 && result.TotalTaxCredits > result.IncomeTaxBase {
		add(SanityWarning, "CREDITS_EXCEED_TAX", fmt.Sprintf(
			"税額控除（%d円）が算出税額（%d円）を超過しています",
			result.TotalTaxCredits.Yen(), result.IncomeTaxBase.Yen()))
	}

	// 9. 給与収入があるのに源泉徴収が0
	if in.SalaryRevenue > 0 && in.SalaryWithheldTax == 0 {
		add(SanityWarning, "NO_WITHHOLDING_ON_SALARY", fmt.Sprintf(
			"給与収入（%d円）がありますが源泉徴収税額が0円です。源泉徴収票を確認してください",
			in.SalaryRevenue.Yen()))
	}

	// 10. 還付額が前払税額の合計を超過
	if result.TaxDue < 0 && -result.TaxDue > result.TotalWithheld {
		add(SanityError, "REFUND_EXCEEDS_WITHHELD", fmt.Sprintf(
			"還付額（%d円）が源泉徴収+予定納税の合計（%d円）を超過しています",
			(-result.TaxDue).Yen(), result.TotalWithheld.Yen()))
	}

	res := &SanityCheckResult{Items: items}
	for _, it := range items {
		switch it.Severity {
		case SanityError:
			res.ErrorCount++
		case SanityWarning:
			res.WarningCount++
		}
	}
	res.Passed = res.ErrorCount == 0
	return res
}
