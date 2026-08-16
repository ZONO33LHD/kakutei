// Package policy は税制定数 (令和7年分・令和8年分 = 2025・2026年課税年度) を一元管理する。
//
// 全ての税法関連の定数・速算表をこのパッケージに集約する。
// 金額は円単位の int64。税率は「分子/分母」の整数ペアで表現し浮動小数点は使わない。
// 年度によって異なる値は XxxFor(year) 関数で提供する。税法改正時はこのパッケージのみ更新すればよい。
package policy

// 対応する課税年度の範囲。税計算サービスは計算前に SupportsFiscalYear で照合し、
// 未対応年度への誤適用を防ぐ。
//
// 令和8年度税制改正 (令和8年12月1日施行、令和8年分以後適用) により、
// 基礎控除・給与所得控除・扶養親族等の所得要件は令和7年分と令和8年分で異なる。
// 年度依存の値は XxxFor(year) 関数で取得すること。
const (
	MinSupportedFiscalYear = 2025
	MaxSupportedFiscalYear = 2026
)

// SupportsFiscalYear は課税年度がこの税制定数群でサポートされるかを返す。
func SupportsFiscalYear(year int) bool {
	return year >= MinSupportedFiscalYear && year <= MaxSupportedFiscalYear
}

// BracketEntry は「上限額まで一定額」型テーブルの1行。
// Threshold 以下なら Value を採用する。
type BracketEntry struct {
	Threshold int64 // 上限 (この値以下なら適用)
	Value     int64 // 適用額
}

// LookupBracket はテーブルから income 以下の最初のエントリの値を返す。
// どのエントリにも該当しない (全上限超) 場合は fallback を返す。
func LookupBracket(table []BracketEntry, income int64, fallback int64) int64 {
	for _, e := range table {
		if income <= e.Threshold {
			return e.Value
		}
	}
	return fallback
}

// ============================================================
// 基礎控除 (所得税法第86条、租税特別措置法第41条の16の2)
// 令和7年分: 本則58万 + 令和7年度改正の加算特例
// 令和8年分: 本則62万 + 令和8年度改正の加算特例 (489万以下+42万、655万以下+5万)
// 2,350万超の区分 (48/32/16/0) は両年度共通。加算特例は居住者のみ。
// ============================================================

// basicDeductionTable2025 は令和7年分の基礎控除額。2,500万超は 0 円。
var basicDeductionTable2025 = []BracketEntry{
	{1_320_000, 950_000},  // ≤132万: 95万 (本則58万+加算37万)
	{3_360_000, 880_000},  // 132万超〜336万: 88万
	{4_890_000, 680_000},  // 336万超〜489万: 68万
	{6_550_000, 630_000},  // 489万超〜655万: 63万
	{23_500_000, 580_000}, // 655万超〜2,350万: 58万 (本則のみ)
	{24_000_000, 480_000}, // 2,350万超〜2,400万: 48万
	{24_500_000, 320_000}, // 2,400万超〜2,450万: 32万
	{25_000_000, 160_000}, // 2,450万超〜2,500万: 16万
}

// basicDeductionTable2026 は令和8年分の基礎控除額 (国税庁「令和8年4月
// 源泉所得税の改正のあらまし」⑴)。2,500万超は 0 円。
var basicDeductionTable2026 = []BracketEntry{
	{4_890_000, 1_040_000}, // ≤489万: 104万 (本則62万+加算42万)
	{6_550_000, 670_000},   // 489万超〜655万: 67万 (本則62万+加算5万)
	{23_500_000, 620_000},  // 655万超〜2,350万: 62万 (本則のみ)
	{24_000_000, 480_000},  // 2,350万超の区分は改正なし
	{24_500_000, 320_000},
	{25_000_000, 160_000},
}

// BasicDeductionTableFor は課税年度の基礎控除表を返す。
func BasicDeductionTableFor(year int) []BracketEntry {
	if year >= 2026 {
		return basicDeductionTable2026
	}
	return basicDeductionTable2025
}

// ============================================================
// 給与所得控除 (所得税法第28条第3項、租税特別措置法第29条の4)
// 令和7年分: 最低保障額65万。令和8年分: 最低保障額74万 (恒久69万+令和8・9年分の特例5万)
// ============================================================

