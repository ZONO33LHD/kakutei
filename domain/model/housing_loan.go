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

// Validate は定義済みの区分かを検証する。
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
	MoveInDate           Date                   // 入居日
	YearEndBalance       Money                  // 年末残高
	IsNewConstruction    bool                   // 新築かどうか (限度額テーブルの選択に使用)
	IsChildcareHousehold bool                   // 子育て世帯・若者夫婦世帯 (R6-R7 の上限維持)
	HasPreR6Permit       bool                   // R5以前の建築確認済み (一般住宅新築の特例)
	AcquisitionCost      Money                  // 住宅取得対価等 (取得価格/増改築費用)。控除対象額の上限。0 = 未入力 (上限適用なし)
	DualApplicationGroup string                 // 重複適用グループID。空 = 単独明細
	CostForProration     Money                  // 按分用コスト (購入価格 or リフォーム費用)
}

// Validate は住宅ローン控除明細の自己検証を行う。
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
