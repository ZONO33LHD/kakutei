package model

import "github.com/ZONO33LHD/kakutei/domain/apperrors"

// DepreciationMethod は減価償却の方法。
type DepreciationMethod string

const (
	DepreciationStraightLine     DepreciationMethod = "straight_line"
	DepreciationDecliningBalance DepreciationMethod = "declining_balance"
)

func (m DepreciationMethod) Validate() error {
	if m == DepreciationStraightLine || m == DepreciationDecliningBalance {
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な償却方法です: %q", string(m))
}

// FixedAsset は固定資産台帳の1件。
type FixedAsset struct {
	ID                      int64
	FiscalYear              FiscalYear
	Name                    string
	AcquisitionDate         Date
	AcquisitionCost         Money
	UsefulLife              int
	Method                  DepreciationMethod
	DecliningRatePerMille   int   // 定率法の償却率 (千分率)。定額法では0
	BusinessUseRatioPct     int   // 事業専用割合 (1〜100)
	AccumulatedDepreciation Money // 期首時点の償却累計額
	Memo                    string
}

func (a *FixedAsset) Validate() error {
	if err := a.FiscalYear.Validate(); err != nil {
		return err
	}
	if a.Name == "" {
		return apperrors.New(apperrors.CodeBadRequest, "資産名は必須です")
	}
	if a.AcquisitionDate.IsZero() {
		return apperrors.New(apperrors.CodeBadRequest, "取得日は必須です")
	}
	if a.AcquisitionCost <= 0 || !a.AcquisitionCost.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "取得価額が不正です: %d", a.AcquisitionCost.Yen())
	}
	if a.UsefulLife <= 0 || a.UsefulLife > 100 {
		return apperrors.Newf(apperrors.CodeBadRequest, "耐用年数が不正です: %d", a.UsefulLife)
	}
	if err := a.Method.Validate(); err != nil {
		return err
	}
	if a.Method == DepreciationDecliningBalance &&
		(a.DecliningRatePerMille <= 0 || a.DecliningRatePerMille > 1000) {
		return apperrors.New(apperrors.CodeBadRequest, "定率法の償却率 (千分率) は 1〜1000 で指定してください")
	}
	if a.BusinessUseRatioPct < 1 || a.BusinessUseRatioPct > 100 {
		return apperrors.Newf(apperrors.CodeBadRequest, "事業専用割合は 1〜100%% です: %d", a.BusinessUseRatioPct)
	}
	if a.AccumulatedDepreciation < 0 || a.AccumulatedDepreciation > a.AcquisitionCost {
		return apperrors.New(apperrors.CodeBadRequest, "償却累計額は 0 以上かつ取得価額以下です")
	}
	return nil
}

// BookValue は期首帳簿価額 (取得価額 − 償却累計額) を返す。
func (a *FixedAsset) BookValue() Money {
	return a.AcquisitionCost - a.AccumulatedDepreciation
}

// Year は所属する課税年度を返す (YearScoped 契約)。
func (a *FixedAsset) Year() FiscalYear { return a.FiscalYear }
