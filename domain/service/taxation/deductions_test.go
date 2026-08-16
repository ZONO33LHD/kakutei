package taxation

import (
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/model"
)

func mustDate(t *testing.T, s string) model.Date {
	t.Helper()
	d, err := model.ParseDate(s)
	if err != nil {
		t.Fatalf("ParseDate(%q): %v", s, err)
	}
	return d
}

func TestBasicDeduction(t *testing.T) {
	tests := []struct {
		income model.Money
		want   model.Money
	}{
		{1_000_000, 950_000},
		{3_000_000, 880_000},
		{5_000_000, 630_000},
		{20_000_000, 580_000},
		{24_800_000, 160_000},
		{25_000_001, 0},
	}
	for _, tt := range tests {
		if got := BasicDeduction(tt.income, 2025); got != tt.want {
			t.Errorf("BasicDeduction(%d) = %d, want %d", tt.income.Yen(), got.Yen(), tt.want.Yen())
		}
	}
}

// 令和8年分の基礎控除 (本則62万+加算特例)。
func TestBasicDeduction2026(t *testing.T) {
	tests := []struct {
		income model.Money
		want   model.Money
	}{
		{1_000_000, 1_040_000},
		{4_890_000, 1_040_000},
		{4_890_001, 670_000},
		{6_550_000, 670_000},
		{6_550_001, 620_000},
		{23_500_000, 620_000},
		{23_500_001, 480_000}, // 2,350万超の区分は改正なし
		{25_000_001, 0},
	}
	for _, tt := range tests {
		if got := BasicDeduction(tt.income, 2026); got != tt.want {
			t.Errorf("BasicDeduction2026(%d) = %d, want %d", tt.income.Yen(), got.Yen(), tt.want.Yen())
		}
	}
}

// 生命保険料控除 (所得税法第76条)。端数は切り上げ。
func TestLifeInsuranceDeduction(t *testing.T) {
	tests := []struct {
		name string
		p    model.LifeInsurancePremiums
		want model.Money
	}{
		{"ゼロ", model.LifeInsurancePremiums{}, 0},
		{"新制度2万以下は全額", model.LifeInsurancePremiums{GeneralNew: 15_000}, 15_000},
		{"新制度2万超は半額+1万 (切上げ)", model.LifeInsurancePremiums{GeneralNew: 30_001}, 25_001}, // ceil(30001/2)=15001
		{"新制度8万超は上限4万", model.LifeInsurancePremiums{GeneralNew: 100_000}, 40_000},
		{"旧制度10万超は上限5万", model.LifeInsurancePremiums{GeneralOld: 120_000}, 50_000},
		{"旧のみ5万は半額+1.25万", model.LifeInsurancePremiums{GeneralOld: 50_000}, 37_500},
		// 新旧併用: max(新のみ, 旧のみ, min(合算, 4万))。旧が有利なら旧を採用
		{"新旧併用で旧が有利", model.LifeInsurancePremiums{GeneralNew: 10_000, GeneralOld: 120_000}, 50_000},
		{"3区分合計は12万上限", model.LifeInsurancePremiums{
			GeneralNew: 100_000, MedicalCare: 100_000, AnnuityNew: 100_000, AnnuityOld: 120_000,
		}, 120_000},
		{"介護医療は新制度のみで上限4万", model.LifeInsurancePremiums{MedicalCare: 90_000}, 40_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LifeInsuranceDeduction(tt.p, 2025, false); got != tt.want {
				t.Errorf("= %d, want %d", got.Yen(), tt.want.Yen())
			}
		})
	}
}

