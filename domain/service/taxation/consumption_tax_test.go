package taxation

import (
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

func calcConsumption(t *testing.T, in ConsumptionTaxInput) *ConsumptionTaxResult {
	t.Helper()
	r, err := NewConsumptionTaxService().Calculate(in)
	if err != nil {
		t.Fatalf("Calculate error: %v", err)
	}
	return r
}

// 課税標準額の1,000円未満切捨て (国税通則法第118条)。
func TestConsumptionTaxableBaseRounding(t *testing.T) {
	tests := []struct {
		name            string
		sales10, sales8 model.Money
		wantB10, wantB8 model.Money
	}{
		{"税抜が1000の倍数", 1_100_000, 0, 1_000_000, 0},
		{"端数切捨て", 1_234_567, 0, 1_122_000, 0}, // 1,234,567×100/110=1,122,333→1,122,000
		{"軽減税率", 0, 108_000, 0, 100_000},
		{"軽減税率の端数", 0, 123_456, 0, 114_000}, // ×100/108=114,311→114,000
		{"混在", 1_100_000, 108_000, 1_000_000, 100_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := calcConsumption(t, ConsumptionTaxInput{
				FiscalYear: 2025, Method: MethodStandard,
				TaxableSales10: tt.sales10, TaxableSales8: tt.sales8,
			})
			if r.TaxableBase10 != tt.wantB10 || r.TaxableBase8 != tt.wantB8 {
				t.Errorf("base10 = %d (want %d), base8 = %d (want %d)",
					r.TaxableBase10.Yen(), tt.wantB10.Yen(), r.TaxableBase8.Yen(), tt.wantB8.Yen())
			}
		})
	}
}

// 2割特例 (freee 検証済みの数値例)。
func TestConsumptionSpecial20Pct(t *testing.T) {
	r := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodSpecial20Pct, TaxableSales10: 165_000,
	})
	if r.TaxableBase10 != 150_000 {
		t.Errorf("TaxableBase10 = %d, want 150000", r.TaxableBase10.Yen())
	}
	if r.NationalTaxOnSales != 11_700 {
		t.Errorf("NationalTaxOnSales = %d, want 11700", r.NationalTaxOnSales.Yen())
	}
	if r.NetTax != 2_300 { // 11,700×20% = 2,340 → 100円切捨
		t.Errorf("NetTax = %d, want 2300", r.NetTax.Yen())
	}
	if r.LocalTaxDue != 600 { // 2,300×22/78 = 648 → 100円切捨
		t.Errorf("LocalTaxDue = %d, want 600", r.LocalTaxDue.Yen())
	}
	if r.TotalDue != 2_900 {
		t.Errorf("TotalDue = %d, want 2900", r.TotalDue.Yen())
	}

	large := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodSpecial20Pct, TaxableSales10: 5_500_000,
	})
	if large.NationalTaxOnSales != 390_000 || large.NetTax != 78_000 || large.TaxDue != 78_000 {
		t.Errorf("national=%d net=%d due=%d", large.NationalTaxOnSales.Yen(), large.NetTax.Yen(), large.TaxDue.Yen())
	}
}

// 簡易課税 (みなし仕入率)。
func TestConsumptionSimplified(t *testing.T) {
	service := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodSimplified,
		TaxableSales10: 1_100_000, SimplifiedBusinessType: policy.SimplifiedService,
	})
	if service.TaxOnPurchases != 39_000 || service.NetTax != 39_000 {
		t.Errorf("第5種: purchases=%d net=%d", service.TaxOnPurchases.Yen(), service.NetTax.Yen())
	}

	wholesale := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodSimplified,
		TaxableSales10: 2_200_000, SimplifiedBusinessType: policy.SimplifiedWholesale,
	})
	if wholesale.NationalTaxOnSales != 156_000 || wholesale.TaxOnPurchases != 140_400 || wholesale.NetTax != 15_600 {
		t.Errorf("第1種: national=%d purchases=%d net=%d",
			wholesale.NationalTaxOnSales.Yen(), wholesale.TaxOnPurchases.Yen(), wholesale.NetTax.Yen())
	}
}

// 簡易課税で事業区分未指定はエラー (暗黙のフォールバック禁止)。
func TestConsumptionSimplifiedRequiresBusinessType(t *testing.T) {
	_, err := NewConsumptionTaxService().Calculate(ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodSimplified, TaxableSales10: 1_100_000,
	})
	if err == nil {
		t.Error("事業区分未指定はエラーになるべき")
	}
}

