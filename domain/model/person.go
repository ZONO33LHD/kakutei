package model

import "github.com/ZONO33LHD/kakutei/domain/apperrors"

// DisabilityKind は障害者控除の区分。
type DisabilityKind string

const (
	DisabilityNone              DisabilityKind = "none"               // 該当なし
	DisabilityGeneral           DisabilityKind = "general"            // 一般障害者
	DisabilitySpecial           DisabilityKind = "special"            // 特別障害者
	DisabilitySpecialCohabiting DisabilityKind = "special_cohabiting" // 同居特別障害者
)

// Validate は定義済みの区分かを検証する。
func (k DisabilityKind) Validate() error {
	switch k {
	case DisabilityNone, DisabilityGeneral, DisabilitySpecial, DisabilitySpecialCohabiting:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な障害者区分です: %q", string(k))
}

// Dependent は扶養親族 (配偶者以外)。
type Dependent struct {
	ID                     int64
	FiscalYear             FiscalYear
	Name                   string
	Relationship           string // 続柄 (子/親 等)
	BirthDate              Date
	Income                 Money          // 年間の合計所得金額
	Disability             DisabilityKind // 障害者控除の区分
	Cohabiting             bool           // 同居しているか
	DirectAscendant        bool           // 本人または配偶者の直系尊属か (同居老親等58万の要件)
	OtherTaxpayerDependent bool           // 他の納税者の扶養親族に該当する (二重控除防止)
}

// Validate は扶養親族の自己検証を行う。
func (d *Dependent) Validate() error {
	if d.Name == "" {
		return apperrors.New(apperrors.CodeBadRequest, "扶養親族の氏名は必須です")
	}
	if d.BirthDate.IsZero() {
		return apperrors.New(apperrors.CodeBadRequest, "扶養親族の生年月日は必須です")
	}
	if d.Income < 0 || !d.Income.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "扶養親族の所得が不正です: %d", d.Income.Yen())
	}
	if d.Disability != "" {
		if err := d.Disability.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Spouse は配偶者情報。
type Spouse struct {
	ID                     int64
	FiscalYear             FiscalYear
	Name                   string
	BirthDate              Date
	Income                 Money          // 年間の合計所得金額
	Disability             DisabilityKind // 障害者控除の区分
	Cohabiting             bool
	OtherTaxpayerDependent bool // 他の納税者の扶養親族に該当する
}

// Validate は配偶者情報の自己検証を行う。
func (s *Spouse) Validate() error {
	if s.Name == "" {
		return apperrors.New(apperrors.CodeBadRequest, "配偶者の氏名は必須です")
	}
	if s.BirthDate.IsZero() {
		return apperrors.New(apperrors.CodeBadRequest, "配偶者の生年月日は必須です")
	}
	if s.Income < 0 || !s.Income.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "配偶者の所得が不正です: %d", s.Income.Yen())
	}
	if s.Disability != "" {
		if err := s.Disability.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// WidowStatus は寡婦/ひとり親控除の区分。
type WidowStatus string

const (
	WidowNone         WidowStatus = "none"          // 該当なし
	WidowWidow        WidowStatus = "widow"         // 寡婦
	WidowSingleParent WidowStatus = "single_parent" // ひとり親
)

// Validate は定義済みの区分かを検証する。
func (s WidowStatus) Validate() error {
	switch s {
	case WidowNone, WidowWidow, WidowSingleParent:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な寡婦/ひとり親区分です: %q", string(s))
}
