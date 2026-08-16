package model

import "github.com/ZONO33LHD/kakutei/domain/apperrors"

// LifeInsurancePremiums は生命保険料の3区分 (新旧制度) の年間支払額。
type LifeInsurancePremiums struct {
	GeneralNew  Money // 一般生命保険料 (新制度)
	GeneralOld  Money // 一般生命保険料 (旧制度)
	MedicalCare Money // 介護医療保険料 (新制度のみ)
	AnnuityNew  Money // 個人年金保険料 (新制度)
	AnnuityOld  Money // 個人年金保険料 (旧制度)
}

func (p *LifeInsurancePremiums) Validate() error {
	for _, v := range []struct {
		name   string
		amount Money
	}{
		{"一般 (新)", p.GeneralNew},
		{"一般 (旧)", p.GeneralOld},
		{"介護医療", p.MedicalCare},
		{"個人年金 (新)", p.AnnuityNew},
		{"個人年金 (旧)", p.AnnuityOld},
	} {
		if v.amount < 0 || !v.amount.ValidateAmountRange() {
			return apperrors.Newf(apperrors.CodeBadRequest,
				"生命保険料 %s の金額が不正です: %d", v.name, v.amount.Yen())
		}
	}
	return nil
}

func (p *LifeInsurancePremiums) IsZero() bool {
	return p.GeneralNew == 0 && p.GeneralOld == 0 && p.MedicalCare == 0 &&
		p.AnnuityNew == 0 && p.AnnuityOld == 0
}
