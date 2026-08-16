package policy

// ============================================================
// 公的年金等控除 (所得税法第35条、令和2年分以後の税制)
// ※ 令和7年改正は基礎控除・給与所得控除のみ。公的年金等控除は改正なし。
// 参照: 国税庁 令和7年分 公的年金等に係る雑所得の速算表
// ============================================================

// PensionDeductionBracket は速算表の1行。
// 年金収入 Threshold 以下 → 控除額 = 年金×RatePct/100 + Fixed。
// RatePct=0 は「一律 Fixed 円」。
//
// 最低保障区分 (65歳未満 60万 / 65歳以上 110万) は固定額として表現し、
// 適用時に「控除額 ≤ 年金収入」でキャップする。他所得による減額 (−10万/−20万) を
// 固定部分に適用した後でも、収入以下にキャップすることで国税庁の3列テーブルと一致する。
type PensionDeductionBracket struct {
	Threshold int64
	RatePct   int64
	Fixed     int64
}

// PensionDeductionUnder65 は 65歳未満のテーブル。
var PensionDeductionUnder65 = []PensionDeductionBracket{
	{1_300_000, 0, 600_000},    // ≤130万: 60万 (収入以下にキャップ)
	{4_100_000, 25, 275_000},   // 130万超〜410万: 年金×25%+27.5万
	{7_700_000, 15, 685_000},   // 410万超〜770万: 年金×15%+68.5万
	{10_000_000, 5, 1_455_000}, // 770万超〜1000万: 年金×5%+145.5万
}

// PensionDeductionUnder65Max は 65歳未満・年金収入1,000万超の控除額。
const PensionDeductionUnder65Max = 1_955_000

// PensionDeductionOver65 は 65歳以上のテーブル。
var PensionDeductionOver65 = []PensionDeductionBracket{
	{3_300_000, 0, 1_100_000},  // ≤330万: 110万 (収入以下にキャップ)
	{4_100_000, 25, 275_000},   // 330万超〜410万: 年金×25%+27.5万
	{7_700_000, 15, 685_000},   // 410万超〜770万: 年金×15%+68.5万
	{10_000_000, 5, 1_455_000}, // 770万超〜1000万: 年金×5%+145.5万
}

// PensionDeductionOver65Max は 65歳以上・年金収入1,000万超の控除額。
const PensionDeductionOver65Max = 1_955_000

// 公的年金等以外の合計所得金額による控除減額。
const (
	PensionOtherIncomeBracket1    = 10_000_000 // 1,000万超で -10万
	PensionOtherIncomeBracket2    = 20_000_000 // 2,000万超で -20万
	PensionOtherIncomeAdjustment1 = 100_000
	PensionOtherIncomeAdjustment2 = 200_000
)

// ============================================================
// 退職所得控除 (所得税法第30条)
// ============================================================

const (
	RetirementDeductionPerYearUnder20  = 400_000   // 勤続20年以下: 40万×勤続年数
	RetirementDeductionMin             = 800_000   // 最低80万
	RetirementDeductionPerYearOver20   = 700_000   // 勤続20年超: 70万×超過年数
	RetirementDeductionBase20          = 8_000_000 // 20年分: 800万
	RetirementDeductionDisabilityAdd   = 1_000_000 // 障害退職: +100万
	RetirementOfficerShortServiceYears = 5         // 役員等の短期退職の基準年数
	RetirementShortServiceHalfLimit    = 3_000_000 // 一般短期退職の1/2適用上限 (令和4年改正)
)

// ============================================================
// 減価償却
// ============================================================

const (
	MonthsPerYear            = 12
	BusinessRatioDenomPct    = 100
	DecliningRateDenominator = 1000 // 定率法の償却率は千分率で保持する (例: 500 = 0.500)
)
