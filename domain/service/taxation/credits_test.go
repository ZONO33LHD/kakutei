package taxation

import (
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

func usedHouseDetail(t *testing.T, kind model.HousingKind, balance, cost model.Money) model.HousingLoanDetail {
	t.Helper()
	return model.HousingLoanDetail{
		Kind:                 kind,
		Category:             policy.HousingGeneral,
		MoveInDate:           mustDate(t, "2025-04-01"),
		YearEndBalance:       balance,
		IsNewConstruction:    false,
		DualApplicationGroup: "loan-01",
		CostForProration:     cost,
	}
}

// 重複適用 (中古購入＋リフォーム) の按分計算 (検証済みの期待値と一致させる)。
func TestHousingLoanCreditDualBasicProration(t *testing.T) {
	purchase := usedHouseDetail(t, model.HousingUsed, 15_151_931, 42_800_000)
	renovation := usedHouseDetail(t, model.HousingRenovation, 15_151_931, 5_000_000)

	total, entries, err := HousingLoanCreditDual([]model.HousingLoanDetail{purchase, renovation})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if entries[0].ProratedBalance != 13_567_000 {
		t.Errorf("購入分按分 = %d, want 13567000", entries[0].ProratedBalance.Yen())
	}
	if entries[1].ProratedBalance != 1_584_931 {
		t.Errorf("リフォーム分按分 = %d, want 1584931", entries[1].ProratedBalance.Yen())
	}
	if got := entries[0].ProratedBalance + entries[1].ProratedBalance; got != 15_151_931 {
		t.Errorf("按分合計 = %d, 元の年末残高と不一致", got.Yen())
	}
	if entries[0].Credit != 94_900 {
		t.Errorf("購入分控除 = %d, want 94900", entries[0].Credit.Yen())
	}
	if entries[1].Credit != 11_000 {
		t.Errorf("リフォーム分控除 = %d, want 11000", entries[1].Credit.Yen())
	}
	if total != 105_900 {
		t.Errorf("合計控除 = %d, want 105900", total.Yen())
	}
}

// 按分後残高が限度額を超える場合のキャップと ㉓欄上限。
func TestHousingLoanCreditDualLimitCap(t *testing.T) {
	purchase := usedHouseDetail(t, model.HousingUsed, 50_000_000, 45_000_000)
	purchase.Category = policy.HousingCertified
	renovation := usedHouseDetail(t, model.HousingRenovation, 50_000_000, 5_000_000)
	renovation.Category = policy.HousingCertified

	total, entries, err := HousingLoanCreditDual([]model.HousingLoanDetail{purchase, renovation})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if entries[0].ProratedBalance != 45_000_000 || entries[0].CappedBalance != 30_000_000 {
		t.Errorf("購入分: prorated=%d capped=%d", entries[0].ProratedBalance.Yen(), entries[0].CappedBalance.Yen())
	}
	if entries[1].CappedBalance != 5_000_000 {
		t.Errorf("リフォーム分 capped = %d", entries[1].CappedBalance.Yen())
	}
	// 各控除限度額の最大 = 3,000万×0.7% = 210,000 でキャップ
	if total != 210_000 {
		t.Errorf("合計控除 = %d, want 210000", total.Yen())
	}
}

func TestHousingLoanCreditDualRoundingSum(t *testing.T) {
	a := usedHouseDetail(t, model.HousingUsed, 10_000_001, 30_000_000)
	b := usedHouseDetail(t, model.HousingRenovation, 10_000_001, 20_000_000)
	_, entries, err := HousingLoanCreditDual([]model.HousingLoanDetail{a, b})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got := entries[0].ProratedBalance + entries[1].ProratedBalance; got != 10_000_001 {
		t.Errorf("按分合計 = %d, want 10000001", got.Yen())
	}
}

func TestHousingLoanCreditDualErrors(t *testing.T) {
	valid := usedHouseDetail(t, model.HousingUsed, 10_000_000, 30_000_000)
	zeroCost := usedHouseDetail(t, model.HousingRenovation, 10_000_000, 0)
	if _, _, err := HousingLoanCreditDual([]model.HousingLoanDetail{valid, zeroCost}); err == nil {
		t.Error("按分コスト0はエラーになるべき")
	}
	if _, _, err := HousingLoanCreditDual([]model.HousingLoanDetail{valid}); err == nil {
		t.Error("明細1件はエラーになるべき")
	}
}

func TestHousingLoanCreditSingle(t *testing.T) {
	// 一般中古住宅: 限度額2,000万。残高2,500万 → 2,000万×0.7% = 140,000
	d := usedHouseDetail(t, model.HousingUsed, 25_000_000, 0)
	d.DualApplicationGroup = ""
	credit, err := HousingLoanCredit(&d)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if credit != 140_000 {
		t.Errorf("= %d, want 140000", credit.Yen())
	}

	// R6-R7 一般住宅新築は原則対象外 (限度額0)
	newGeneral := d
	newGeneral.IsNewConstruction = true
	newGeneral.Kind = model.HousingNewCustom
	credit, err = HousingLoanCredit(&newGeneral)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if credit != 0 {
		t.Errorf("R6以降入居の一般住宅新築 = %d, want 0", credit.Yen())
	}

	// R5以前建築確認済みなら特例上限2,000万
	newGeneral.HasPreR6Permit = true
	credit, err = HousingLoanCredit(&newGeneral)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if credit != 140_000 {
		t.Errorf("R5確認済み特例 = %d, want 140000", credit.Yen())
	}

	// R4-R5入居の認定住宅新築: 限度額5,000万
	r5 := model.HousingLoanDetail{
		Kind: model.HousingNewCustom, Category: policy.HousingCertified,
		MoveInDate: mustDate(t, "2023-06-01"), YearEndBalance: 60_000_000, IsNewConstruction: true,
	}
	credit, err = HousingLoanCredit(&r5)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if credit != 350_000 {
		t.Errorf("R4-R5認定住宅 = %d, want 350000", credit.Yen())
	}
}

func TestTotalHousingLoanCreditMixedGroups(t *testing.T) {
	// 重複適用グループ (105,900) + 単独明細 (140,000)
	purchase := usedHouseDetail(t, model.HousingUsed, 15_151_931, 42_800_000)
	renovation := usedHouseDetail(t, model.HousingRenovation, 15_151_931, 5_000_000)
	single := usedHouseDetail(t, model.HousingUsed, 25_000_000, 0)
	single.DualApplicationGroup = ""

	total, err := TotalHousingLoanCredit([]model.HousingLoanDetail{purchase, renovation, single}, 5_000_000)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if total != 245_900 {
		t.Errorf("= %d, want 245900", total.Yen())
	}
}

// 合計所得金額 2,000万円超は住宅ローン控除の適用外。
func TestTotalHousingLoanCreditIncomeLimit(t *testing.T) {
	single := usedHouseDetail(t, model.HousingUsed, 25_000_000, 0)
	single.DualApplicationGroup = ""
	details := []model.HousingLoanDetail{single}

	ok, err := TotalHousingLoanCredit(details, 20_000_000)
	if err != nil || ok != 140_000 {
		t.Errorf("所得2,000万ちょうどは適用: %d, err=%v", ok.Yen(), err)
	}
	over, err := TotalHousingLoanCredit(details, 20_000_001)
	if err != nil || over != 0 {
		t.Errorf("所得2,000万超は0: %d, err=%v", over.Yen(), err)
	}
}

// 住宅取得対価等が年末残高より小さい場合は対価が上限になる。
func TestHousingLoanCreditAcquisitionCostCap(t *testing.T) {
	d := usedHouseDetail(t, model.HousingUsed, 20_000_000, 0)
	d.DualApplicationGroup = ""
	d.AcquisitionCost = 10_000_000
	credit, err := HousingLoanCredit(&d)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if credit != 70_000 { // min(2000万, 1000万, 2000万)×0.7%
		t.Errorf("= %d, want 70000", credit.Yen())
	}
}

// 重複適用グループ内で年末残高が不一致ならエラー。
func TestHousingLoanCreditDualBalanceMismatch(t *testing.T) {
	a := usedHouseDetail(t, model.HousingUsed, 10_000_000, 30_000_000)
	b := usedHouseDetail(t, model.HousingRenovation, 9_999_999, 20_000_000)
	if _, _, err := HousingLoanCreditDual([]model.HousingLoanDetail{a, b}); err == nil {
		t.Error("残高不一致はエラーになるべき")
	}
}

// 配当控除: 課税所得1,000万の閾値をまたぐ場合は按分する。
func TestDividendCredit(t *testing.T) {
	tests := []struct {
		name              string
		dividend, taxable model.Money
		want              model.Money
	}{
		{"ゼロ", 0, 5_000_000, 0},
		{"1000万以下は10%", 500_000, 8_000_000, 50_000},
		{"閾値ぎりぎり10%", 500_000, 9_800_000, 50_000}, // 配当が全て1000万以下部分に対応
		{"閾値またぎは按分", 500_000, 10_200_000, 40_000}, // 30万×10% + 20万×5%
		{"1000万超は全額5%", 500_000, 12_000_000, 25_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DividendCredit(tt.dividend, tt.taxable); got != tt.want {
				t.Errorf("= %d, want %d", got.Yen(), tt.want.Yen())
			}
		})
	}
}
