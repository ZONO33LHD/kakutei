package model

import (
	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

// AccountCode は勘定科目コード (4桁数字)。
//
// コード体系:
//
//	1xxx: 資産 / 2xxx: 負債 / 3xxx: 純資産 / 4xxx: 収益 / 5xxx: 費用
type AccountCode string

func (c AccountCode) Validate() error {
	if len(c) != 4 {
		return apperrors.Newf(apperrors.CodeBadRequest, "勘定科目コードは4桁の数字です: %q", string(c))
	}
	for _, r := range c {
		if r < '0' || r > '9' {
			return apperrors.Newf(apperrors.CodeBadRequest, "勘定科目コードは4桁の数字です: %q", string(c))
		}
	}
	return nil
}

// AccountCategory は勘定科目の5分類。
type AccountCategory string

const (
	CategoryAsset     AccountCategory = "asset"
	CategoryLiability AccountCategory = "liability"
	CategoryEquity    AccountCategory = "equity"
	CategoryRevenue   AccountCategory = "revenue"
	CategoryExpense   AccountCategory = "expense"
)

func (c AccountCategory) Validate() error {
	switch c {
	case CategoryAsset, CategoryLiability, CategoryEquity, CategoryRevenue, CategoryExpense:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な勘定科目分類です: %q", string(c))
}

// NormalSide は正常残高の側を返す (資産・費用 = 借方、負債・純資産・収益 = 貸方)。
func (c AccountCategory) NormalSide() EntrySide {
	if c == CategoryAsset || c == CategoryExpense {
		return SideDebit
	}
	return SideCredit
}

// ConsumptionTaxCategory は勘定科目の既定の消費税区分。
type ConsumptionTaxCategory string

const (
	ConsumptionTaxable    ConsumptionTaxCategory = "taxable"      // 課税
	ConsumptionNonTaxable ConsumptionTaxCategory = "non_taxable"  // 非課税
	ConsumptionExempt     ConsumptionTaxCategory = "exempt"       // 免税
	ConsumptionOutOfScope ConsumptionTaxCategory = "out_of_scope" // 不課税
)

func (c ConsumptionTaxCategory) Validate() error {
	switch c {
	case ConsumptionTaxable, ConsumptionNonTaxable, ConsumptionExempt, ConsumptionOutOfScope:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な消費税区分です: %q", string(c))
}

// Account は勘定科目マスタの1件。
type Account struct {
	Code        AccountCode
	Name        string
	Category    AccountCategory
	SubCategory string                 // 補助分類 (current_asset 等)。空 = 未分類
	TaxCategory ConsumptionTaxCategory // 既定の消費税区分。空 = 対象外 (残高科目等)
	IsActive    bool
	SortOrder   int
}

func (a *Account) Validate() error {
	if err := a.Code.Validate(); err != nil {
		return err
	}
	if a.Name == "" {
		return apperrors.New(apperrors.CodeBadRequest, "勘定科目名は必須です")
	}
	if err := a.Category.Validate(); err != nil {
		return err
	}
	if a.TaxCategory != "" {
		if err := a.TaxCategory.Validate(); err != nil {
			return err
		}
	}
	return nil
}
