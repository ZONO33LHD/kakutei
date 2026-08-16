package model

import (
	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// HousingKind は住宅取得等の区分。
type HousingKind string

const (
	HousingNewCustom      HousingKind = "new_custom"      // 注文新築
	HousingNewSubdivision HousingKind = "new_subdivision" // 分譲新築
	HousingResale         HousingKind = "resale"          // 買取再販
	HousingUsed           HousingKind = "used"            // 既存 (中古)
	HousingRenovation     HousingKind = "renovation"      // 増改築 (リフォーム)
)

// UsesNewConstructionTable はこの取得区分・性能区分の組み合わせが
// 「新築・買取再販」の限度額表を使うかを返す。
//
// 買取再販で新築の枠が使えるのは性能要件を満たす認定住宅等のみで、
// 一般 (その他) の買取再販は既存住宅として扱う (租税特別措置法第41条)。
// 増改築は限度額一律のためどちらの表も使わない。
func (k HousingKind) UsesNewConstructionTable(category policy.HousingCategory) bool {
	switch k {
	case HousingNewCustom, HousingNewSubdivision:
		return true
	case HousingResale:
		return category != policy.HousingGeneral
	}
	return false
}

func (k HousingKind) Validate() error {
	switch k {
	case HousingNewCustom, HousingNewSubdivision, HousingResale, HousingUsed, HousingRenovation:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な住宅取得区分です: %q", string(k))
}

// HousingLoanDetail は住宅ローン控除の明細1件。
//
// 中古住宅の購入とリフォームを同一ローンで行った場合 (重複適用) は、
// 同じ DualApplicationGroup を持つ複数明細として登録し、
// CostForProration の比率で年末残高を按分して控除額を計算する。
type HousingLoanDetail struct {
	ID                   int64
	FiscalYear           FiscalYear
	Kind                 HousingKind
	Category             policy.HousingCategory // 住宅性能区分
	MoveInDate           Date
	YearEndBalance       Money
	IsChildcareHousehold bool   // 子育て世帯・若者夫婦世帯 (借入限度額の上乗せ)
	HasPreR6Permit       bool   // R5以前の建築確認済み (一般住宅新築の特例)
	AcquisitionCost      Money  // 住宅取得対価等 (取得価格/増改築費用)。控除対象額の上限。0 = 未入力 (上限適用なし)
	DualApplicationGroup string // 重複適用グループID。空 = 単独明細
	CostForProration     Money  // 按分用コスト (購入価格 or リフォーム費用)
}

func (h *HousingLoanDetail) Validate() error {
	if err := h.Kind.Validate(); err != nil {
		return err
	}
	if !h.Category.Valid() {
		return apperrors.Newf(apperrors.CodeBadRequest, "不正な住宅性能区分です: %q", string(h.Category))
	}
	if h.MoveInDate.IsZero() {
		return apperrors.New(apperrors.CodeBadRequest, "入居日は必須です")
	}
	if h.YearEndBalance < 0 || !h.YearEndBalance.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "年末残高が不正です: %d", h.YearEndBalance.Yen())
	}
	if h.CostForProration < 0 || !h.CostForProration.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "按分用コストが不正です: %d", h.CostForProration.Yen())
	}
	if h.AcquisitionCost < 0 || !h.AcquisitionCost.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "住宅取得対価等が不正です: %d", h.AcquisitionCost.Yen())
	}
	return nil
}