// 本則課税。
func TestConsumptionStandard(t *testing.T) {
	r := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodStandard,
		TaxableSales10: 1_234_567, TaxablePurchases10: 987_654,
	})
	if r.TotalDue != 22_300 {
		t.Errorf("TotalDue = %d, want 22300", r.TotalDue.Yen())
	}

	purchases := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodStandard,
		TaxableSales10: 2_200_000, TaxablePurchases10: 1_100_000,
	})
	if purchases.TaxOnPurchases != 78_000 { // 1,100,000×7.8/110
		t.Errorf("TaxOnPurchases = %d, want 78000", purchases.TaxOnPurchases.Yen())
	}

	reduced := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodStandard,
		TaxableSales10: 1_100_000, TaxablePurchases8: 108_000,
	})
	if reduced.TaxOnPurchases != 6_240 { // 108,000×6.24/108
		t.Errorf("TaxOnPurchases(8%%) = %d, want 6240", reduced.TaxOnPurchases.Yen())
	}

	mixed := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodStandard,
		TaxableSales10: 5_500_000, TaxablePurchases10: 1_100_000, TaxablePurchases8: 216_000,
	})
	if mixed.TaxOnPurchases != 78_000+12_480 {
		t.Errorf("混在仕入 = %d, want 90480", mixed.TaxOnPurchases.Yen())
	}
}

// 還付 (控除不足還付税額)。
func TestConsumptionRefund(t *testing.T) {
	r := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodStandard,
		TaxableSales10: 110_000, TaxablePurchases10: 1_100_000,
	})
	// 売上国税 7,800 − 仕入国税 78,000 = -70,200
	if r.RefundShortfall != 70_200 {
		t.Errorf("RefundShortfall = %d, want 70200", r.RefundShortfall.Yen())
	}
	if r.NetTax != 0 {
		t.Errorf("NetTax = %d, want 0", r.NetTax.Yen())
	}
	if r.LocalTaxDue != -19_800 { // 70,200×22/78 = 19,800
		t.Errorf("LocalTaxDue = %d, want -19800", r.LocalTaxDue.Yen())
	}
	// 合計還付額は国税分 (控除不足還付税額) を含む
	if r.TaxDue != -70_200 {
		t.Errorf("TaxDue = %d, want -70200", r.TaxDue.Yen())
	}
	if r.TotalDue != -90_000 {
		t.Errorf("TotalDue = %d, want -90000 (国税70,200+地方19,800の還付)", r.TotalDue.Yen())
	}
}

// 課税売上割合95%未満では一括比例配分方式で仕入税額を按分する。
func TestConsumptionProportionalCredit(t *testing.T) {
	// 課税売上 (税抜) 1,000,000、非課税売上 1,000,000 → 課税売上割合50%
	r := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodStandard,
		TaxableSales10: 1_100_000, TaxablePurchases10: 550_000, NonTaxableSales: 1_000_000,
	})
	if !r.ProportionalCreditApplied {
		t.Error("一括比例配分が適用されるべき")
	}
	// 仕入税額全額 = 550,000×78/1100 = 39,000 → ×1,000,000/2,000,000 = 19,500
	if r.TaxOnPurchases != 19_500 {
		t.Errorf("TaxOnPurchases = %d, want 19500", r.TaxOnPurchases.Yen())
	}

	// 割合95%以上なら全額控除
	full := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodStandard,
		TaxableSales10: 1_100_000, TaxablePurchases10: 550_000, NonTaxableSales: 50_000,
	})
	if full.ProportionalCreditApplied || full.TaxOnPurchases != 39_000 {
		t.Errorf("95%%以上は全額控除: applied=%t purchases=%d",
			full.ProportionalCreditApplied, full.TaxOnPurchases.Yen())
	}
}

// 中間納付の控除と混在売上。
func TestConsumptionInterimAndMixedSales(t *testing.T) {
	mixed := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodSpecial20Pct,
		TaxableSales10: 1_100_000, TaxableSales8: 108_000,
	})
	if mixed.NationalTaxOnSales != 84_240 { // 78,000 + 6,240
		t.Errorf("NationalTaxOnSales = %d, want 84240", mixed.NationalTaxOnSales.Yen())
	}

	interim := calcConsumption(t, ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodSpecial20Pct,
		TaxableSales10: 5_500_000, InterimPayment: 30_000,
	})
	if interim.TaxDue != 48_000 { // 78,000 − 30,000
		t.Errorf("TaxDue = %d, want 48000", interim.TaxDue.Yen())
	}
}

func TestConsumptionZeroAndValidation(t *testing.T) {
	zero := calcConsumption(t, ConsumptionTaxInput{FiscalYear: 2025, Method: MethodStandard})
	if zero.NetTax != 0 || zero.TotalDue != 0 {
		t.Errorf("売上0は全て0: %+v", zero)
	}

	svc := NewConsumptionTaxService()
	if _, err := svc.Calculate(ConsumptionTaxInput{FiscalYear: 2025, Method: "flat"}); err == nil {
		t.Error("不正な方式はエラー")
	}
	if _, err := svc.Calculate(ConsumptionTaxInput{FiscalYear: 2024, Method: MethodStandard}); err == nil {
		t.Error("未対応年度はエラー")
	}
	if _, err := svc.Calculate(ConsumptionTaxInput{
		FiscalYear: 2025, Method: MethodStandard, TaxableSales10: -1,
	}); err == nil {
		t.Error("負の売上はエラー")
	}
}
