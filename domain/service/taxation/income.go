// Package taxation は所得税・消費税等の税額計算を行うドメインサービス群。
//
// 全て純粋な計算ロジックであり、リポジトリや外部 I/O に依存しない。
// 金額は model.Money (円単位の整数)、端数処理は国税庁の規定に従う。
// 税制定数は domain/policy (令和7・8年分) を参照する。
package taxation

import (
	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// validateFiscalYear は課税年度が税制定数のサポート範囲かを検証する。
// 全ての税計算サービスの入口で呼び、未対応年度への誤適用を防ぐ。
func validateFiscalYear(year model.FiscalYear) error {
	if err := year.Validate(); err != nil {
		return err
	}
	if !policy.SupportsFiscalYear(int(year)) {
		return apperrors.Newf(apperrors.CodeBadRequest,
			"課税年度 %d は未対応です (対応年度: %d〜%d)",
			int(year), policy.MinSupportedFiscalYear, policy.MaxSupportedFiscalYear)
	}
	return nil
}

// SalaryIncome は給与収入から給与所得金額を計算する (所得税法第28条)。
//
// 国税庁の速算表に従い、収入 660 万円以下は A = 収入÷4 (千円未満切捨) を
// 用いる端数規定を適用する。令和8年分は最低保障74万の特例速算 (収入220万未満)
// が優先される。戻り値は給与所得金額 (控除後、非負)。
func SalaryIncome(revenue model.Money, year model.FiscalYear) model.Money {
	if revenue <= 0 {
		return 0
	}
	if int(year) >= 2026 {
		if income, ok := salaryIncome2026Special(revenue); ok {
			return income
		}
		// 収入220万以上は令和7年分と同じ速算 (下の 190万以下の分岐には到達しない)
	}
	switch {
	case revenue <= policy.SalaryDeductionBracket1:
		return (revenue - policy.SalaryDeductionMin).ClampNonNegative()
	case revenue <= policy.SalaryDeductionBracket2:
		a := quarterRounded(revenue)
		return a.MulDiv(policy.SalaryIncomeFactor2Num, policy.SalaryIncomeFactor2Denom) - policy.SalaryIncomeAdjust2
	case revenue <= policy.SalaryDeductionBracket3:
		a := quarterRounded(revenue)
		return a.MulDiv(policy.SalaryIncomeFactor3Num, policy.SalaryIncomeFactor3Denom) - policy.SalaryIncomeAdjust3
	case revenue <= policy.SalaryDeductionBracket4:
		return revenue.MulDiv(policy.SalaryIncomeFactor4Num, policy.SalaryIncomeFactor4Denom) - policy.SalaryIncomeAdjust4
	default:
		return revenue - policy.SalaryDeductionMax
	}
}

// salaryIncome2026Special は令和8・9年分の特例速算 (収入220万未満) を適用する。
func salaryIncome2026Special(revenue model.Money) (model.Money, bool) {
	switch {
	case revenue < policy.Salary2026Step0Max:
		return 0, true
	case revenue < policy.Salary2026Step1Max:
		return revenue - policy.SalaryDeductionMin2026, true
	case revenue < policy.Salary2026Step2Max:
		return policy.Salary2026Step2Income, true
	case revenue < policy.Salary2026Step3Max:
		return policy.Salary2026Step3Income, true
	case revenue < policy.Salary2026Step4Max:
		return policy.Salary2026Step4Income, true
	default:
		return 0, false
	}
}

// quarterRounded は速算表の A 値 (収入÷4、千円未満切捨) を返す。
func quarterRounded(revenue model.Money) model.Money {
	return (revenue / 4).RoundDownTo(policy.SalaryQuarterRoundingUnit)
}

// BusinessIncomeResult は事業所得の計算結果。
type BusinessIncomeResult struct {
	// Income は青色申告特別控除適用後の事業所得。赤字の場合は負値 (損益通算対象)。
	Income model.Money
	// EffectiveBlueReturnDeduction は実際に適用された青色申告特別控除額。
	// 控除は事業利益 (収入−経費) を上限とする (租税特別措置法第25条の2)。
	EffectiveBlueReturnDeduction model.Money
	// Capped は控除が利益上限で自動調整されたかどうか。
	Capped bool
}

// BusinessIncome は事業所得を計算する。
//
// 青色申告特別控除は事業利益を上限として自動調整する。
// 赤字の場合は控除 0 とし、赤字額をそのまま返す (損益通算のため)。
func BusinessIncome(revenue, expenses, blueReturnDeduction model.Money) BusinessIncomeResult {
	profit := revenue - expenses
	effective := blueReturnDeduction.Min(profit.ClampNonNegative())
	return BusinessIncomeResult{
		Income:                       profit - effective,
		EffectiveBlueReturnDeduction: effective,
		Capped:                       effective < blueReturnDeduction,
	}
}

// OneTimeIncome は一時所得の課税対象額を計算する (所得税法第34条)。
//
// 課税対象 = max(0, 総収入 − 収入を得るための支出 − 特別控除50万) × 1/2
// gross には支出控除後の金額を渡す。
func OneTimeIncome(gross model.Money) model.Money {
	return (gross - policy.OneTimeIncomeSpecialDeduction).ClampNonNegative() / 2
}

// ValidateBlueReturnDeduction は青色申告特別控除額が法定額かを検証する。
// 0 (白色申告) / 10万 (簡易帳簿) / 55万 (正規の簿記) / 65万 (e-Tax or 電子帳簿保存)。
func ValidateBlueReturnDeduction(amount model.Money) error {
	switch amount {
	case 0, policy.BlueReturnDeduction10, policy.BlueReturnDeduction55, policy.BlueReturnDeduction65:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest,
		"青色申告特別控除は 0 / 100,000 / 550,000 / 650,000 円のいずれかです: %d", amount.Yen())
}

// SalaryIncomeAdjustment は所得金額調整控除 (子ども・特別障害者等) を計算する
// (租税特別措置法第41条の3の11)。
//
// 給与収入 850万円超で以下のいずれかに該当する場合:
//   - 本人が特別障害者
//   - 23歳未満の扶養親族がいる (税法上の年齢、年度別の所得要件を満たす者)
//   - 特別障害者の同一生計配偶者・扶養親族がいる
//
// 控除額 = (min(給与収入, 1,000万) − 850万) × 10% (1円未満切上げ)。
// ※ 給与所得と公的年金等の双方がある場合の調整 (最大10万) は未対応。
func SalaryIncomeAdjustment(
	salaryRevenue model.Money, selfDisability model.DisabilityKind,
	spouse *model.Spouse, dependents []model.Dependent, year model.FiscalYear,
) model.Money {
	if salaryRevenue <= policy.SalaryAdjustmentThreshold {
		return 0
	}
	if !salaryAdjustmentEligible(selfDisability, spouse, dependents, year) {
		return 0
	}
	capped := salaryRevenue.Min(policy.SalaryAdjustmentRevenueCap)
	return ceilDiv((capped-policy.SalaryAdjustmentThreshold)*policy.SalaryAdjustmentRatePct, 100)
}

// salaryAdjustmentEligible は所得金額調整控除の人的要件を判定する。
func salaryAdjustmentEligible(
	selfDisability model.DisabilityKind, spouse *model.Spouse,
	dependents []model.Dependent, year model.FiscalYear,
) bool {
	isSpecial := func(k model.DisabilityKind) bool {
		return k == model.DisabilitySpecial || k == model.DisabilitySpecialCohabiting
	}
	if isSpecial(selfDisability) {
		return true
	}
	incomeMax := policy.DependentIncomeMaxFor(int(year))
	// 特別障害者の同一生計配偶者
	if spouse != nil && spouse.Income.Yen() <= incomeMax && isSpecial(spouse.Disability) {
		return true
	}
	yearEnd := year.End()
	for i := range dependents {
		dep := &dependents[i]
		if dep.Income.Yen() > incomeMax {
			continue // 扶養親族の所得要件
		}
		// 23歳未満の扶養親族 (16歳未満も含む) または特別障害者の扶養親族
		if dep.BirthDate.TaxAgeAt(yearEnd) < policy.SalaryAdjustmentChildAge || isSpecial(dep.Disability) {
			return true
		}
	}
	return false
}
