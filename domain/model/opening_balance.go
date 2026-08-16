package model

import "github.com/ZONO33LHD/kakutei/domain/apperrors"

// OpeningBalance は勘定科目の期首残高。
// 金額は科目の正常残高側の符号を正とする (資産・費用 = 借方残、負債・純資産・収益 = 貸方残)。
type OpeningBalance struct {
	ID          int64
	FiscalYear  FiscalYear
	AccountCode AccountCode
	Amount      Money
}

func (o *OpeningBalance) Validate() error {
	if err := o.FiscalYear.Validate(); err != nil {
		return err
	}
	if err := o.AccountCode.Validate(); err != nil {
		return err
	}
	if !o.Amount.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "期首残高が不正です: %d", o.Amount.Yen())
	}
	return nil
}
