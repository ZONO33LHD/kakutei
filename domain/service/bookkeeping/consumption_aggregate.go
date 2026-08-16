package bookkeeping

import (
	"github.com/ZONO33LHD/kakutei/domain/model"
)

// ConsumptionAggregate は帳簿から集計した消費税計算の材料 (全て税込金額)。
type ConsumptionAggregate struct {
	FiscalYear         model.FiscalYear
	TaxableSales10     model.Money // 課税売上高 (標準税率10%)
	TaxableSales8      model.Money // 課税売上高 (軽減税率8%)
	TaxablePurchases10 model.Money // 課税仕入高 (標準税率10%)
	TaxablePurchases8  model.Money // 課税仕入高 (軽減税率8%)
	NonTaxableSales    model.Money // 非課税売上高 (課税売上割合の分母用)
	ExemptSales        model.Money // 免税売上高 (輸出等。課税売上割合の分子・分母両方)

	// DefaultedLineCount は明細に税区分がなく科目既定から補完した明細数。
	// 補完は標準税率10%とみなすため、軽減税率の可能性がある場合は
	// 利用者に明細の税区分指定を促す警告に使う。
	DefaultedLineCount int

	// NegativeClamped は値引・返品の相殺で集計値が負になり 0 に丸めた項目があるか。
	// (前年売上の返品等。true の場合は手動調整が必要)
	NegativeClamped bool
}

// AggregateConsumption は仕訳から消費税の課税売上・課税仕入を集計する。
//
// 集計ルール:
//   - 課税売上: 収益科目の明細のうち税区分 taxable_10/8 のもの (貸方 − 借方。値引・返品は控除)
//   - 非課税売上・免税売上: 収益科目の non_taxable / exempt の明細 (課税売上割合用)
//   - 課税仕入: 費用科目の taxable_10/8 の明細 (借方 − 貸方)、および
//     資産科目の taxable_10/8 の「借方」明細 (固定資産の取得)。
//     資産の貸方 (売却・除却・減価償却の直接控除) は課税仕入の減額ではないため集計しない。
//
// 明細に税区分が未設定の場合は科目マスタの既定区分から補完する
// (既定の「課税」は標準税率10%とみなす。補完件数は DefaultedLineCount に記録)。
//
// 制限: 資産譲渡による課税売上・非課税売上 (土地等) は集計対象外のため、
// 該当がある場合は消費税計算の入力を手動調整すること。
func (s *StatementService) AggregateConsumption(
	year model.FiscalYear, accounts []model.Account, entries []model.JournalEntry,
) (*ConsumptionAggregate, error) {
	idx := accountIndex(accounts)
	if err := validateInputs(year, idx, entries, nil); err != nil {
		return nil, err
	}
	agg := &ConsumptionAggregate{FiscalYear: year}

	for i := range entries {
		for j := range entries[i].Lines {
			line := &entries[i].Lines[j]
			account := idx[line.AccountCode]
			category, defaulted := effectiveTaxCategory(line, &account)
			if defaulted && affectsAggregate(&account, category) {
				agg.DefaultedLineCount++
			}
			addToAggregate(agg, &account, line, category)
		}
	}

	clampAggregate(agg)
	return agg, nil
}

func affectsAggregate(account *model.Account, category model.LineTaxCategory) bool {
	switch account.Category {
	case model.CategoryRevenue:
		return category == model.LineTaxTaxable10 || category == model.LineTaxTaxable8 ||
			category == model.LineTaxNone || category == model.LineTaxExempt
	case model.CategoryExpense, model.CategoryAsset:
		return category == model.LineTaxTaxable10 || category == model.LineTaxTaxable8
	default:
		return false
	}
}

func addToAggregate(
	agg *ConsumptionAggregate, account *model.Account,
	line *model.JournalLine, category model.LineTaxCategory,
) {
	switch account.Category {
	case model.CategoryRevenue:
		// 収益: 貸方が正 (借方は値引・返品の控除)
		signed := line.Amount
		if line.Side == model.SideDebit {
			signed = -line.Amount
		}
		switch category {
		case model.LineTaxTaxable10:
			agg.TaxableSales10 += signed
		case model.LineTaxTaxable8:
			agg.TaxableSales8 += signed
		case model.LineTaxNone:
			agg.NonTaxableSales += signed
		case model.LineTaxExempt:
			agg.ExemptSales += signed
		}
	case model.CategoryExpense:
		// 費用: 借方が正 (貸方は値引・返品の控除)
		signed := line.Amount
		if line.Side == model.SideCredit {
			signed = -line.Amount
		}
		switch category {
		case model.LineTaxTaxable10:
			agg.TaxablePurchases10 += signed
		case model.LineTaxTaxable8:
			agg.TaxablePurchases8 += signed
		}
	case model.CategoryAsset:
		// 資産: 借方 (取得) のみ課税仕入。貸方 (売却・償却等) は対象外
		if line.Side != model.SideDebit {
			return
		}
		switch category {
		case model.LineTaxTaxable10:
			agg.TaxablePurchases10 += line.Amount
		case model.LineTaxTaxable8:
			agg.TaxablePurchases8 += line.Amount
		}
	}
}

func clampAggregate(agg *ConsumptionAggregate) {
	for _, p := range []*model.Money{
		&agg.TaxableSales10, &agg.TaxableSales8,
		&agg.TaxablePurchases10, &agg.TaxablePurchases8,
		&agg.NonTaxableSales, &agg.ExemptSales,
	} {
		if *p < 0 {
			*p = 0
			agg.NegativeClamped = true
		}
	}
}

// effectiveTaxCategory は明細の実効税区分と、既定から補完したかどうかを返す。
func effectiveTaxCategory(line *model.JournalLine, account *model.Account) (model.LineTaxCategory, bool) {
	if line.TaxCategory != "" {
		return line.TaxCategory, false
	}
	switch account.TaxCategory {
	case model.ConsumptionTaxable:
		// 科目既定の「課税」は標準税率とみなす (軽減税率は明細で明示する)
		return model.LineTaxTaxable10, true
	case model.ConsumptionNonTaxable:
		return model.LineTaxNone, true
	case model.ConsumptionExempt:
		return model.LineTaxExempt, true
	default:
		return model.LineTaxOutOfScope, false
	}
}
