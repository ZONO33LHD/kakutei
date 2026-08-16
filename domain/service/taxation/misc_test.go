package taxation

import (
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/model"
)

// 公的年金等控除 (令和2年分以後の現行法)。
func TestPensionDeduction(t *testing.T) {
	tests := []struct {
		name          string
		in            PensionDeductionInput
		wantDeduction model.Money
		wantTaxable   model.Money
	}{
		{"65歳未満・全額控除", PensionDeductionInput{PensionRevenue: 500_000}, 500_000, 0},
		{"65歳未満・300万", PensionDeductionInput{PensionRevenue: 3_000_000}, 1_025_000, 1_975_000},
		{"65歳以上・200万", PensionDeductionInput{PensionRevenue: 2_000_000, IsOver65: true}, 1_100_000, 900_000},
		{"65歳以上・400万", PensionDeductionInput{PensionRevenue: 4_000_000, IsOver65: true}, 1_275_000, 2_725_000},
		{"1000万超は上限195.5万", PensionDeductionInput{PensionRevenue: 12_000_000}, 1_955_000, 10_045_000},
		{"他所得1000万超で-10万",
			PensionDeductionInput{PensionRevenue: 3_000_000, OtherIncome: 15_000_000}, 925_000, 2_075_000},
		{"他所得2000万超で-20万",
			PensionDeductionInput{PensionRevenue: 3_000_000, OtherIncome: 25_000_000}, 825_000, 2_175_000},
		// 減額後も控除は年金収入を超えない (低額年金に架空の所得を生まない)
		{"低額年金は他所得減額でも所得0",
			PensionDeductionInput{PensionRevenue: 500_000, IsOver65: true, OtherIncome: 15_000_000}, 500_000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := PensionDeduction(tt.in)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if r.DeductionAmount != tt.wantDeduction {
				t.Errorf("Deduction = %d, want %d", r.DeductionAmount.Yen(), tt.wantDeduction.Yen())
			}
			if r.TaxablePensionIncome != tt.wantTaxable {
				t.Errorf("Taxable = %d, want %d", r.TaxablePensionIncome.Yen(), tt.wantTaxable.Yen())
			}
		})
	}

	if _, err := PensionDeduction(PensionDeductionInput{PensionRevenue: -1}); err == nil {
		t.Error("負の年金収入はエラー")
	}
}

// 退職所得 (所得税法第30条)。
func TestRetirementIncome(t *testing.T) {
	tests := []struct {
		name          string
		in            RetirementIncomeInput
		wantDeduction model.Money
		wantTaxable   model.Money
		wantHalf      bool
	}{
		{"勤続30年", RetirementIncomeInput{SeverancePay: 25_000_000, YearsOfService: 30},
			15_000_000, 5_000_000, true}, // 800万+70万×10、(2500万−1500万)/2
		{"勤続2年でも最低80万", RetirementIncomeInput{SeverancePay: 1_000_000, YearsOfService: 2},
			800_000, 100_000, true},
		{"役員5年以下は1/2なし", RetirementIncomeInput{SeverancePay: 5_000_000, YearsOfService: 5, IsOfficer: true},
			2_000_000, 3_000_000, false},
		{"一般短期300万超は部分1/2", RetirementIncomeInput{SeverancePay: 6_000_000, YearsOfService: 4},
			1_600_000, 2_900_000, false}, // 控除後440万 → 150万+140万
		{"一般短期300万以下は1/2", RetirementIncomeInput{SeverancePay: 3_000_000, YearsOfService: 4},
			1_600_000, 700_000, true},
		{"障害退職は+100万", RetirementIncomeInput{SeverancePay: 3_000_000, YearsOfService: 10, IsDisabilityRetirement: true},
			5_000_000, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := RetirementIncome(tt.in)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if r.RetirementDeduction != tt.wantDeduction {
				t.Errorf("Deduction = %d, want %d", r.RetirementDeduction.Yen(), tt.wantDeduction.Yen())
			}
			if r.TaxableRetirementIncome != tt.wantTaxable {
				t.Errorf("Taxable = %d, want %d", r.TaxableRetirementIncome.Yen(), tt.wantTaxable.Yen())
			}
			if r.HalfTaxationApplied != tt.wantHalf {
				t.Errorf("Half = %t, want %t", r.HalfTaxationApplied, tt.wantHalf)
			}
		})
	}

	if _, err := RetirementIncome(RetirementIncomeInput{SeverancePay: 100, YearsOfService: 0}); err == nil {
		t.Error("勤続年数0はエラー")
	}
}

