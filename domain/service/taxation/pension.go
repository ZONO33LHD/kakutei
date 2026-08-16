package taxation

import (
	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// PensionDeductionInput は公的年金等控除の計算入力。
type PensionDeductionInput struct {
	PensionRevenue model.Money // 公的年金等の収入金額
	IsOver65       bool        // 年度末時点で65歳以上か (税法上の年齢)
	OtherIncome    model.Money // 公的年金等以外の合計所得金額
}

// PensionDeductionResult は公的年金等控除の計算結果。
type PensionDeductionResult struct {
	PensionRevenue        model.Money
	DeductionAmount       model.Money // 公的年金等控除額
	TaxablePensionIncome  model.Money // 雑所得 (年金) = 収入 − 控除
	IsOver65              bool
	OtherIncomeAdjustment model.Money // 他所得による控除減額 (0 / 10万 / 20万)
}

// PensionDeduction は公的年金等控除を計算する (所得税法第35条)。
func PensionDeduction(in PensionDeductionInput) (*PensionDeductionResult, error) {
	if in.PensionRevenue < 0 || !in.PensionRevenue.ValidateAmountRange() {
		return nil, apperrors.Newf(apperrors.CodeBadRequest, "年金収入が不正です: %d", in.PensionRevenue.Yen())
	}
	if in.OtherIncome < 0 || !in.OtherIncome.ValidateAmountRange() {
		return nil, apperrors.Newf(apperrors.CodeBadRequest, "他所得が不正です: %d", in.OtherIncome.Yen())
	}
	if in.PensionRevenue == 0 {
		return &PensionDeductionResult{IsOver65: in.IsOver65}, nil
	}

	table := policy.PensionDeductionUnder65
	maxDeduction := model.Money(policy.PensionDeductionUnder65Max)
	if in.IsOver65 {
		table = policy.PensionDeductionOver65
		maxDeduction = policy.PensionDeductionOver65Max
	}

	deduction := maxDeduction // 全区分超 (1,000万超) は上限
	for _, b := range table {
		if in.PensionRevenue.Yen() <= b.Threshold {
			if b.RatePct == 0 {
				deduction = model.Money(b.Fixed)
			} else {
				deduction = in.PensionRevenue.MulDiv(b.RatePct, 100) + model.Money(b.Fixed)
			}
			break
		}
	}

	// 公的年金等以外の所得が1,000万超の場合の控除減額。
	// 減額は速算表の固定部分に対して適用され、控除額は年金収入を超えない
	// (年金収入以下にキャップすることで国税庁の3列テーブルと一致する)。
	var adjustment model.Money
	switch {
	case in.OtherIncome > policy.PensionOtherIncomeBracket2:
		adjustment = policy.PensionOtherIncomeAdjustment2
	case in.OtherIncome > policy.PensionOtherIncomeBracket1:
		adjustment = policy.PensionOtherIncomeAdjustment1
	}
	deduction = (deduction - adjustment).ClampNonNegative().Min(in.PensionRevenue)

	return &PensionDeductionResult{
		PensionRevenue:        in.PensionRevenue,
		DeductionAmount:       deduction,
		TaxablePensionIncome:  in.PensionRevenue - deduction,
		IsOver65:              in.IsOver65,
		OtherIncomeAdjustment: adjustment,
	}, nil
}
