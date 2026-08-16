package taxation

import (
	"strings"
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/model"
)

func calcIncomeTax(t *testing.T, in IncomeTaxInput) *IncomeTaxResult {
	t.Helper()
	r, err := NewIncomeTaxService().Calculate(in)
	if err != nil {
		t.Fatalf("Calculate error: %v", err)
	}
	return r
}

// 課税所得の1,000円未満切捨て (国税通則法第118条)。
func TestIncomeTaxTaxableIncomeRounding(t *testing.T) {
	// 事業所得 1,234,567 → 基礎控除95万 + 社保1万 → 274,567 → 274,000
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:      2025,
		BusinessRevenue: 1_234_567,
		SocialInsurance: 10_000,
	})
	if r.TaxableIncome != 274_000 {
		t.Errorf("TaxableIncome = %d, want 274000", r.TaxableIncome.Yen())
	}
}

// 検証済みのエンドツーエンドの数値例。
func TestIncomeTaxFullFlow(t *testing.T) {
	// 事業収入500万、青色65万、社保50万:
	// 事業所得 = 435万、基礎控除68万 → 課税所得 317万
	// 税額 = 317万×10% − 97,500 = 219,500
	// 復興税 = 219,500×2.1% = 4,609 (1円未満切捨)
	// 合計 224,109 → 納付 224,100 (100円未満切捨)
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:          2025,
		BusinessRevenue:     5_000_000,
		BlueReturnDeduction: 650_000,
		SocialInsurance:     500_000,
	})
	if r.BusinessIncome != 4_350_000 {
		t.Errorf("BusinessIncome = %d", r.BusinessIncome.Yen())
	}
	if r.TaxableIncome != 3_170_000 {
		t.Errorf("TaxableIncome = %d, want 3170000", r.TaxableIncome.Yen())
	}
	if r.IncomeTaxBase != 219_500 {
		t.Errorf("IncomeTaxBase = %d, want 219500", r.IncomeTaxBase.Yen())
	}
	if r.ReconstructionTax != 4_609 {
		t.Errorf("ReconstructionTax = %d, want 4609", r.ReconstructionTax.Yen())
	}
	if r.TotalTax != 224_109 {
		t.Errorf("TotalTax = %d, want 224109", r.TotalTax.Yen())
	}
	if r.TaxDue != 224_100 {
		t.Errorf("TaxDue = %d, want 224100", r.TaxDue.Yen())
	}
}

// 還付 (負) は100円未満切捨てせず1円単位 (国税通則法第120条)。
func TestIncomeTaxRefundNotTruncated(t *testing.T) {
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:          2025,
		BusinessRevenue:     500_000,
		BlueReturnDeduction: 650_000,
		SocialInsurance:     100_000,
		SalaryWithheldTax:   50_000,
	})
	if r.TaxDue != -50_000 {
		t.Errorf("TaxDue = %d, want -50000 (1円単位のまま)", r.TaxDue.Yen())
	}
}

func TestIncomeTaxZero(t *testing.T) {
	r := calcIncomeTax(t, IncomeTaxInput{FiscalYear: 2025})
	if r.TaxDue != 0 || r.TotalTax != 0 || r.TaxableIncome != 0 {
		t.Errorf("全て0であるべき: %+v", r)
	}
}

// 青色申告特別控除の自動調整と警告。
func TestIncomeTaxBlueReturnAutoAdjustWarning(t *testing.T) {
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:          2025,
		BusinessRevenue:     500_000,
		BusinessExpenses:    200_000,
		BlueReturnDeduction: 650_000,
	})
	if r.EffectiveBlueReturnDeduction != 300_000 {
		t.Errorf("Effective = %d, want 300000", r.EffectiveBlueReturnDeduction.Yen())
	}
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "自動調整") {
		t.Errorf("自動調整の警告が出るべき: %v", r.Warnings)
	}

	noAdjust := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:          2025,
		BusinessRevenue:     3_000_000,
		BusinessExpenses:    1_000_000,
		BlueReturnDeduction: 650_000,
	})
	if len(noAdjust.Warnings) != 0 {
		t.Errorf("調整なしなら警告なし: %v", noAdjust.Warnings)
	}
}