// 給与所得金額は国税庁の速算表に従って直接計算する。令和7年分:
// 収入 660万円以下は A = 収入÷4 (千円未満切捨) を用いる端数規定がある:
//
//	≤190万:          給与所得 = 収入 − 65万 (マイナスは0)
//	190万超〜360万:   給与所得 = A×2.8 − 8万
//	360万超〜660万:   給与所得 = A×3.2 − 44万
//	660万超〜850万:   給与所得 = 収入×0.9 − 110万
//	850万超:          給与所得 = 収入 − 195万
const (
	SalaryDeductionMin = 650_000   // 最低保障額 (令和7年改正)
	SalaryDeductionMax = 1_950_000 // 上限 (給与収入850万超)

	SalaryDeductionBracket1 = 1_900_000 // ≤190万
	SalaryDeductionBracket2 = 3_600_000 // 190万超〜360万
	SalaryDeductionBracket3 = 6_600_000 // 360万超〜660万
	SalaryDeductionBracket4 = 8_500_000 // 660万超〜850万

	SalaryQuarterRoundingUnit = 1_000 // A = 収入÷4 の千円未満切捨て

	// 190万超〜360万: 所得 = A×28/10 − 80,000
	SalaryIncomeFactor2Num   = 28
	SalaryIncomeFactor2Denom = 10
	SalaryIncomeAdjust2      = 80_000

	// 360万超〜660万: 所得 = A×32/10 − 440,000
	SalaryIncomeFactor3Num   = 32
	SalaryIncomeFactor3Denom = 10
	SalaryIncomeAdjust3      = 440_000

	// 660万超〜850万: 所得 = 収入×9/10 − 1,100,000
	SalaryIncomeFactor4Num   = 9
	SalaryIncomeFactor4Denom = 10
	SalaryIncomeAdjust4      = 1_100_000
)

// 令和8・9年分の給与所得の特例 (国税庁「令和8年4月 源泉所得税の改正のあらまし」⑵ハ):
// 収入 220万円未満は最低保障74万を基礎とした次の速算になる。220万円以上は改正なし。
//
//	74.1万未満:              所得 0
//	74.1万以上219.1万未満:    収入 − 74万
//	219.1万以上219.3万未満:   145.1万
//	219.3万以上219.6万未満:   145.3万
//	219.6万以上220万未満:     145.6万
const (
	SalaryDeductionMin2026 = 740_000 // 最低保障額 (恒久69万 + 令和8・9年分の特例5万)

	Salary2026Step0Max = 741_000   // これ未満は所得0
	Salary2026Step1Max = 2_191_000 // これ未満は 収入−74万
	Salary2026Step2Max = 2_193_000 // これ未満は 145.1万
	Salary2026Step3Max = 2_196_000 // これ未満は 145.3万
	Salary2026Step4Max = 2_200_000 // これ未満は 145.6万

	Salary2026Step2Income = 1_451_000
	Salary2026Step3Income = 1_453_000
	Salary2026Step4Income = 1_456_000
)

// 所得金額調整控除 (子ども・特別障害者等、租税特別措置法第41条の3の11)。
// 給与収入 850万超で、本人が特別障害者/23歳未満の扶養親族がいる/
// 特別障害者の同一生計配偶者・扶養親族がいる場合:
//
//	控除額 = (min(給与収入, 1,000万) − 850万) × 10% (1円未満切上げ)
const (
	SalaryAdjustmentThreshold  = 8_500_000  // 適用開始の給与収入
	SalaryAdjustmentRevenueCap = 10_000_000 // 収入の上限 (これ以上は1,000万として計算)
	SalaryAdjustmentRatePct    = 10
	SalaryAdjustmentChildAge   = 23 // 「23歳未満の扶養親族」の判定年齢
)

// ============================================================
// 生命保険料控除 (所得税法第76条)
// ============================================================

const (
	// 新制度 (平成24年1月1日以後の契約)
	LifeInsuranceNewMax      = 40_000 // 新制度1区分上限
	LifeInsuranceNewBracket1 = 20_000 // ≤2万: 全額
	LifeInsuranceNewBracket2 = 40_000 // 2万超〜4万: 支払額/2+10,000
	LifeInsuranceNewBracket3 = 80_000 // 4万超〜8万: 支払額/4+20,000

	// 旧制度 (平成23年12月31日以前の契約)
	LifeInsuranceOldMax      = 50_000  // 旧制度1区分上限
	LifeInsuranceOldBracket1 = 25_000  // ≤2.5万: 全額
	LifeInsuranceOldBracket2 = 50_000  // 2.5万超〜5万: 支払額/2+12,500
	LifeInsuranceOldBracket3 = 100_000 // 5万超〜10万: 支払額/4+25,000

	LifeInsuranceCombinedMax = 40_000  // 新旧合算1区分上限
	LifeInsuranceTotalMax    = 120_000 // 3区分合計上限
)