// 減価償却。
func TestDepreciation(t *testing.T) {
	straight := func(cost model.Money, life, ratio, months int) model.Money {
		t.Helper()
		got, err := StraightLineDepreciation(cost, life, ratio, months)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		return got
	}
	if got := straight(1_200_000, 4, 100, 12); got != 300_000 {
		t.Errorf("定額法 = %d, want 300000", got.Yen())
	}
	if got := straight(1_200_000, 4, 100, 6); got != 150_000 {
		t.Errorf("定額法半年 = %d, want 150000", got.Yen())
	}
	if got := straight(1_200_000, 4, 50, 12); got != 150_000 {
		t.Errorf("事業割合50%% = %d, want 150000", got.Yen())
	}
	if _, err := StraightLineDepreciation(1_200_000, 0, 100, 12); err == nil {
		t.Error("耐用年数0はエラー")
	}
	if _, err := StraightLineDepreciation(1_200_000, 4, 100, 13); err == nil {
		t.Error("13ヶ月はエラー")
	}

	decl := func(book model.Money, rate, ratio, months int) model.Money {
		t.Helper()
		got, err := DecliningBalanceDepreciation(book, rate, ratio, months)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		return got
	}
	if got := decl(1_000_000, 500, 100, 12); got != 500_000 {
		t.Errorf("定率法 = %d, want 500000", got.Yen())
	}
	if got := decl(1_000_000, 500, 50, 6); got != 125_000 {
		t.Errorf("定率法半年50%% = %d, want 125000", got.Yen())
	}
	if _, err := DecliningBalanceDepreciation(1_000_000, 0, 100, 12); err == nil {
		t.Error("償却率0はエラー")
	}
}

// 公的年金等の雑所得が所得税計算に統合される。
func TestIncomeTaxWithPension(t *testing.T) {
	// 65歳未満・年金300万のみ: 控除102.5万 → 雑所得197.5万
	r := calcIncomeTax(t, IncomeTaxInput{
		FiscalYear:     2025,
		PensionRevenue: 3_000_000,
	})
	if r.PensionIncome != 1_975_000 {
		t.Errorf("PensionIncome = %d, want 1975000", r.PensionIncome.Yen())
	}
	if r.AggregateIncome != 1_975_000 {
		t.Errorf("AggregateIncome = %d, want 1975000", r.AggregateIncome.Yen())
	}
}

// ふるさと納税の控除上限推定。
func TestEstimateFurusatoLimit(t *testing.T) {
	// 課税所得 195万 (税率5%): 住民税所得割 195,000
	// 分母 = 1000 − 51 − 100 = 849、上限 = 195,000×200/849 + 2,000 = 47,936
	got := EstimateFurusatoLimit(3_000_000, 1_050_000, 0)
	if got != 47_936 {
		t.Errorf("= %d, want 47936", got.Yen())
	}

	if got := EstimateFurusatoLimit(1_000_000, 2_000_000, 0); got != 0 {
		t.Errorf("課税所得0なら0: %d", got.Yen())
	}

	// 明示的な税率指定も受け付ける
	explicit := EstimateFurusatoLimit(3_000_000, 1_050_000, 5)
	if explicit != got {
		t.Errorf("明示税率と自動判定が一致すべき: %d != %d", explicit.Yen(), got.Yen())
	}
}

// サニティチェック。
func TestSanityCheckIncomeTax(t *testing.T) {
	svc := NewIncomeTaxService()

	t.Run("正常ケースはpass", func(t *testing.T) {
		in := IncomeTaxInput{
			FiscalYear:          2025,
			BusinessRevenue:     5_000_000,
			BlueReturnDeduction: 650_000,
			SocialInsurance:     500_000,
		}
		r, err := svc.Calculate(in)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		check := SanityCheckIncomeTax(&in, r)
		if !check.Passed || check.ErrorCount != 0 {
			t.Errorf("passすべき: %+v", check.Items)
		}
	})

	t.Run("給与ありで源泉0は警告", func(t *testing.T) {
		in := IncomeTaxInput{FiscalYear: 2025, SalaryRevenue: 3_000_000}
		r, err := svc.Calculate(in)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		check := SanityCheckIncomeTax(&in, r)
		if !hasSanityCode(check, "NO_WITHHOLDING_ON_SALARY") {
			t.Error("NO_WITHHOLDING_ON_SALARY の警告が出るべき")
		}
		if !check.Passed {
			t.Error("警告のみなら passed = true")
		}
	})

	t.Run("改ざんされた結果はエラー検出", func(t *testing.T) {
		in := IncomeTaxInput{FiscalYear: 2025, BusinessRevenue: 5_000_000}
		r, err := svc.Calculate(in)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		r.TaxableIncome = 3_170_500 // 1,000円未満の端数
		r.ReconstructionTax += 1    // 復興税不一致
		check := SanityCheckIncomeTax(&in, r)
		if check.Passed {
			t.Error("エラーを検出すべき")
		}
		if !hasSanityCode(check, "TAXABLE_INCOME_ROUNDING") || !hasSanityCode(check, "RECONSTRUCTION_TAX_MISMATCH") {
			t.Errorf("期待したコードが見つからない: %+v", check.Items)
		}
	})
}

func hasSanityCode(r *SanityCheckResult, code string) bool {
	for _, it := range r.Items {
		if it.Code == code {
			return true
		}
	}
	return false
}
