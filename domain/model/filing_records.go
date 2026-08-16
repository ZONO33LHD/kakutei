package model

import "github.com/ZONO33LHD/kakutei/domain/apperrors"

// MedicalExpense は医療費控除の明細1件。
type MedicalExpense struct {
	ID                     int64
	FiscalYear             FiscalYear
	Date                   Date
	PatientName            string // 受診者
	MedicalInstitution     string // 医療機関・薬局名
	Amount                 Money  // 支払医療費
	InsuranceReimbursement Money  // 保険等で補填される金額
	Description            string // 空 = 未設定
}

// Validate は医療費明細の自己検証を行う。
func (m *MedicalExpense) Validate() error {
	if err := m.FiscalYear.Validate(); err != nil {
		return err
	}
	if m.Date.IsZero() {
		return apperrors.New(apperrors.CodeBadRequest, "医療費の支払日は必須です")
	}
	if !m.FiscalYear.Contains(m.Date) {
		return apperrors.Newf(apperrors.CodeBadRequest,
			"医療費の支払日 %s が年度 %d の期間外です", m.Date, int(m.FiscalYear))
	}
	if m.PatientName == "" || m.MedicalInstitution == "" {
		return apperrors.New(apperrors.CodeBadRequest, "受診者名と医療機関名は必須です")
	}
	if m.Amount <= 0 || !m.Amount.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "医療費が不正です: %d", m.Amount.Yen())
	}
	if m.InsuranceReimbursement < 0 || m.InsuranceReimbursement > m.Amount {
		return apperrors.New(apperrors.CodeBadRequest, "保険補填額は 0 以上かつ医療費以下です")
	}
	return nil
}

// NetAmount は補填控除後の医療費を返す。
func (m *MedicalExpense) NetAmount() Money {
	return m.Amount - m.InsuranceReimbursement
}

// SocialInsuranceKind は社会保険料の種別。
type SocialInsuranceKind string

const (
	SocialInsuranceNationalHealth      SocialInsuranceKind = "national_health"       // 国民健康保険
	SocialInsuranceNationalPension     SocialInsuranceKind = "national_pension"      // 国民年金
	SocialInsuranceNationalPensionFund SocialInsuranceKind = "national_pension_fund" // 国民年金基金
	SocialInsuranceNursingCare         SocialInsuranceKind = "nursing_care"          // 介護保険
	SocialInsuranceLabor               SocialInsuranceKind = "labor_insurance"       // 労働保険
	SocialInsuranceOther               SocialInsuranceKind = "other"                 // その他
)

// Validate は定義済みの種別かを検証する。
func (k SocialInsuranceKind) Validate() error {
	switch k {
	case SocialInsuranceNationalHealth, SocialInsuranceNationalPension,
		SocialInsuranceNationalPensionFund, SocialInsuranceNursingCare,
		SocialInsuranceLabor, SocialInsuranceOther:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な社会保険料種別です: %q", string(k))
}

// SocialInsuranceItem は社会保険料の種別別内訳1件。
type SocialInsuranceItem struct {
	ID         int64
	FiscalYear FiscalYear
	Kind       SocialInsuranceKind
	Name       string // 保険者名等。空 = 未設定
	Amount     Money
}

// Validate は社会保険料内訳の自己検証を行う。
func (s *SocialInsuranceItem) Validate() error {
	if err := s.FiscalYear.Validate(); err != nil {
		return err
	}
	if err := s.Kind.Validate(); err != nil {
		return err
	}
	if s.Amount <= 0 || !s.Amount.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "社会保険料が不正です: %d", s.Amount.Yen())
	}
	return nil
}

// InsurancePolicyKind は保険契約の種別 (控除区分)。
type InsurancePolicyKind string

const (
	PolicyLifeGeneralNew  InsurancePolicyKind = "life_general_new"  // 一般生命保険 (新制度)
	PolicyLifeGeneralOld  InsurancePolicyKind = "life_general_old"  // 一般生命保険 (旧制度)
	PolicyLifeMedicalCare InsurancePolicyKind = "life_medical_care" // 介護医療保険
	PolicyLifeAnnuityNew  InsurancePolicyKind = "life_annuity_new"  // 個人年金 (新制度)
	PolicyLifeAnnuityOld  InsurancePolicyKind = "life_annuity_old"  // 個人年金 (旧制度)
	PolicyEarthquake      InsurancePolicyKind = "earthquake"        // 地震保険
	PolicyOldLongTerm     InsurancePolicyKind = "old_long_term"     // 旧長期損害保険
)

// Validate は定義済みの種別かを検証する。
func (k InsurancePolicyKind) Validate() error {
	switch k {
	case PolicyLifeGeneralNew, PolicyLifeGeneralOld, PolicyLifeMedicalCare,
		PolicyLifeAnnuityNew, PolicyLifeAnnuityOld, PolicyEarthquake, PolicyOldLongTerm:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な保険契約種別です: %q", string(k))
}

// InsurancePolicy は保険契約 (支払保険料の控除区分別内訳) 1件。
type InsurancePolicy struct {
	ID          int64
	FiscalYear  FiscalYear
	Kind        InsurancePolicyKind
	CompanyName string
	Premium     Money // 年間支払保険料
}

// Validate は保険契約の自己検証を行う。
func (p *InsurancePolicy) Validate() error {
	if err := p.FiscalYear.Validate(); err != nil {
		return err
	}
	if err := p.Kind.Validate(); err != nil {
		return err
	}
	if p.CompanyName == "" {
		return apperrors.New(apperrors.CodeBadRequest, "保険会社名は必須です")
	}
	if p.Premium <= 0 || !p.Premium.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "支払保険料が不正です: %d", p.Premium.Yen())
	}
	return nil
}