// 令和8年分限定の子育て世帯特例 (令和7年度改正):
// 23歳未満の扶養親族を有する場合、一般生命保険料 (新制度) の上限を6万円に拡充する。
//
//	≤3万: 全額 / 3万超〜6万: /2+15,000 / 6万超〜12万: /4+30,000 / 12万超: 60,000
//
// 新旧併用時の一般枠上限も6万円になる。3区分合計の12万円上限は不変。
const (
	LifeInsuranceChildcareYear       = 2026    // 特例の適用年度 (令和8年分のみ)
	LifeInsuranceNewMaxExpanded      = 60_000  // 拡充後の一般 (新) 上限
	LifeInsuranceExpandedBracket1    = 30_000  // ≤3万: 全額
	LifeInsuranceExpandedBracket2    = 60_000  // 3万超〜6万: /2+15,000
	LifeInsuranceExpandedBracket3    = 120_000 // 6万超〜12万: /4+30,000
	LifeInsuranceExpandedAdd2        = 15_000
	LifeInsuranceExpandedAdd3        = 30_000
	LifeInsuranceCombinedMaxExpanded = 60_000 // 特例適用時の新旧合算一般枠上限
)

// ============================================================
// 地震保険料控除 (所得税法第77条)
// ============================================================

const (
	EarthquakeInsuranceMax = 50_000 // 地震保険料控除の上限
	OldLongTermMax         = 15_000 // 旧長期損害保険料控除の上限
	OldLongTermBracket1    = 5_000  // ≤5,000: 全額
	OldLongTermBracket2    = 15_000 // 5,000超〜15,000: 支払額/2+2,500
)

// ============================================================
// 人的控除
// ============================================================

const (
	// 寡婦控除・ひとり親控除 (所得税法第81条・第81条の2)
	WidowDeduction             = 270_000   // 寡婦控除
	SingleParentDeduction      = 350_000   // ひとり親控除 (38万への引上げは令和9年分以後のため対象外)
	PersonalDeductionIncomeMax = 5_000_000 // 人的控除の所得制限

	// 障害者控除 (所得税法第79条)
	DisabilityGeneral           = 270_000 // 一般障害者
	DisabilitySpecial           = 400_000 // 特別障害者
	DisabilitySpecialCohabiting = 750_000 // 同居特別障害者

	// 勤労学生控除 (所得税法第82条)
	WorkingStudentDeduction = 270_000
)

// WorkingStudentIncomeMaxFor は勤労学生控除の所得要件を返す
// (令和7年分: 85万、令和8年分以後: 89万)。
func WorkingStudentIncomeMaxFor(year int) int64 {
	if year >= 2026 {
		return 890_000
	}
	return 850_000
}

// ============================================================
// 扶養控除 (所得税法第84条)
// ============================================================

const (
	DependentGeneral           = 380_000 // 一般扶養 (16歳以上)
	DependentSpecific          = 630_000 // 特定扶養 (19歳以上23歳未満)
	DependentElderly           = 480_000 // 老人扶養 (70歳以上、別居)
	DependentElderlyCohabiting = 580_000 // 老人扶養 (70歳以上、同居)

	// 年齢境界 (年度末時点)
	DependentAgeMin         = 16 // 扶養控除の最低年齢
	DependentAgeSpecificMin = 19 // 特定扶養の最低年齢
	DependentAgeSpecificMax = 23 // 特定扶養の最大年齢 (未満)
	DependentAgeElderly     = 70 // 老人扶養の最低年齢
)

// DependentIncomeMaxFor は扶養親族・同一生計配偶者・ひとり親の子の所得要件を返す
// (令和7年分: 58万、令和8年分以後: 62万)。
func DependentIncomeMaxFor(year int) int64 {
	if year >= 2026 {
		return 620_000
	}
	return 580_000
}

// SpecificRelativeSpecialDeductionTable は特定親族特別控除
// (令和7年新設、租税特別措置法第41条の17)。
// 19〜22歳の親族で所得が扶養要件超〜123万以下に段階適用する
// (下限は DependentIncomeMaxFor に連動: 令和7年分 58万超、令和8年分 62万超。
// 控除額の区分は両年度共通)。
var SpecificRelativeSpecialDeductionTable = []BracketEntry{
	{850_000, 630_000},   // 〜85万: 63万
	{900_000, 610_000},   // 85万超〜90万: 61万
	{950_000, 510_000},   // 90万超〜95万: 51万
	{1_000_000, 410_000}, // 95万超〜100万: 41万
	{1_050_000, 310_000}, // 100万超〜105万: 31万
	{1_100_000, 210_000}, // 105万超〜110万: 21万
	{1_150_000, 110_000}, // 110万超〜115万: 11万
	{1_200_000, 60_000},  // 115万超〜120万: 6万
	{1_230_000, 30_000},  // 120万超〜123万: 3万
}