// 令和8年分限定・子育て世帯特例: 23歳未満の扶養親族ありで一般 (新) の上限が6万に拡充。
func TestLifeInsuranceDeductionChildcare2026(t *testing.T) {
	tests := []struct {
		name  string
		p     model.LifeInsurancePremiums
		year  model.FiscalYear
		young bool
		want  model.Money
	}{
		{"特例: 12万超は上限6万", model.LifeInsurancePremiums{GeneralNew: 130_000}, 2026, true, 60_000},
		{"特例: 3万以下は全額", model.LifeInsurancePremiums{GeneralNew: 28_000}, 2026, true, 28_000},
		{"特例: 3万超6万以下は/2+1.5万", model.LifeInsurancePremiums{GeneralNew: 50_000}, 2026, true, 40_000},
		{"特例: 6万超12万以下は/4+3万", model.LifeInsurancePremiums{GeneralNew: 100_000}, 2026, true, 55_000},
		{"特例: 新旧併用の一般枠上限も6万", model.LifeInsurancePremiums{GeneralNew: 80_000, GeneralOld: 100_000}, 2026, true, 60_000},
		{"特例: 合計12万上限は不変", model.LifeInsurancePremiums{
			GeneralNew: 130_000, MedicalCare: 100_000, AnnuityNew: 100_000,
		}, 2026, true, 120_000},
		{"扶養親族なしは通常上限4万", model.LifeInsurancePremiums{GeneralNew: 130_000}, 2026, false, 40_000},
		{"令和7年分は特例なし", model.LifeInsurancePremiums{GeneralNew: 130_000}, 2025, true, 40_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LifeInsuranceDeduction(tt.p, tt.year, tt.young); got != tt.want {
				t.Errorf("= %d, want %d", got.Yen(), tt.want.Yen())
			}
		})
	}
}

func TestEarthquakeInsuranceDeduction(t *testing.T) {
	tests := []struct {
		name       string
		earthquake model.Money
		oldLong    model.Money
		want       model.Money
	}{
		{"ゼロ", 0, 0, 0},
		{"地震のみ上限内", 30_000, 0, 30_000},
		{"地震のみ上限超", 80_000, 0, 50_000},
		{"旧長期5千以下は全額", 0, 4_000, 4_000},
		{"旧長期1万は半額+2500 (切上げ)", 0, 10_001, 7_501},
		{"旧長期1.5万超は上限1.5万", 0, 30_000, 15_000},
		{"合算は5万上限", 45_000, 15_000, 50_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EarthquakeInsuranceDeduction(tt.earthquake, tt.oldLong); got != tt.want {
				t.Errorf("= %d, want %d", got.Yen(), tt.want.Yen())
			}
		})
	}
}

func TestMedicalDeduction(t *testing.T) {
	tests := []struct {
		name             string
		expenses, income model.Money
		want             model.Money
	}{
		{"足切り10万以下は0", 100_000, 5_000_000, 0},
		{"10万超は超過分", 300_000, 5_000_000, 200_000},
		{"低所得は所得5%が足切り", 60_000, 1_000_000, 10_000}, // 足切り5万
		{"上限200万", 3_000_000, 10_000_000, 2_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MedicalDeduction(tt.expenses, tt.income); got != tt.want {
				t.Errorf("= %d, want %d", got.Yen(), tt.want.Yen())
			}
		})
	}
}

func TestSelfMedicationDeduction(t *testing.T) {
	if got := SelfMedicationDeduction(12_000); got != 0 {
		t.Errorf("閾値以下は0: got %d", got.Yen())
	}
	if got := SelfMedicationDeduction(50_000); got != 38_000 {
		t.Errorf("= %d, want 38000", got.Yen())
	}
	if got := SelfMedicationDeduction(200_000); got != 88_000 {
		t.Errorf("上限88,000: got %d", got.Yen())
	}
}

