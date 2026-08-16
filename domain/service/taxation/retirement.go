package taxation

import (
	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// RetirementIncomeInput は退職所得の計算入力。
type RetirementIncomeInput struct {
	SeverancePay           model.Money // 退職手当等の収入金額
	YearsOfService         int         // 勤続年数 (1年未満切上げ)
	IsOfficer              bool        // 役員等か (勤続5年以下は1/2適用なし)
	IsDisabilityRetirement bool        // 障害退職か (控除+100万)
}

// RetirementIncomeResult は退職所得の計算結果。
type RetirementIncomeResult struct {
	SeverancePay            model.Money
	RetirementDeduction     model.Money // 退職所得控除額
	TaxableRetirementIncome model.Money // 退職所得 (1/2適用後)
	YearsOfService          int
	IsOfficer               bool
	HalfTaxationApplied     bool // 1/2課税が全額に適用されたか
}

// RetirementIncome は退職所得を計算する (所得税法第30条)。
//
//   - 退職所得控除: 勤続20年以下 40万×年数 (最低80万)、20年超 800万+70万×超過年数
//   - 役員等で勤続5年以下: 1/2課税なし
//   - 一般で勤続5年以下 (令和4年改正): 控除後300万以下は1/2、300万超の部分は1/2なし
func RetirementIncome(in RetirementIncomeInput) (*RetirementIncomeResult, error) {
	if in.SeverancePay < 0 || !in.SeverancePay.ValidateAmountRange() {
		return nil, apperrors.Newf(apperrors.CodeBadRequest, "退職手当等の金額が不正です: %d", in.SeverancePay.Yen())
	}
	if in.YearsOfService <= 0 || in.YearsOfService > 100 {
		return nil, apperrors.Newf(apperrors.CodeBadRequest, "勤続年数が不正です: %d", in.YearsOfService)
	}

	deduction := retirementDeduction(in.YearsOfService)
	if in.IsDisabilityRetirement {
		deduction += policy.RetirementDeductionDisabilityAdd
	}

	excess := (in.SeverancePay - deduction).ClampNonNegative()
	taxable, halfApplied := applyRetirementHalfTaxation(excess, in.YearsOfService, in.IsOfficer)

	return &RetirementIncomeResult{
		SeverancePay:            in.SeverancePay,
		RetirementDeduction:     deduction,
		TaxableRetirementIncome: taxable,
		YearsOfService:          in.YearsOfService,
		IsOfficer:               in.IsOfficer,
		HalfTaxationApplied:     halfApplied,
	}, nil
}

// retirementDeduction は退職所得控除額を計算する。
func retirementDeduction(years int) model.Money {
	if years <= 20 {
		return model.Money(policy.RetirementDeductionPerYearUnder20 * int64(years)).
			Max(policy.RetirementDeductionMin)
	}
	return model.Money(policy.RetirementDeductionBase20 +
		policy.RetirementDeductionPerYearOver20*int64(years-20))
}

// applyRetirementHalfTaxation は 1/2 課税の適用可否を判定して課税退職所得を返す。
func applyRetirementHalfTaxation(excess model.Money, years int, isOfficer bool) (model.Money, bool) {
	shortService := years <= policy.RetirementOfficerShortServiceYears
	switch {
	case isOfficer && shortService:
		// 役員等の短期退職: 1/2適用なし
		return excess, false
	case !isOfficer && shortService:
		// 一般の短期退職 (令和4年改正): 300万以下は1/2、超過分はそのまま
		if excess <= policy.RetirementShortServiceHalfLimit {
			return excess / 2, true
		}
		return model.Money(policy.RetirementShortServiceHalfLimit)/2 +
			(excess - policy.RetirementShortServiceHalfLimit), false
	default:
		return excess / 2, true
	}
}