// SpecificRelativeSpecialIncomeMax を超える所得の特定親族には控除なし。
const SpecificRelativeSpecialIncomeMax = 1_230_000

// ============================================================
// 医療費控除 (所得税法第73条)
// ============================================================

const (
	MedicalExpenseThreshold      = 100_000   // 足切り額 (または所得の5%の低い方)
	MedicalExpenseIncomeRatioPct = 5         // 所得の5%
	MedicalExpenseMax            = 2_000_000 // 控除額の上限

	// セルフメディケーション税制 (租税特別措置法第41条の17の2)
	SelfMedicationThreshold = 12_000
	SelfMedicationMax       = 88_000
)

// ============================================================
// 一時所得 (所得税法第34条)
// ============================================================

// OneTimeIncomeSpecialDeduction は一時所得の特別控除額。
const OneTimeIncomeSpecialDeduction = 500_000

// ============================================================
// 配偶者控除 / 配偶者特別控除 (所得税法第83条・第83条の2)
// 令和7年改正: 配偶者所得要件拡大 (48万→58万で配偶者控除)
// ============================================================

// SpouseTaxpayerIncomeMax を超える所得の納税者には配偶者控除なし。
const SpouseTaxpayerIncomeMax = 10_000_000

// SpouseDeductionTable は納税者所得 ≤900万 の配偶者(特別)控除。
var SpouseDeductionTable = []BracketEntry{
	{580_000, 380_000},   // ≤58万: 配偶者控除 38万
	{950_000, 380_000},   // 58万超〜95万: 配偶者特別控除 38万 (満額)
	{1_000_000, 360_000}, // 95万超〜100万: 36万
	{1_050_000, 310_000},
	{1_100_000, 260_000},
	{1_150_000, 210_000},
	{1_200_000, 160_000},
	{1_250_000, 110_000},
	{1_300_000, 60_000},
	{1_330_000, 30_000},
}

// SpouseDeductionTable9M は納税者所得 900万超〜950万。
var SpouseDeductionTable9M = []BracketEntry{
	{580_000, 260_000},
	{950_000, 260_000},
	{1_000_000, 240_000},
	{1_050_000, 210_000},
	{1_100_000, 180_000},
	{1_150_000, 140_000},
	{1_200_000, 110_000},
	{1_250_000, 80_000},
	{1_300_000, 40_000},
	{1_330_000, 20_000},
}

// SpouseDeductionTable10M は納税者所得 950万超〜1,000万。
var SpouseDeductionTable10M = []BracketEntry{
	{580_000, 130_000},
	{950_000, 130_000},
	{1_000_000, 120_000},
	{1_050_000, 110_000},
	{1_100_000, 90_000},
	{1_150_000, 70_000},
	{1_200_000, 60_000},
	{1_250_000, 40_000},
	{1_300_000, 20_000},
	{1_330_000, 10_000},
}

const (
	SpouseTaxpayerBracket1 = 9_000_000 // ≤900万
	SpouseTaxpayerBracket2 = 9_500_000 // 900万超〜950万
)

// 老人控除対象配偶者 (70歳以上・所得58万以下) の配偶者控除額 (所得税法第83条)。
// 納税者の合計所得金額の区分ごとに一般配偶者より 10万〜3万円高い。
const (
	SpouseElderlyDeduction    = 480_000 // 納税者所得 ≤900万
	SpouseElderlyDeduction9M  = 320_000 // 900万超〜950万
	SpouseElderlyDeduction10M = 160_000 // 950万超〜1,000万
	SpouseElderlyAgeMin       = 70      // 老人控除対象配偶者の年齢下限 (年度末・税法上の年齢)
)

// ============================================================
// 寄附金控除 (所得税法第78条・租税特別措置法第41条の18等)
// ============================================================

