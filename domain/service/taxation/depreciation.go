package taxation

import (
	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// validateDepreciationParams は減価償却の共通パラメータを検証する。
func validateDepreciationParams(businessUseRatioPct, months int) error {
	if businessUseRatioPct <= 0 || businessUseRatioPct > 100 {
		return apperrors.Newf(apperrors.CodeBadRequest, "事業専用割合は 1〜100%% です: %d", businessUseRatioPct)
	}
	if months <= 0 || months > policy.MonthsPerYear {
		return apperrors.Newf(apperrors.CodeBadRequest, "使用月数は 1〜12 です: %d", months)
	}
	return nil
}

// StraightLineDepreciation は定額法の年間償却費を計算する。
//
//	償却費 = 取得価額 ÷ 耐用年数 × 事業専用割合 × 使用月数/12
//
// businessUseRatioPct は 1〜100 (%)、months は当年の使用月数 (1〜12)。
func StraightLineDepreciation(acquisitionCost model.Money, usefulLife, businessUseRatioPct, months int) (model.Money, error) {
	if acquisitionCost < 0 || !acquisitionCost.ValidateAmountRange() {
		return 0, apperrors.Newf(apperrors.CodeBadRequest, "取得価額が不正です: %d", acquisitionCost.Yen())
	}
	if usefulLife <= 0 || usefulLife > 100 {
		return 0, apperrors.Newf(apperrors.CodeBadRequest, "耐用年数が不正です: %d", usefulLife)
	}
	if err := validateDepreciationParams(businessUseRatioPct, months); err != nil {
		return 0, err
	}
	annual := acquisitionCost.MulDiv(1, int64(usefulLife))
	return annual.MulDiv(int64(businessUseRatioPct)*int64(months),
		policy.BusinessRatioDenomPct*policy.MonthsPerYear), nil
}

// DecliningBalanceDepreciation は定率法の年間償却費を計算する。
//
//	償却費 = 期首帳簿価額 × 償却率 (千分率) × 事業専用割合 × 使用月数/12
//
// decliningRatePerMille は千分率の償却率 (例: 500 = 0.500)。
func DecliningBalanceDepreciation(bookValue model.Money, decliningRatePerMille, businessUseRatioPct, months int) (model.Money, error) {
	if bookValue < 0 || !bookValue.ValidateAmountRange() {
		return 0, apperrors.Newf(apperrors.CodeBadRequest, "帳簿価額が不正です: %d", bookValue.Yen())
	}
	if decliningRatePerMille <= 0 || decliningRatePerMille > policy.DecliningRateDenominator {
		return 0, apperrors.Newf(apperrors.CodeBadRequest, "償却率 (千分率) が不正です: %d", decliningRatePerMille)
	}
	if err := validateDepreciationParams(businessUseRatioPct, months); err != nil {
		return 0, err
	}
	return bookValue.MulDiv(
		int64(decliningRatePerMille)*int64(businessUseRatioPct)*int64(months),
		policy.DecliningRateDenominator*policy.BusinessRatioDenomPct*policy.MonthsPerYear), nil
}