// 損益通算: 事業赤字は給与所得と通算される。
func TestIncomeTaxLossOffset(t *testing.T) {
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:          2025,
		SalaryRevenue:       3_000_000, // 給与所得 2,020,000 (A=750,000×2.8−8万)
		BusinessRevenue:     100_000,
		BusinessExpenses:    600_000, // 赤字 -500,000
		BlueReturnDeduction: 0,
	})
	if r.SalaryIncome != 2_020_000 {
		t.Errorf("SalaryIncome = %d", r.SalaryIncome.Yen())
	}
	if r.BusinessIncome != -500_000 {
		t.Errorf("BusinessIncome = %d", r.BusinessIncome.Yen())
	}
	if r.TotalIncome != 1_520_000 {
		t.Errorf("TotalIncome = %d, want 1520000", r.TotalIncome.Yen())
	}
}

// 純損失の繰越控除。人的控除の所得判定は繰越控除「前」の合計所得金額で行う。
func TestIncomeTaxLossCarryforward(t *testing.T) {
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:       2025,
		BusinessRevenue:  2_000_000,
		LossCarryforward: 500_000,
	})
	if r.LossCarryforwardApplied != 500_000 {
		t.Errorf("Applied = %d, want 500000", r.LossCarryforwardApplied.Yen())
	}
	if r.AggregateIncome != 2_000_000 {
		t.Errorf("AggregateIncome = %d, want 2000000", r.AggregateIncome.Yen())
	}
	if r.TotalIncome != 1_500_000 {
		t.Errorf("TotalIncome = %d, want 1500000", r.TotalIncome.Yen())
	}
	// 基礎控除は合計所得金額 200万 (繰越前) で判定 → 88万 (95万ではない)
	for _, item := range r.Deductions.IncomeDeductions {
		if item.Type == DeductionBasic && item.Amount != 880_000 {
			t.Errorf("基礎控除 = %d, want 880000 (繰越控除前の合計所得で判定)", item.Amount.Yen())
		}
	}

	// 繰越損失が所得を上回る場合は所得までしか使わない
	over := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:       2025,
		BusinessRevenue:  300_000,
		LossCarryforward: 1_000_000,
	})
	if over.LossCarryforwardApplied != 300_000 {
		t.Errorf("Applied = %d, want 300000", over.LossCarryforwardApplied.Yen())
	}
	if over.TotalIncome != 0 {
		t.Errorf("TotalIncome = %d, want 0", over.TotalIncome.Yen())
	}
}

// 一時所得は経常所得の損失を充当した後に1/2する (所得税法第69条の損益通算順序)。
func TestIncomeTaxOneTimeIncomeLossOffsetOrder(t *testing.T) {
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:         2025,
		BusinessExpenses:   1_000_000, // 事業赤字 -100万
		OneTimeIncomeGross: 2_500_000, // 特別控除後 200万
	})
	// (200万 − 100万) × 1/2 = 50万 (先に1/2すると 100万−100万 = 0 になる誤り)
	if r.AggregateIncome != 500_000 {
		t.Errorf("AggregateIncome = %d, want 500000", r.AggregateIncome.Yen())
	}
}

// 所得金額調整控除 (子ども等): 給与収入850万超で23歳未満の扶養親族がいる場合。
func TestIncomeTaxSalaryAdjustment(t *testing.T) {
	child := model.Dependent{Name: "子", BirthDate: mustDate(t, "2010-01-01")}
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:    2025,
		SalaryRevenue: 10_000_000,
		Dependents:    []model.Dependent{child},
	})
	// 給与所得 805万 − 調整控除 (1000万−850万)×10% = 15万 → 790万
	if r.SalaryIncome != 7_900_000 {
		t.Errorf("SalaryIncome = %d, want 7900000", r.SalaryIncome.Yen())
	}

	// 要件非該当 (扶養なし) なら調整なし
	no := calcIncomeTax(t, IncomeTaxInput{FiscalYear: 2025, SalaryRevenue: 10_000_000})
	if no.SalaryIncome != 8_050_000 {
		t.Errorf("SalaryIncome = %d, want 8050000", no.SalaryIncome.Yen())
	}
}

