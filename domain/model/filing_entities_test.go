package model

import "testing"

func date(t *testing.T, s string) Date {
	t.Helper()
	d, err := ParseDate(s)
	if err != nil {
		t.Fatalf("ParseDate(%q): %v", s, err)
	}
	return d
}

func TestDependentValidate(t *testing.T) {
	valid := Dependent{Name: "子", Relationship: "子", BirthDate: date(t, "2010-01-01"), Cohabiting: true}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid dependent がエラー: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Dependent)
	}{
		{"名前なし", func(d *Dependent) { d.Name = "" }},
		{"生年月日なし", func(d *Dependent) { d.BirthDate = Date{} }},
		{"負の所得", func(d *Dependent) { d.Income = -1 }},
		{"不正な障害区分", func(d *Dependent) { d.Disability = "severe" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := valid
			tt.mutate(&bad)
			if err := bad.Validate(); err == nil {
				t.Error("エラーになるべき")
			}
		})
	}
}

func TestSpouseValidate(t *testing.T) {
	valid := Spouse{Name: "花子", BirthDate: date(t, "1990-05-01"), Cohabiting: true}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid spouse がエラー: %v", err)
	}
	bad := valid
	bad.Name = ""
	if err := bad.Validate(); err == nil {
		t.Error("名前なしはエラー")
	}
	bad2 := valid
	bad2.Income = -100
	if err := bad2.Validate(); err == nil {
		t.Error("負の所得はエラー")
	}
	bad3 := valid
	bad3.BirthDate = Date{}
	if err := bad3.Validate(); err == nil {
		t.Error("生年月日なしはエラー")
	}
}

func TestWidowStatusValidate(t *testing.T) {
	for _, s := range []WidowStatus{WidowNone, WidowWidow, WidowSingleParent} {
		if err := s.Validate(); err != nil {
			t.Errorf("%q は valid: %v", s, err)
		}
	}
	if err := WidowStatus("divorced").Validate(); err == nil {
		t.Error("未定義区分はエラー")
	}
}

func TestDonationRecordValidate(t *testing.T) {
	valid := DonationRecord{
		Kind: DonationPolitical, RecipientName: "政党X", Amount: 10_000, Date: date(t, "2025-06-01"),
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid donation がエラー: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*DonationRecord)
	}{
		{"不正な種別", func(d *DonationRecord) { d.Kind = "charity" }},
		{"寄附先なし", func(d *DonationRecord) { d.RecipientName = "" }},
		{"金額ゼロ", func(d *DonationRecord) { d.Amount = 0 }},
		{"日付なし", func(d *DonationRecord) { d.Date = Date{} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := valid
			tt.mutate(&bad)
			if err := bad.Validate(); err == nil {
				t.Error("エラーになるべき")
			}
		})
	}
}

func TestDonationKindCreditSelectable(t *testing.T) {
	selectable := []DonationKind{DonationPolitical, DonationNPO, DonationPublicInterest}
	for _, k := range selectable {
		if !k.CreditSelectable() {
			t.Errorf("%q は税額控除選択可であるべき", k)
		}
	}
	for _, k := range []DonationKind{DonationSpecified, DonationOther} {
		if k.CreditSelectable() {
			t.Errorf("%q は所得控除のみであるべき", k)
		}
	}
}

func TestFurusatoDonationValidate(t *testing.T) {
	valid := FurusatoDonation{Municipality: "○○市", Amount: 10_000, Date: date(t, "2025-06-01")}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid furusato がエラー: %v", err)
	}
	bad := valid
	bad.Municipality = ""
	if err := bad.Validate(); err == nil {
		t.Error("自治体名なしはエラー")
	}
	bad2 := valid
	bad2.Amount = 0
	if err := bad2.Validate(); err == nil {
		t.Error("金額ゼロはエラー")
	}
	bad3 := valid
	bad3.Date = Date{}
	if err := bad3.Validate(); err == nil {
		t.Error("日付なしはエラー")
	}
}

func TestHousingLoanDetailValidate(t *testing.T) {
	valid := HousingLoanDetail{
		Kind: HousingUsed, Category: "general",
		MoveInDate: date(t, "2025-04-01"), YearEndBalance: 10_000_000,
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid housing loan がエラー: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*HousingLoanDetail)
	}{
		{"不正な取得区分", func(h *HousingLoanDetail) { h.Kind = "castle" }},
		{"不正な性能区分", func(h *HousingLoanDetail) { h.Category = "mansion" }},
		{"入居日なし", func(h *HousingLoanDetail) { h.MoveInDate = Date{} }},
		{"負の残高", func(h *HousingLoanDetail) { h.YearEndBalance = -1 }},
		{"負の按分コスト", func(h *HousingLoanDetail) { h.CostForProration = -1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := valid
			tt.mutate(&bad)
			if err := bad.Validate(); err == nil {
				t.Error("エラーになるべき")
			}
		})
	}
}

func TestLifeInsurancePremiumsValidate(t *testing.T) {
	valid := LifeInsurancePremiums{GeneralNew: 50_000, MedicalCare: 30_000}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid premiums がエラー: %v", err)
	}
	if valid.IsZero() {
		t.Error("入力ありは IsZero = false")
	}
	if !(&LifeInsurancePremiums{}).IsZero() {
		t.Error("全区分0は IsZero = true")
	}
	bad := LifeInsurancePremiums{AnnuityOld: -1}
	if err := bad.Validate(); err == nil {
		t.Error("負の保険料はエラー")
	}
}

func TestDisabilityKindValidate(t *testing.T) {
	for _, k := range []DisabilityKind{DisabilityNone, DisabilityGeneral, DisabilitySpecial, DisabilitySpecialCohabiting} {
		if err := k.Validate(); err != nil {
			t.Errorf("%q は valid: %v", k, err)
		}
	}
	if err := DisabilityKind("mild").Validate(); err == nil {
		t.Error("未定義区分はエラー")
	}
}