// 配偶者控除 / 配偶者特別控除 / 老人控除対象配偶者。
func TestSpouseDeduction(t *testing.T) {
	year := model.FiscalYear(2025)
	young := mustDate(t, "1990-05-01")
	elderly := mustDate(t, "1955-01-01") // 令和7年末で70歳 (税法年齢71)

	tests := []struct {
		name           string
		spouse         *model.Spouse
		taxpayerIncome model.Money
		want           model.Money
	}{
		{"配偶者なし", nil, 5_000_000, 0},
		{"一般・所得58万以下", &model.Spouse{Name: "花子", BirthDate: young, Income: 500_000}, 5_000_000, 380_000},
		{"特別控除満額", &model.Spouse{Name: "花子", BirthDate: young, Income: 900_000}, 5_000_000, 380_000},
		{"特別控除逓減 (110万ちょうど)", &model.Spouse{Name: "花子", BirthDate: young, Income: 1_100_000}, 5_000_000, 260_000},
		{"特別控除逓減 (110万超)", &model.Spouse{Name: "花子", BirthDate: young, Income: 1_100_001}, 5_000_000, 210_000},
		{"133万超は0", &model.Spouse{Name: "花子", BirthDate: young, Income: 1_400_000}, 5_000_000, 0},
		{"納税者1000万超は0", &model.Spouse{Name: "花子", BirthDate: young, Income: 0}, 10_000_001, 0},
		{"納税者900万超は26万", &model.Spouse{Name: "花子", BirthDate: young, Income: 0}, 9_200_000, 260_000},
		{"老人配偶者48万", &model.Spouse{Name: "梅", BirthDate: elderly, Income: 0}, 5_000_000, 480_000},
		{"老人配偶者・納税者900万超は32万", &model.Spouse{Name: "梅", BirthDate: elderly, Income: 0}, 9_200_000, 320_000},
		{"老人でも所得58万超なら特別控除側", &model.Spouse{Name: "梅", BirthDate: elderly, Income: 900_000}, 5_000_000, 380_000},
		{"他の納税者の扶養は0", &model.Spouse{Name: "花子", BirthDate: young, Income: 0, OtherTaxpayerDependent: true}, 5_000_000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SpouseDeduction(tt.spouse, tt.taxpayerIncome, year); got != tt.want {
				t.Errorf("= %d, want %d", got.Yen(), tt.want.Yen())
			}
		})
	}
}

// 扶養控除 + 特定親族特別控除 + 障害者控除。年齢は税法上の満年齢 (前日加齢)。
func TestDependentDeductions(t *testing.T) {
	year := model.FiscalYear(2025)

	sum := func(deps []model.Dependent) model.Money {
		return sumItems(DependentDeductions(deps, year))
	}

	t.Run("16歳境界は前日加齢で判定", func(t *testing.T) {
		// 2010-01-01生まれ → 2025-12-31に (税法上) 16歳 → 一般扶養38万
		d := model.Dependent{Name: "子", BirthDate: mustDate(t, "2010-01-01")}
		if got := sum([]model.Dependent{d}); got != 380_000 {
			t.Errorf("= %d, want 380000", got.Yen())
		}
		// 2010-01-02生まれ → 15歳 → 控除なし
		d2 := model.Dependent{Name: "子", BirthDate: mustDate(t, "2010-01-02")}
		if got := sum([]model.Dependent{d2}); got != 0 {
			t.Errorf("= %d, want 0", got.Yen())
		}
	})

	t.Run("特定扶養と特定親族特別控除", func(t *testing.T) {
		base := model.Dependent{Name: "大学生", BirthDate: mustDate(t, "2004-06-01")} // 21歳
		if got := sum([]model.Dependent{base}); got != 630_000 {
			t.Errorf("特定扶養 = %d, want 630000", got.Yen())
		}
		withIncome := base
		withIncome.Income = 900_000 // 58万超〜90万 → 特定親族特別控除61万
		if got := sum([]model.Dependent{withIncome}); got != 610_000 {
			t.Errorf("特定親族特別控除 = %d, want 610000", got.Yen())
		}
		over := base
		over.Income = 1_300_000 // 123万超 → 0
		if got := sum([]model.Dependent{over}); got != 0 {
			t.Errorf("123万超 = %d, want 0", got.Yen())
		}
	})

	t.Run("老人扶養", func(t *testing.T) {
		parent := model.Dependent{
			Name: "母", BirthDate: mustDate(t, "1950-03-01"),
			Cohabiting: true, DirectAscendant: true,
		}
		if got := sum([]model.Dependent{parent}); got != 580_000 {
			t.Errorf("同居老親等 = %d, want 580000", got.Yen())
		}
		parent.Cohabiting = false
		if got := sum([]model.Dependent{parent}); got != 480_000 {
			t.Errorf("別居 = %d, want 480000", got.Yen())
		}
		// 直系尊属でない同居の老人扶養親族 (兄姉等) は58万ではなく48万
		sibling := model.Dependent{
			Name: "兄", BirthDate: mustDate(t, "1950-03-01"), Cohabiting: true,
		}
		if got := sum([]model.Dependent{sibling}); got != 480_000 {
			t.Errorf("直系尊属でない同居老人 = %d, want 480000", got.Yen())
		}
	})

	t.Run("障害者控除は年齢制限なし", func(t *testing.T) {
		// 10歳 (扶養控除なし) でも障害者控除は適用
		child := model.Dependent{
			Name: "子", BirthDate: mustDate(t, "2015-06-01"),
			Disability: model.DisabilitySpecialCohabiting, Cohabiting: true,
		}
		if got := sum([]model.Dependent{child}); got != 750_000 {
			t.Errorf("= %d, want 750000", got.Yen())
		}
	})

	t.Run("所得58万超の特定親族に障害者控除は付かない", func(t *testing.T) {
		// 扶養親族の所得要件 (58万以下) を満たさないため、
		// 特定親族特別控除 61万のみで障害者控除は加算しない
		d := model.Dependent{
			Name: "大学生", BirthDate: mustDate(t, "2004-06-01"),
			Income: 900_000, Disability: model.DisabilityGeneral,
		}
		if got := sum([]model.Dependent{d}); got != 610_000 {
			t.Errorf("= %d, want 610000", got.Yen())
		}
	})

	t.Run("所得超過と二重控除の除外", func(t *testing.T) {
		overIncome := model.Dependent{Name: "兄", BirthDate: mustDate(t, "1995-01-01"), Income: 600_000}
		other := model.Dependent{Name: "弟", BirthDate: mustDate(t, "1998-01-01"), OtherTaxpayerDependent: true}
		if got := sum([]model.Dependent{overIncome, other}); got != 0 {
			t.Errorf("= %d, want 0", got.Yen())
		}
	})
}