// 申告納税額は源泉徴収控除後に100円未満切捨て、その後に予定納税を差し引く。
func TestIncomeTaxEstimatedPaymentOrdering(t *testing.T) {
	// totalTax 224,109 → 源泉0 → 申告納税額 224,100 (切捨) → 予定納税 224,109 → -9円 (還付)
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:          2025,
		BusinessRevenue:     5_000_000,
		BlueReturnDeduction: 650_000,
		SocialInsurance:     500_000,
		EstimatedTaxPayment: 224_109,
	})
	if r.TaxDue != -9 {
		t.Errorf("TaxDue = %d, want -9 (切捨後に予定納税を控除)", r.TaxDue.Yen())
	}
}

// 政治活動寄附金: 税額控除 (30%) と所得控除の有利選択。
func TestIncomeTaxDonationCreditSelection(t *testing.T) {
	donation := model.DonationRecord{
		Kind: model.DonationPolitical, RecipientName: "政党X",
		Amount: 100_000, Date: mustDate(t, "2025-06-01"),
	}
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:      2025,
		BusinessRevenue: 5_000_000, // 限界税率20% → 税額控除30%が有利
		Donations:       []model.DonationRecord{donation},
	})
	// 税額控除: (100,000−2,000)×30% = 29,400 (100円未満切捨で29,400のまま)
	if got := creditAmountOf(r.Deductions.TaxCredits, CreditPoliticalDonation); got != 29_400 {
		t.Errorf("政党等寄附金特別控除 = %d, want 29400", got.Yen())
	}
	// 所得控除側に寄附金控除は含まれない
	for _, item := range r.Deductions.IncomeDeductions {
		if item.Type == DeductionDonation {
			t.Error("税額控除選択時は所得控除の寄附金控除は0であるべき")
		}
	}
	if len(r.Deductions.Notes) == 0 {
		t.Error("選択適用の注記があるべき")
	}
}

// 認定NPO寄附金: 税額控除 (40%) が選択される。
func TestIncomeTaxNPODonationCreditSelection(t *testing.T) {
	donation := model.DonationRecord{
		Kind: model.DonationNPO, RecipientName: "認定NPO A",
		Amount: 100_000, Date: mustDate(t, "2025-06-01"),
	}
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:      2025,
		BusinessRevenue: 5_000_000, // 限界税率20% → 40%税額控除が有利
		Donations:       []model.DonationRecord{donation},
	})
	// (100,000−2,000)×40% = 39,200
	if got := creditAmountOf(r.Deductions.TaxCredits, CreditNPODonation); got != 39_200 {
		t.Errorf("認定NPO寄附金特別控除 = %d, want 39200", got.Yen())
	}
}

// 3区分混在: 政党等30% + 認定NPO40% + 公益社団等40% が同時に適用される。
func TestIncomeTaxMixedDonationCredits(t *testing.T) {
	mk := func(kind model.DonationKind, name string) model.DonationRecord {
		return model.DonationRecord{Kind: kind, RecipientName: name, Amount: 50_000, Date: mustDate(t, "2025-06-01")}
	}
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:       2025,
		BusinessRevenue:  5_000_000,
		FurusatoDonation: 20_000, // 2,000円の自己負担は所得控除側が消化
		Donations: []model.DonationRecord{
			mk(model.DonationPolitical, "政党X"),
			mk(model.DonationNPO, "NPO A"),
			mk(model.DonationPublicInterest, "公益B"),
		},
	})
	if got := creditAmountOf(r.Deductions.TaxCredits, CreditPoliticalDonation); got != 15_000 {
		t.Errorf("政党等 = %d, want 15000 (50,000×30%%)", got.Yen())
	}
	if got := creditAmountOf(r.Deductions.TaxCredits, CreditNPODonation); got != 20_000 {
		t.Errorf("認定NPO = %d, want 20000 (50,000×40%%)", got.Yen())
	}
	if got := creditAmountOf(r.Deductions.TaxCredits, CreditPublicInterestDonation); got != 20_000 {
		t.Errorf("公益社団等 = %d, want 20000 (50,000×40%%)", got.Yen())
	}
}

