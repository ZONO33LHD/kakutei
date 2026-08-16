package taxation

import (
	"math/big"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/policy"
)

// mulDivBig は a×b÷c (切り捨て) を big.Int で計算する。
// a×b が int64 を超え得る箇所 (按分計算等) で使う。c > 0 が前提。
func mulDivBig(a, b, c model.Money) model.Money {
	if c <= 0 {
		panic("taxation.mulDivBig: 分母が 0 以下です")
	}
	var r big.Int
	r.Mul(big.NewInt(a.Yen()), big.NewInt(b.Yen()))
	r.Quo(&r, big.NewInt(c.Yen()))
	return model.Money(r.Int64())
}

// housingLoanBalanceLimit は住宅ローン控除の年末残高上限を返す。
//
// 入居年・住宅性能区分・新築/中古・世帯区分でテーブルを選択する。
// テーブルに定義のない組み合わせはエラー (暗黙のフォールバックはしない)。
func housingLoanBalanceLimit(d *model.HousingLoanDetail) (model.Money, error) {
	// 法定済みテーブルの範囲 (令和12年末入居まで) を超える入居は誤適用を防いで拒否する
	if d.MoveInDate.Year() > policy.HousingLoanTableLastYear {
		return 0, apperrors.Newf(apperrors.CodeBadRequest,
			"入居年 %d の住宅ローン控除限度額は未対応です (対応: %d年末入居まで)",
			d.MoveInDate.Year(), policy.HousingLoanTableLastYear)
	}
	moveInYear := d.MoveInDate.Year()
	// 増改築等は性能区分・入居年によらず一律の上限 (控除期間10年)
	if d.Kind == model.HousingRenovation {
		return policy.HousingLoanRenovationLimit, nil
	}
	// 表の選択区分 (新築・買取再販 / 既存) は入力フラグでなく取得区分から導出する
	newOrResale := d.Kind.UsesNewConstructionTable(d.Category)

	key := policy.HousingCategoryKey{Category: d.Category, NewConstruction: newOrResale}
	var limits map[policy.HousingCategoryKey]int64
	switch {
	case moveInYear <= policy.HousingLoanR4R5LastYear:
		limits = policy.HousingLoanLimitsR4R5
	case moveInYear <= policy.HousingLoanR6R7LastYear && d.IsChildcareHousehold:
		limits = policy.HousingLoanLimitsR6R7Childcare
	case moveInYear <= policy.HousingLoanR6R7LastYear:
		limits = policy.HousingLoanLimitsR6R7
	case d.IsChildcareHousehold:
		limits = policy.HousingLoanLimitsR8Childcare
	default:
		limits = policy.HousingLoanLimitsR8
	}
	limit, ok := limits[key]
	if !ok {
		return 0, apperrors.Newf(apperrors.CodeBadRequest,
			"住宅ローン控除の限度額が未定義の組み合わせです: %s/%s", d.Category, d.Kind)
	}
	// 一般住宅の新築等の R5 以前建築確認済み特例 (令和6〜7年入居のみの措置)
	if limit == 0 && moveInYear <= policy.HousingLoanR6R7LastYear &&
		d.Category == policy.HousingGeneral && newOrResale && d.HasPreR6Permit {
		limit = policy.HousingLoanGeneralR5Confirmed
	}
	return model.Money(limit), nil
}

// housingLoanCreditFromBalance は残高×0.7% (100円未満切捨) の控除額を返す。
func housingLoanCreditFromBalance(balance model.Money) model.Money {
	credit := balance.MulDiv(policy.HousingLoanRateNum, policy.HousingLoanRateDenom)
	return credit.RoundDownTo(policy.TaxAmountRoundingUnit)
}

// housingLoanCreditBase は控除対象となる残高を返す。
// min(年末残高, 住宅取得対価等 (入力時のみ), 借入限度額)。
func housingLoanCreditBase(d *model.HousingLoanDetail, balance model.Money) (model.Money, error) {
	limit, err := housingLoanBalanceLimit(d)
	if err != nil {
		return 0, err
	}
	base := balance
	if d.AcquisitionCost > 0 {
		base = base.Min(d.AcquisitionCost)
	}
	return base.Min(limit), nil
}

// HousingLoanCredit は住宅ローン控除 (単独明細) を計算する。
// 控除率 0.7% (令和4年以降入居)。控除対象額は年末残高・住宅取得対価等・
// 区分別の借入限度額の最小値。
func HousingLoanCredit(d *model.HousingLoanDetail) (model.Money, error) {
	if d.YearEndBalance <= 0 {
		return 0, nil
	}
	base, err := housingLoanCreditBase(d, d.YearEndBalance)
	if err != nil {
		return 0, err
	}
	return housingLoanCreditFromBalance(base), nil
}

// HousingLoanCreditEntry は重複適用の個別明細の計算結果。
type HousingLoanCreditEntry struct {
	Kind             model.HousingKind
	ProratedBalance  model.Money // 按分後の年末残高
	BalanceLimit     model.Money // 適用される借入限度額
	CappedBalance    model.Money // min(按分後残高, 取得対価, 限度額)
	Credit           model.Money // 控除額 (100円未満切捨)
	ProrationRatioPM int64       // 按分比率 (万分率: 6667 = 66.67%)
}