func TestWidowAndStudentDeductions(t *testing.T) {
	if got := WidowDeduction(model.WidowSingleParent, 4_000_000, 2025); got != 350_000 {
		t.Errorf("ひとり親 = %d", got.Yen())
	}
	if got := WidowDeduction(model.WidowWidow, 4_000_000, 2025); got != 270_000 {
		t.Errorf("寡婦 = %d", got.Yen())
	}
	if got := WidowDeduction(model.WidowSingleParent, 5_000_001, 2025); got != 0 {
		t.Errorf("所得500万超 = %d", got.Yen())
	}
	if got := WorkingStudentDeduction(true, 850_000, 2025); got != 270_000 {
		t.Errorf("勤労学生 = %d", got.Yen())
	}
	if got := WorkingStudentDeduction(true, 850_001, 2025); got != 0 {
		t.Errorf("= %d, want 0", got.Yen())
	}
	// 令和8年分は所得要件 89万以下
	if got := WorkingStudentDeduction(true, 890_000, 2026); got != 270_000 {
		t.Errorf("2026年89万 = %d, want 270000", got.Yen())
	}
	if got := WorkingStudentDeduction(true, 890_001, 2026); got != 0 {
		t.Errorf("所得85万超 = %d", got.Yen())
	}
	if got := SelfDisabilityDeduction(model.DisabilityGeneral); got != 270_000 {
		t.Errorf("本人一般障害 = %d", got.Yen())
	}
	if got := SelfDisabilityDeduction(model.DisabilitySpecial); got != 400_000 {
		t.Errorf("本人特別障害 = %d", got.Yen())
	}
}

func TestDonationIncomeDeduction(t *testing.T) {
	if got := DonationIncomeDeduction(2_000, 5_000_000); got != 0 {
		t.Errorf("足切り以下 = %d", got.Yen())
	}
	if got := DonationIncomeDeduction(50_000, 5_000_000); got != 48_000 {
		t.Errorf("= %d, want 48000", got.Yen())
	}
	// 40%上限: 所得10万 → 上限4万 → 4万−2千 = 3.8万
	if got := DonationIncomeDeduction(100_000, 100_000); got != 38_000 {
		t.Errorf("40%%上限 = %d, want 38000", got.Yen())
	}
}