// 認定NPOと公益社団等は共通の25%キャップ。キャップに達すると
// 片方を所得控除に回す混合パターンが最有利になり得る。
func TestIncomeTaxNPOSharedCapMixedSelection(t *testing.T) {
	mk := func(kind model.DonationKind, name string) model.DonationRecord {
		return model.DonationRecord{Kind: kind, RecipientName: name, Amount: 100_000, Date: mustDate(t, "2025-06-01")}
	}
	// 事業所得150万 → 基礎控除88万 → 課税所得62万 → 税額31,000 → 25%枠 7,750
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:      2025,
		BusinessRevenue: 1_500_000,
		Donations: []model.DonationRecord{
			mk(model.DonationNPO, "NPO A"),
			mk(model.DonationPublicInterest, "公益B"),
		},
	})
	// 最有利: 片方を所得控除 (98,000)、もう片方を税額控除 (25%枠まで)
	hasIncomeDonation := false
	for _, item := range r.Deductions.IncomeDeductions {
		if item.Type == DeductionDonation && item.Amount == 98_000 {
			hasIncomeDonation = true
		}
	}
	creditTotal := creditAmountOf(r.Deductions.TaxCredits, CreditNPODonation) +
		creditAmountOf(r.Deductions.TaxCredits, CreditPublicInterestDonation)
	if !hasIncomeDonation || creditTotal != 6_500 {
		t.Errorf("混合選択が最有利: incomeDonation=%t creditTotal=%d (want 6500)",
			hasIncomeDonation, creditTotal.Yen())
	}
	// netTax = 26,100 − 6,500 = 19,600
	if r.IncomeTaxAfterCredits != 19_600 {
		t.Errorf("IncomeTaxAfterCredits = %d, want 19600", r.IncomeTaxAfterCredits.Yen())
	}
}

// エンティティの年度が計算対象年度と不一致ならエラー。
func TestIncomeTaxEntityYearMismatch(t *testing.T) {
	dep := model.Dependent{
		Name: "子", FiscalYear: 2024, BirthDate: mustDate(t, "2010-01-01"),
	}
	if _, err := NewIncomeTaxService().Calculate(IncomeTaxInput{
		FiscalYear: 2025,
		Dependents: []model.Dependent{dep},
	}); err == nil {
		t.Error("年度不一致の扶養親族はエラーになるべき")
	}
}

// 2,000円の自己負担は全ての特定寄附金で共通 (二重に差し引かない)。
func TestIncomeTaxDonationSharedBurden(t *testing.T) {
	donation := model.DonationRecord{
		Kind: model.DonationPolitical, RecipientName: "政党X",
		Amount: 100_000, Date: mustDate(t, "2025-06-01"),
	}
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:       2025,
		BusinessRevenue:  5_000_000,
		FurusatoDonation: 50_000, // 所得控除側が 2,000円 を消化する
		Donations:        []model.DonationRecord{donation},
	})
	// ふるさと納税の所得控除: 50,000−2,000 = 48,000
	// 政党等の税額控除: 100,000×30% = 30,000 (2,000円は既に消化済み)
	if got := creditAmountOf(r.Deductions.TaxCredits, CreditPoliticalDonation); got != 30_000 {
		t.Errorf("政党等寄附金特別控除 = %d, want 30000", got.Yen())
	}
}