// HousingLoanCreditDual は重複適用 (中古住宅購入＋リフォーム同時) の按分計算を行う。
//
// 同一 DualApplicationGroup の複数明細を受け取り、CostForProration の比率で
// 共有の年末残高を按分して各明細の控除額を計算する。最後の明細で端数調整し、
// 按分合計が元の年末残高と一致することを保証する。
// 合計控除額は「各取得等の控除限度額 (借入限度額×0.7%) のうち最大のもの」でキャップする。
func HousingLoanCreditDual(details []model.HousingLoanDetail) (model.Money, []HousingLoanCreditEntry, error) {
	if len(details) < 2 {
		return 0, nil, apperrors.New(apperrors.CodeBadRequest, "重複適用には2件以上の明細が必要です")
	}
	sharedBalance := details[0].YearEndBalance
	var totalCost model.Money
	for i := range details {
		if details[i].CostForProration <= 0 {
			return 0, nil, apperrors.Newf(apperrors.CodeBadRequest,
				"重複適用の按分用コストは正の整数が必要です (kind=%s)", details[i].Kind)
		}
		// 同一ローンの年末残高は全明細で共有されるため一致が必須
		if details[i].YearEndBalance != sharedBalance {
			return 0, nil, apperrors.New(apperrors.CodeBadRequest,
				"重複適用グループ内の年末残高が一致していません")
		}
		totalCost += details[i].CostForProration
	}

	entries := make([]HousingLoanCreditEntry, 0, len(details))
	var totalCredit, allocated model.Money
	var maxAnnualLimit model.Money

	for i := range details {
		d := &details[i]
		var prorated model.Money
		if i < len(details)-1 {
			prorated = mulDivBig(sharedBalance, d.CostForProration, totalCost)
			allocated += prorated
		} else {
			// 端数調整: 合計が元の年末残高と一致するようにする
			prorated = sharedBalance - allocated
		}

		limit, err := housingLoanBalanceLimit(d)
		if err != nil {
			return 0, nil, err
		}
		capped, err := housingLoanCreditBase(d, prorated)
		if err != nil {
			return 0, nil, err
		}
		credit := housingLoanCreditFromBalance(capped)
		maxAnnualLimit = maxAnnualLimit.Max(housingLoanCreditFromBalance(limit))

		entries = append(entries, HousingLoanCreditEntry{
			Kind:             d.Kind,
			ProratedBalance:  prorated,
			BalanceLimit:     limit,
			CappedBalance:    capped,
			Credit:           credit,
			ProrationRatioPM: mulDivBig(d.CostForProration, 10_000, totalCost).Yen(),
		})
		totalCredit += credit
	}

	return totalCredit.Min(maxAnnualLimit), entries, nil
}

// TotalHousingLoanCredit は明細リスト全体の住宅ローン控除額を計算する。
//
// 適用要件: 合計所得金額 2,000万円以下 (令和4年以降入居、租税特別措置法第41条)。
// 超える場合は明細があっても控除 0。
// DualApplicationGroup が同じ2件以上の明細は按分計算し、それ以外は単独計算する。
func TotalHousingLoanCredit(details []model.HousingLoanDetail, aggregateIncome model.Money) (model.Money, error) {
	if len(details) == 0 {
		return 0, nil
	}
	if aggregateIncome > policy.HousingLoanIncomeLimit {
		return 0, nil
	}
	grouped := map[string][]model.HousingLoanDetail{}
	var singles []model.HousingLoanDetail
	for i := range details {
		if g := details[i].DualApplicationGroup; g != "" {
			grouped[g] = append(grouped[g], details[i])
		} else {
			singles = append(singles, details[i])
		}
	}

	var total model.Money
	for _, group := range grouped {
		if len(group) >= 2 {
			credit, _, err := HousingLoanCreditDual(group)
			if err != nil {
				return 0, err
			}
			total += credit
			continue
		}
		singles = append(singles, group...)
	}
	for i := range singles {
		credit, err := HousingLoanCredit(&singles[i])
		if err != nil {
			return 0, err
		}
		total += credit
	}
	return total, nil
}

// DividendCredit は配当控除 (税額控除) を計算する (所得税法第92条)。
//
// 課税総所得金額が 1,000 万円以下の部分に対応する配当は 10%、
// 超過部分に対応する配当は 5%。
// 証券投資信託の収益分配に係る軽減率 (5%/2.5% 等) は未対応 (通常の剰余金配当のみ)。
func DividendCredit(dividendIncome, taxableIncome model.Money) model.Money {
	if dividendIncome <= 0 {
		return 0
	}
	if taxableIncome <= policy.DividendCreditThreshold {
		return dividendIncome.MulDiv(policy.DividendCreditRateLowPct, 100)
	}
	// 1,000万超の場合: 1,000万以下に対応する部分=10%, 超過部分=5%
	under := (model.Money(policy.DividendCreditThreshold) - (taxableIncome - dividendIncome)).ClampNonNegative()
	if under >= dividendIncome {
		return dividendIncome.MulDiv(policy.DividendCreditRateLowPct, 100)
	}
	over := dividendIncome - under
	return under.MulDiv(policy.DividendCreditRateLowPct, 100) + over.MulDiv(policy.DividendCreditRateHighPct, 100)
}
