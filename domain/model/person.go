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
	Income                 Money // 年間の合計所得金額
	Disability             DisabilityKind
	Cohabiting             bool
	DirectAscendant        bool // 本人または配偶者の直系尊属か (同居老親等58万の要件)
	OtherTaxpayerDependent bool // 他の納税者の扶養親族に該当する (二重控除防止)
}

// Year は所属する課税年度を返す (YearScoped 契約)。
func (d *Dependent) Year() FiscalYear { return d.FiscalYear }

// Validate は FiscalYear が設定されている場合、年度の妥当性と生年月日の整合も
// 検証する (0 は純粋な計算入力として許容し、永続化時は usecase が非0を保証する)。
func (d *Dependent) Validate() error {
	if d.FiscalYear != 0 {
		if err := d.FiscalYear.Validate(); err != nil {
			return err
		}
		if !d.BirthDate.IsZero() && d.BirthDate.After(d.FiscalYear.End()) {
			return apperrors.New(apperrors.CodeBadRequest, "扶養親族の生年月日が課税年度末より後です")
		}
	}
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
	Income                 Money // 年間の合計所得金額
	Disability             DisabilityKind
	Cohabiting             bool
	OtherTaxpayerDependent bool // 他の納税者の扶養親族に該当する
}

// Year は所属する課税年度を返す (YearScoped 契約)。
func (s *Spouse) Year() FiscalYear { return s.FiscalYear }

// Validate は FiscalYear が設定されている場合、年度と生年月日の整合も検証する。
func (s *Spouse) Validate() error {
	if s.FiscalYear != 0 {
		if err := s.FiscalYear.Validate(); err != nil {
			return err
		}
		if !s.BirthDate.IsZero() && s.BirthDate.After(s.FiscalYear.End()) {
			return apperrors.New(apperrors.CodeBadRequest, "配偶者の生年月日が課税年度末より後です")
		}
	}
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

func (s WidowStatus) Validate() error {
	switch s {
	case WidowNone, WidowWidow, WidowSingleParent:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な寡婦/ひとり親区分です: %q", string(s))
}