const (
	DonationSelfBurden              = 2_000 // 自己負担額
	DonationIncomeDeductionRatioPct = 40    // 所得控除上限: 総所得金額等の40%

	PoliticalDonationCreditRatePct = 30 // 政治活動寄附金の税額控除率
	PoliticalDonationCreditCapPct  = 25 // 所得税額の25%上限
	NPODonationCreditRatePct       = 40 // 認定NPO等の税額控除率
	NPODonationCreditCapPct        = 25 // 所得税額の25%上限
)

// ============================================================
// ふるさと納税 (所得税法第78条)
// ============================================================

const (
	FurusatoSelfBurden          = 2_000 // 自己負担額
	FurusatoIncomeRatioPct      = 40    // 総所得金額等の40%上限
	FurusatoResidentTaxRatioPct = 20    // 住民税所得割額の20%
	ResidentTaxRatePct          = 10    // 住民税所得割の標準税率
)

// ============================================================
// 住宅ローン控除 (租税特別措置法第41条)
// 令和4年以降入居
// ============================================================

const (
	HousingLoanRateNum   = 7    // 控除率 0.7% = 7/1000
	HousingLoanRateDenom = 1000 // 控除率の分母
)

// HousingCategory は住宅性能区分。
type HousingCategory string

const (
	HousingGeneral         HousingCategory = "general"          // 一般住宅
	HousingCertified       HousingCategory = "certified"        // 認定住宅 (長期優良/低炭素)
	HousingZEH             HousingCategory = "zeh"              // ZEH水準省エネ住宅
	HousingEnergyEfficient HousingCategory = "energy_efficient" // 省エネ基準適合住宅
)

// Valid は定義済みの住宅性能区分かどうか。
func (c HousingCategory) Valid() bool {
	switch c {
	case HousingGeneral, HousingCertified, HousingZEH, HousingEnergyEfficient:
		return true
	}
	return false
}

// HousingCategoryKey は住宅性能区分 × 新築/中古の組み合わせ。
type HousingCategoryKey struct {
	Category        HousingCategory
	NewConstruction bool
}

// HousingLoanLimitsR4R5 は令和4〜5年入居の年末残高上限。
var HousingLoanLimitsR4R5 = map[HousingCategoryKey]int64{
	{"certified", true}:         50_000_000, // 認定住宅 (長期優良/低炭素)
	{"zeh", true}:               45_000_000, // ZEH水準省エネ住宅
	{"energy_efficient", true}:  40_000_000, // 省エネ基準適合住宅
	{"general", true}:           30_000_000, // 一般住宅
	{"certified", false}:        30_000_000,
	{"zeh", false}:              30_000_000,
	{"energy_efficient", false}: 30_000_000,
	{"general", false}:          20_000_000,
}

// HousingLoanLimitsR6R7 は令和6〜7年入居 (一般世帯) の年末残高上限。
// 令和5年度税制改正で一般世帯は上限引下げ。
var HousingLoanLimitsR6R7 = map[HousingCategoryKey]int64{
	{"certified", true}:         45_000_000,
	{"zeh", true}:               35_000_000,
	{"energy_efficient", true}:  30_000_000,
	{"general", true}:           0, // 一般住宅新築は原則対象外
	{"certified", false}:        30_000_000,
	{"zeh", false}:              30_000_000,
	{"energy_efficient", false}: 30_000_000,
	{"general", false}:          20_000_000,
}

// HousingLoanLimitsR6R7Childcare は令和6〜7年入居 (子育て世帯・若者夫婦世帯)。
// 従来水準を維持する。
var HousingLoanLimitsR6R7Childcare = map[HousingCategoryKey]int64{
	{"certified", true}:         50_000_000,
	{"zeh", true}:               45_000_000,
	{"energy_efficient", true}:  40_000_000,
	{"general", true}:           0, // 子育て世帯でも一般住宅新築は対象外
	{"certified", false}:        30_000_000,
	{"zeh", false}:              30_000_000,
	{"energy_efficient", false}: 30_000_000,
	{"general", false}:          20_000_000,
}

// HousingLoanLimitsR8 は令和8年入居 (一般世帯) の年末残高上限
// (令和8年度税制改正で適用期限は令和12年末まで延長されたが、
// 令和9年以降入居は省エネ基準適合新築の経過措置等の追加要件があるため、
// 対応課税年度の拡張時に一次資料を確認して別表として追加する)。
// 一般住宅の新築等は対象外。中古は認定・ZEHが引上げ、省エネ適合は引下げ。
var HousingLoanLimitsR8 = map[HousingCategoryKey]int64{
	{"certified", true}:         45_000_000,
	{"zeh", true}:               35_000_000,
	{"energy_efficient", true}:  20_000_000,
	{"general", true}:           0,
	{"certified", false}:        35_000_000,
	{"zeh", false}:              35_000_000,
	{"energy_efficient", false}: 20_000_000,
	{"general", false}:          20_000_000,
}

