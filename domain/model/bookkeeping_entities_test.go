package model

import "testing"

func TestFiscalYearStatusValidate(t *testing.T) {
	ok := FiscalYearStatus{Year: 2025, State: FiscalYearOpen}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid status がエラー: %v", err)
	}
	if err := (&FiscalYearStatus{Year: 2025, State: "archived"}).Validate(); err == nil {
		t.Error("未定義状態はエラー")
	}
	if err := (&FiscalYearStatus{Year: 7, State: FiscalYearOpen}).Validate(); err == nil {
		t.Error("不正年度はエラー")
	}
}

func TestOpeningBalanceValidate(t *testing.T) {
	ok := OpeningBalance{FiscalYear: 2025, AccountCode: "1001", Amount: 100_000}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid opening balance がエラー: %v", err)
	}
	bad := ok
	bad.AccountCode = "xx"
	if err := bad.Validate(); err == nil {
		t.Error("不正な科目コードはエラー")
	}
	bad2 := ok
	bad2.Amount = MaxAmount + 1
	if err := bad2.Validate(); err == nil {
		t.Error("範囲外の金額はエラー")
	}
}

func TestJournalAuditLogValidate(t *testing.T) {
	ok := JournalAuditLog{JournalID: 1, FiscalYear: 2025, Operation: AuditUpdate,
		BeforeSnapshot: "{}", AfterSnapshot: "{}"}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid audit log がエラー: %v", err)
	}
	del := JournalAuditLog{JournalID: 1, FiscalYear: 2025, Operation: AuditDelete, BeforeSnapshot: "{}"}
	if err := del.Validate(); err != nil {
		t.Errorf("削除ログは After なしで valid: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*JournalAuditLog)
	}{
		{"仕訳IDなし", func(l *JournalAuditLog) { l.JournalID = 0 }},
		{"不正な操作", func(l *JournalAuditLog) { l.Operation = "insert" }},
		{"Beforeなし", func(l *JournalAuditLog) { l.BeforeSnapshot = "" }},
		{"訂正でAfterなし", func(l *JournalAuditLog) { l.AfterSnapshot = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := ok
			tt.mutate(&bad)
			if err := bad.Validate(); err == nil {
				t.Error("エラーになるべき")
			}
		})
	}
}

func TestWithholdingSlipValidate(t *testing.T) {
	ok := WithholdingSlip{FiscalYear: 2025, PayerName: "株式会社X",
		PaymentAmount: 5_000_000, WithheldTax: 150_000, SocialInsurance: 700_000}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid slip がエラー: %v", err)
	}
	bad := ok
	bad.WithheldTax = -1
	if err := bad.Validate(); err == nil {
		t.Error("負の源泉徴収税額はエラー")
	}
}

func TestMedicalExpenseValidate(t *testing.T) {
	d, _ := ParseDate("2025-03-01")
	ok := MedicalExpense{FiscalYear: 2025, Date: d, PatientName: "本人",
		MedicalInstitution: "○○病院", Amount: 12_000, InsuranceReimbursement: 2_000}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid expense がエラー: %v", err)
	}
	if ok.NetAmount() != 10_000 {
		t.Errorf("NetAmount = %d", ok.NetAmount().Yen())
	}
	bad := ok
	bad.InsuranceReimbursement = 20_000
	if err := bad.Validate(); err == nil {
		t.Error("補填が医療費超過はエラー")
	}
	bad2 := ok
	bad2.PatientName = ""
	if err := bad2.Validate(); err == nil {
		t.Error("受診者名なしはエラー")
	}
}

func TestSocialInsuranceItemValidate(t *testing.T) {
	ok := SocialInsuranceItem{FiscalYear: 2025, Kind: SocialInsuranceNationalPension, Amount: 200_000}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid item がエラー: %v", err)
	}
	if err := (&SocialInsuranceItem{Kind: "company_pension", Amount: 1}).Validate(); err == nil {
		t.Error("未定義種別はエラー")
	}
	if err := (&SocialInsuranceItem{Kind: SocialInsuranceOther, Amount: 0}).Validate(); err == nil {
		t.Error("金額0はエラー")
	}
}

func TestInsurancePolicyValidate(t *testing.T) {
	ok := InsurancePolicy{FiscalYear: 2025, Kind: PolicyLifeGeneralNew, CompanyName: "○○生命", Premium: 60_000}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid policy がエラー: %v", err)
	}
	if err := (&InsurancePolicy{Kind: "car", CompanyName: "x", Premium: 1}).Validate(); err == nil {
		t.Error("未定義種別はエラー")
	}
	if err := (&InsurancePolicy{Kind: PolicyEarthquake, Premium: 1}).Validate(); err == nil {
		t.Error("会社名なしはエラー")
	}
}

