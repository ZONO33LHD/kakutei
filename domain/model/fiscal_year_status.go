package model

import (
	"time"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

// FiscalYearState は年度の開閉状態。
type FiscalYearState string

const (
	FiscalYearOpen   FiscalYearState = "open"   // 記帳可能
	FiscalYearClosed FiscalYearState = "closed" // 締め済み (仕訳の追加・訂正不可)
)

func (s FiscalYearState) Validate() error {
	if s == FiscalYearOpen || s == FiscalYearClosed {
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な年度状態です: %q", string(s))
}

// FiscalYearStatus は年度管理レコード。
type FiscalYearStatus struct {
	Year      FiscalYear
	State     FiscalYearState
	CreatedAt time.Time
}

func (f *FiscalYearStatus) Validate() error {
	if err := f.Year.Validate(); err != nil {
		return err
	}
	return f.State.Validate()
}
