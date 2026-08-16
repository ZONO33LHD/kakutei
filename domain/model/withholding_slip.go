package model

import "github.com/ZONO33LHD/kakutei/domain/apperrors"

// WithholdingSlip は給与所得の源泉徴収票 1 枚分。
// 複数の勤務先がある場合は複数レコードとして保存し、申告時に合算する。
type WithholdingSlip struct {
	ID         int64
	FiscalYear FiscalYear
	PayerName  string // 支払者名。空 = 未設定

	PaymentAmount   Money // 支払金額 (給与収入)
	WithheldTax     Money // 源泉徴収税額
	SocialInsurance Money // 社会保険料等の金額

	// 保険料控除の内訳 (年末調整済みの場合に転記)
	LifeInsuranceGeneralNew  Money // 新生命保険料
	LifeInsuranceGeneralOld  Money // 旧生命保険料
	LifeInsuranceMedicalCare Money // 介護医療保険料
	LifeInsuranceAnnuityNew  Money // 新個人年金保険料
	LifeInsuranceAnnuityOld  Money // 旧個人年金保険料
	EarthquakeInsurance      Money // 地震保険料
	OldLongTermInsurance     Money // 旧長期損害保険料
	NationalPensionPremium   Money // 国民年金保険料等

	HousingLoanDeduction Money  // 住宅借入金等特別控除の額 (年末調整分)
	SourceFile           string // 読み取り元ファイル。空 = 未設定
}

// Validate は源泉徴収票の自己検証を行う。
func (w *WithholdingSlip) Validate() error {
	if err := w.FiscalYear.Validate(); err != nil {
		return err
	}
	amounts := []struct {
		name   string
		amount Money
	}{
		{"支払金額", w.PaymentAmount}, {"源泉徴収税額", w.WithheldTax}, {"社会保険料", w.SocialInsurance},
		{"新生命保険料", w.LifeInsuranceGeneralNew}, {"旧生命保険料", w.LifeInsuranceGeneralOld},
		{"介護医療保険料", w.LifeInsuranceMedicalCare}, {"新個人年金保険料", w.LifeInsuranceAnnuityNew},
		{"旧個人年金保険料", w.LifeInsuranceAnnuityOld}, {"地震保険料", w.EarthquakeInsurance},
		{"旧長期損害保険料", w.OldLongTermInsurance}, {"国民年金保険料", w.NationalPensionPremium},
		{"住宅借入金等特別控除", w.HousingLoanDeduction},
	}
	for _, v := range amounts {
		if v.amount < 0 || !v.amount.ValidateAmountRange() {
			return apperrors.Newf(apperrors.CodeBadRequest, "源泉徴収票の%sが不正です: %d", v.name, v.amount.Yen())
		}
	}
	if w.WithheldTax > w.PaymentAmount {
		return apperrors.New(apperrors.CodeBadRequest, "源泉徴収税額が支払金額を超えています")
	}
	return nil
}