// 寄附金特別控除の最終額は100円未満切捨て。
func TestIncomeTaxDonationCreditRounding(t *testing.T) {
	donation := model.DonationRecord{
		Kind: model.DonationPolitical, RecipientName: "政党X",
		Amount: 10_801, Date: mustDate(t, "2025-06-01"),
	}
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:      2025,
		BusinessRevenue: 5_000_000,
		Donations:       []model.DonationRecord{donation},
	})
	// (10,801−2,000)×30% = 2,640 → 100円未満切捨 = 2,600
	if got := creditAmountOf(r.Deductions.TaxCredits, CreditPoliticalDonation); got != 2_600 {
		t.Errorf("= %d, want 2600", got.Yen())
	}
}

// 寄附日が課税年度外はエラー。
func TestIncomeTaxDonationDateOutOfYear(t *testing.T) {
	donation := model.DonationRecord{
		Kind: model.DonationOther, RecipientName: "団体Y",
		Amount: 10_000, Date: mustDate(t, "2024-12-31"),
	}
	if _, err := NewIncomeTaxService().Calculate(IncomeTaxInput{
		FiscalYear:      2025,
		BusinessRevenue: 5_000_000,
		Donations:       []model.DonationRecord{donation},
	}); err == nil {
		t.Error("年度外の寄附日はエラーになるべき")
	}
}

// ふるさと納税は常に所得控除 (税額控除の選択肢なし)。
func TestIncomeTaxFurusatoIncomeDeduction(t *testing.T) {
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:       2025,
		BusinessRevenue:  5_000_000,
		FurusatoDonation: 50_000,
	})
	found := false
	for _, item := range r.Deductions.IncomeDeductions {
		if item.Type == DeductionDonation && item.Amount == 48_000 {
			found = true
		}
	}
	if !found {
		t.Error("ふるさと納税の寄附金控除 48,000 が含まれるべき")
	}
}

// 配当控除が税額控除に反映される。
func TestIncomeTaxDividendCredit(t *testing.T) {
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:      2025,
		BusinessRevenue: 5_000_000,
		DividendIncome:  200_000,
	})
	if r.DividendCredit != 20_000 {
		t.Errorf("DividendCredit = %d, want 20000", r.DividendCredit.Yen())
	}
}

// 入力バリデーション。
func TestIncomeTaxInputValidation(t *testing.T) {
	svc := NewIncomeTaxService()
	cases := []IncomeTaxInput{
		{FiscalYear: 2024}, // 未対応年度
		{FiscalYear: 2025, BlueReturnDeduction: 12345}, // 法定外の青色控除
		{FiscalYear: 2025, SalaryRevenue: -1},          // 負の金額
		{FiscalYear: 2025, WidowStatus: "divorced"},    // 不正な区分
	}
	for i, in := range cases {
		if _, err := svc.Calculate(in); err == nil {
			t.Errorf("case %d: エラーになるべき", i)
		}
	}
}

// 税額控除が算出税額を超える場合は0で下げ止まる。
func TestIncomeTaxCreditsFloorAtZero(t *testing.T) {
	loan := model.HousingLoanDetail{
		Kind: model.HousingUsed, Category: "general",
		MoveInDate: mustDate(t, "2025-04-01"), YearEndBalance: 20_000_000,
	}
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:      2025,
		BusinessRevenue: 1_500_000, // 少額の税額 < 住宅ローン控除 140,000
		HousingLoans:    []model.HousingLoanDetail{loan},
	})
	if r.IncomeTaxAfterCredits != 0 {
		t.Errorf("IncomeTaxAfterCredits = %d, want 0", r.IncomeTaxAfterCredits.Yen())
	}
	if r.TotalTax != 0 || r.TaxDue != 0 {
		t.Errorf("TotalTax = %d, TaxDue = %d, want 0", r.TotalTax.Yen(), r.TaxDue.Yen())
	}
}
