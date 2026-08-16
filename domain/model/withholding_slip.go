package model

import "github.com/ZONO33LHD/kakutei/domain/apperrors"

// WithholdingSlip は給与所得の源泉徴収票 1 枚分。
// 複数の勤務先がある場合は複数レコードとして保存し、申告時に合算する。
type WithholdingSlip struct {
	ID         int64
	FiscalYear FiscalYear
	PayerName  string // 支払者名。空 = 未設定

	PaymentAmount   Money // 支払金額 (給与収入)
	WithheldTax     Money
	SocialInsurance Money

	// 保険料控除の内訳 (年末調整済みの場合に転記)
	LifeInsuranceGeneralNew  Money
	LifeInsuranceGeneralOld  Money
	LifeInsuranceMedicalCare Money
	LifeInsuranceAnnuityNew  Money
	LifeInsuranceAnnuityOld  Money
	EarthquakeInsurance      Money
	OldLongTermInsurance     Money
	NationalPensionPremium   Money

	HousingLoanDeduction Money  // 住宅借入金等特別控除の額 (年末調整分)
	SourceFile           string // 読み取り元ファイル。空 = 未設定
}

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

// Year は所属する課税年度を返す (YearScoped 契約)。
func (w *WithholdingSlip) Year() FiscalYear { return w.FiscalYear }