// BusinessWithholding は事業所得の源泉徴収 (取引先別) 1件。
type BusinessWithholding struct {
	ID          int64
	FiscalYear  FiscalYear
	ClientName  string
	GrossAmount Money // 支払金額
	WithheldTax Money // 源泉徴収税額
}

// Validate は事業源泉徴収の自己検証を行う。
func (b *BusinessWithholding) Validate() error {
	if err := b.FiscalYear.Validate(); err != nil {
		return err
	}
	if b.ClientName == "" {
		return apperrors.New(apperrors.CodeBadRequest, "取引先名は必須です")
	}
	if b.GrossAmount <= 0 || !b.GrossAmount.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "支払金額が不正です: %d", b.GrossAmount.Yen())
	}
	if b.WithheldTax < 0 || b.WithheldTax > b.GrossAmount {
		return apperrors.New(apperrors.CodeBadRequest, "源泉徴収税額は 0 以上かつ支払金額以下です")
	}
	return nil
}

// LossCarryforward は純損失の繰越控除の1件 (青色申告、3年繰越)。
type LossCarryforward struct {
	ID         int64
	FiscalYear FiscalYear // 適用する年度
	LossYear   FiscalYear // 損失が発生した年度
	Amount     Money      // 繰越損失額
	UsedAmount Money      // 充当済みの額
}

// maxLossCarryforwardYears は純損失の繰越可能年数 (所得税法第70条)。
const maxLossCarryforwardYears = 3

// Validate は繰越損失の自己検証を行う。
func (l *LossCarryforward) Validate() error {
	if err := l.FiscalYear.Validate(); err != nil {
		return err
	}
	if err := l.LossYear.Validate(); err != nil {
		return err
	}
	if l.LossYear >= l.FiscalYear {
		return apperrors.New(apperrors.CodeBadRequest, "損失発生年度は適用年度より前である必要があります")
	}
	if int(l.FiscalYear-l.LossYear) > maxLossCarryforwardYears {
		return apperrors.Newf(apperrors.CodeBadRequest,
			"純損失の繰越は%d年までです (損失年度: %d)", maxLossCarryforwardYears, int(l.LossYear))
	}
	if l.Amount <= 0 || !l.Amount.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "繰越損失額が不正です: %d", l.Amount.Yen())
	}
	if l.UsedAmount < 0 || l.UsedAmount > l.Amount {
		return apperrors.New(apperrors.CodeBadRequest, "充当済み額は 0 以上かつ繰越損失額以下です")
	}
	return nil
}

// Remaining は未充当の繰越損失額を返す。
func (l *LossCarryforward) Remaining() Money {
	return l.Amount - l.UsedAmount
}

// OtherIncomeKind はその他所得の種別。
type OtherIncomeKind string

const (
	OtherIncomeMiscellaneous OtherIncomeKind = "miscellaneous"          // 雑所得 (年金以外)
	OtherIncomeDividend      OtherIncomeKind = "dividend_comprehensive" // 配当所得 (総合課税)
	OtherIncomeOneTime       OtherIncomeKind = "one_time"               // 一時所得
)

// Validate は定義済みの種別かを検証する。
func (k OtherIncomeKind) Validate() error {
	switch k {
	case OtherIncomeMiscellaneous, OtherIncomeDividend, OtherIncomeOneTime:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正なその他所得種別です: %q", string(k))
}

// OtherIncome はその他所得 (雑/配当/一時) の1件。
type OtherIncome struct {
	ID          int64
	FiscalYear  FiscalYear
	Kind        OtherIncomeKind
	Description string
	Revenue     Money // 収入金額
	Expenses    Money // 必要経費 (収入を得るための支出)
	WithheldTax Money // 源泉徴収税額
	PayerName   string
}

// Validate はその他所得の自己検証を行う。
func (o *OtherIncome) Validate() error {
	if err := o.FiscalYear.Validate(); err != nil {
		return err
	}
	if err := o.Kind.Validate(); err != nil {
		return err
	}
	if o.Description == "" {
		return apperrors.New(apperrors.CodeBadRequest, "所得の内容は必須です")
	}
	if o.Revenue < 0 || !o.Revenue.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "収入金額が不正です: %d", o.Revenue.Yen())
	}
	if o.Expenses < 0 || !o.Expenses.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest, "必要経費が不正です: %d", o.Expenses.Yen())
	}
	if o.WithheldTax < 0 || o.WithheldTax > o.Revenue {
		return apperrors.New(apperrors.CodeBadRequest, "源泉徴収税額は 0 以上かつ収入金額以下です")
	}
	return nil
}

// NetIncome は必要経費控除後の所得金額を返す (負にはならない)。
func (o *OtherIncome) NetIncome() Money {
	return (o.Revenue - o.Expenses).ClampNonNegative()
}