// HousingLoanLimitsR8Childcare は令和8年入居 (子育て世帯・若者夫婦世帯)。
// 中古にも上乗せが適用される (一般中古は上乗せなし)。
var HousingLoanLimitsR8Childcare = map[HousingCategoryKey]int64{
	{"certified", true}:         50_000_000,
	{"zeh", true}:               45_000_000,
	{"energy_efficient", true}:  30_000_000,
	{"general", true}:           0,
	{"certified", false}:        45_000_000,
	{"zeh", false}:              45_000_000,
	{"energy_efficient", false}: 30_000_000,
	{"general", false}:          20_000_000,
}

const (
	// HousingLoanRenovationLimit は増改築等の年末残高上限 (性能区分によらず一律、控除期間10年)。
	HousingLoanRenovationLimit = 20_000_000
	// HousingLoanGeneralR5Confirmed は R5 以前建築確認済み一般住宅新築の特例上限
	// (令和6〜7年入居のみの措置)。
	HousingLoanGeneralR5Confirmed = 20_000_000
	// HousingLoanR4R5LastYear はこの年以前の入居に R4-R5 テーブルを適用する。
	HousingLoanR4R5LastYear = 2023
	// HousingLoanR6R7LastYear はこの年以前の入居に R6-R7 テーブルを適用する。
	HousingLoanR6R7LastYear = 2025
	// HousingLoanTableLastYear はテーブルを実装済みの最終入居年 (令和8年末)。
	// 対応課税年度 (令和7・8年分) の申告に令和9年以降の入居は現れない。
	// これより後の入居分は誤適用を防ぐため計算をエラーで拒否する。
	HousingLoanTableLastYear = 2026
	// HousingLoanIncomeLimit は適用要件の合計所得金額上限 (令和4年以降入居)。
	HousingLoanIncomeLimit = 20_000_000
)

// ============================================================
// 所得税速算表 (所得税法第89条)
// ============================================================

// IncomeTaxBracket は速算表の1行: 課税所得 Threshold 以下 → 税額 = 課税所得×RatePct/100 − Deduction。
type IncomeTaxBracket struct {
	Threshold int64
	RatePct   int64
	Deduction int64
}

// IncomeTaxTable は課税総所得金額に対する速算表。4,000万超は Top 定数を使う。
var IncomeTaxTable = []IncomeTaxBracket{
	{1_950_000, 5, 0},
	{3_300_000, 10, 97_500},
	{6_950_000, 20, 427_500},
	{9_000_000, 23, 636_000},
	{18_000_000, 33, 1_536_000},
	{40_000_000, 40, 2_796_000},
}

const (
	IncomeTaxTopRatePct   = 45        // 4,000万超の税率
	IncomeTaxTopDeduction = 4_796_000 // 4,000万超の控除額
)

// ============================================================
// 復興特別所得税 (復興財源確保法第13条)。令和19年 (2037年) まで。
// ============================================================

const (
	ReconstructionTaxRateNum   = 21 // 2.1% = 21/1000
	ReconstructionTaxRateDenom = 1000
)

// ============================================================
// 配当控除 (所得税法第92条)
// ============================================================

const (
	DividendCreditRateLowPct  = 10         // 課税所得1,000万以下: 配当の10%
	DividendCreditRateHighPct = 5          // 課税所得1,000万超: 配当の5%
	DividendCreditThreshold   = 10_000_000 // 税率が変わる閾値
)

// ============================================================
// 青色申告特別控除 (租税特別措置法第25条の2)
// ============================================================

const (
	BlueReturnDeduction65 = 650_000 // e-Tax 提出 or 電子帳簿保存
	BlueReturnDeduction55 = 550_000 // 書面提出 (正規の簿記の原則)
	BlueReturnDeduction10 = 100_000 // 簡易帳簿
)

// ============================================================
// 端数処理 (国税通則法第118条・第119条)
// ============================================================

const (
	TaxableIncomeRoundingUnit = 1_000 // 課税所得: 1,000円未満切捨て
	TaxAmountRoundingUnit     = 100   // 確定金額 (納付時): 100円未満切捨て
)

// ============================================================
// 予定納税 (所得税法第104条)
// ============================================================

// EstimatedTaxThreshold は予定納税の基準額。
const EstimatedTaxThreshold = 150_000