func TestBusinessWithholdingValidate(t *testing.T) {
	ok := BusinessWithholding{FiscalYear: 2025, ClientName: "取引先A", GrossAmount: 1_100_000, WithheldTax: 102_100}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid withholding がエラー: %v", err)
	}
	bad := ok
	bad.WithheldTax = 2_000_000
	if err := bad.Validate(); err == nil {
		t.Error("源泉徴収 > 支払金額はエラー")
	}
}

func TestLossCarryforwardValidate(t *testing.T) {
	ok := LossCarryforward{FiscalYear: 2025, LossYear: 2023, Amount: 500_000, UsedAmount: 100_000}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid loss がエラー: %v", err)
	}
	if ok.Remaining() != 400_000 {
		t.Errorf("Remaining = %d", ok.Remaining().Yen())
	}
	tests := []struct {
		name   string
		mutate func(*LossCarryforward)
	}{
		{"損失年度が適用年度以降", func(l *LossCarryforward) { l.LossYear = 2025 }},
		{"3年超の繰越", func(l *LossCarryforward) { l.LossYear = 2021 }},
		{"充当が超過", func(l *LossCarryforward) { l.UsedAmount = 600_000 }},
		{"金額ゼロ", func(l *LossCarryforward) { l.Amount = 0; l.UsedAmount = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := ok
			tt.mutate(&bad)
			if err := bad.Validate(); err == nil {
				t.Error("エラーになるべき")
			}
		})
	}
}

func TestOtherIncomeValidate(t *testing.T) {
	ok := OtherIncome{FiscalYear: 2025, Kind: OtherIncomeMiscellaneous,
		Description: "原稿料", Revenue: 300_000, Expenses: 50_000, WithheldTax: 30_630}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid income がエラー: %v", err)
	}
	if ok.NetIncome() != 250_000 {
		t.Errorf("NetIncome = %d", ok.NetIncome().Yen())
	}
	over := ok
	over.Expenses = 400_000
	if over.NetIncome() != 0 {
		t.Error("経費超過の所得は0")
	}
	if err := (&OtherIncome{Kind: "capital_gain", Description: "x"}).Validate(); err == nil {
		t.Error("未定義種別はエラー")
	}
	noDesc := ok
	noDesc.Description = ""
	if err := noDesc.Validate(); err == nil {
		t.Error("内容なしはエラー")
	}
}

func TestFixedAssetValidate(t *testing.T) {
	d, _ := ParseDate("2024-06-01")
	ok := FixedAsset{FiscalYear: 2025, Name: "PC", AcquisitionDate: d,
		AcquisitionCost: 300_000, UsefulLife: 4, Method: DepreciationStraightLine,
		BusinessUseRatioPct: 100, AccumulatedDepreciation: 75_000}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid asset がエラー: %v", err)
	}
	if ok.BookValue() != 225_000 {
		t.Errorf("BookValue = %d", ok.BookValue().Yen())
	}
	tests := []struct {
		name   string
		mutate func(*FixedAsset)
	}{
		{"名前なし", func(a *FixedAsset) { a.Name = "" }},
		{"耐用年数0", func(a *FixedAsset) { a.UsefulLife = 0 }},
		{"不正な償却方法", func(a *FixedAsset) { a.Method = "sum_of_years" }},
		{"定率法で償却率なし", func(a *FixedAsset) { a.Method = DepreciationDecliningBalance }},
		{"事業割合0", func(a *FixedAsset) { a.BusinessUseRatioPct = 0 }},
		{"償却累計が取得価額超", func(a *FixedAsset) { a.AccumulatedDepreciation = 400_000 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := ok
			tt.mutate(&bad)
			if err := bad.Validate(); err == nil {
				t.Error("エラーになるべき")
			}
		})
	}
}

func TestDefaultAccounts(t *testing.T) {
	accounts := DefaultAccounts()
	if len(accounts) != 72 {
		t.Errorf("科目数 = %d, want 72", len(accounts))
	}
	seen := map[AccountCode]bool{}
	for i := range accounts {
		a := &accounts[i]
		if err := a.Validate(); err != nil {
			t.Errorf("科目 %s が不正: %v", a.Code, err)
		}
		if seen[a.Code] {
			t.Errorf("科目コード重複: %s", a.Code)
		}
		seen[a.Code] = true
		if !a.IsActive {
			t.Errorf("科目 %s が inactive", a.Code)
		}
	}
	// コード体系と分類の整合
	for i := range accounts {
		a := &accounts[i]
		var want AccountCategory
		switch a.Code[0] {
		case '1':
			want = CategoryAsset
		case '2':
			want = CategoryLiability
		case '3':
			want = CategoryEquity
		case '4':
			want = CategoryRevenue
		case '5':
			want = CategoryExpense
		}
		if a.Category != want {
			t.Errorf("科目 %s: 分類 %s がコード体系と不一致", a.Code, a.Category)
		}
	}
	// 呼び出しごとに独立したスライスであること (共有状態の変更防止)
	first := DefaultAccounts()
	first[0].Name = "改変"
	if DefaultAccounts()[0].Name == "改変" {
		t.Error("DefaultAccounts が共有状態を返している")
	}
}
