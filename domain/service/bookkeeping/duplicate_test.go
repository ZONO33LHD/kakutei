package bookkeeping

import (
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/model"
)

func dupEntry(t *testing.T, id int64, date string, code model.AccountCode, amount model.Money) model.JournalEntry {
	t.Helper()
	return model.JournalEntry{
		ID: id, FiscalYear: 2025, Date: mustDate(t, date),
		Lines: []model.JournalLine{
			{Side: model.SideDebit, AccountCode: code, Amount: amount},
			{Side: model.SideCredit, AccountCode: "1002", Amount: amount},
		},
	}
}

func TestCheckOnInsertExactMatch(t *testing.T) {
	svc := NewDuplicateService()
	entry := dupEntry(t, 0, "2025-04-01", "5140", 10_000)
	existing := dupEntry(t, 7, "2025-04-01", "5140", 10_000)

	warn := svc.CheckOnInsert(&entry, []model.JournalEntry{existing})
	if warn == nil || warn.Kind != MatchExact || warn.ExistingJournalID != 7 || warn.Score != 100 {
		t.Errorf("完全一致を検出すべき: %+v", warn)
	}
}

func TestCheckOnInsertSimilarMatch(t *testing.T) {
	svc := NewDuplicateService()
	entry := dupEntry(t, 0, "2025-04-01", "5140", 10_000)
	// 同日・同額だが科目が異なる → similar
	existing := dupEntry(t, 8, "2025-04-01", "5190", 10_000)

	warn := svc.CheckOnInsert(&entry, []model.JournalEntry{existing})
	if warn == nil || warn.Kind != MatchSimilar || warn.Score != 70 {
		t.Errorf("類似を検出すべき: %+v", warn)
	}
}

func TestCheckOnInsertNoMatch(t *testing.T) {
	svc := NewDuplicateService()
	entry := dupEntry(t, 0, "2025-04-01", "5140", 10_000)
	others := []model.JournalEntry{
		dupEntry(t, 9, "2025-04-02", "5140", 10_000),  // 日付違い
		dupEntry(t, 10, "2025-04-01", "5140", 20_000), // 金額違い
	}
	if warn := svc.CheckOnInsert(&entry, others); warn != nil {
		t.Errorf("重複なしのはず: %+v", warn)
	}
}

func TestFindDuplicatePairs(t *testing.T) {
	svc := NewDuplicateService()
	entries := []model.JournalEntry{
		dupEntry(t, 1, "2025-04-01", "5140", 10_000), // 1と2は完全一致
		dupEntry(t, 2, "2025-04-01", "5140", 10_000),
		dupEntry(t, 3, "2025-04-01", "5190", 10_000), // 同日同額のみ (借方科目は不一致) → 70
		dupEntry(t, 4, "2025-05-01", "5140", 10_000), // 日付違い → 対象外
	}
	result := svc.FindDuplicatePairs(entries, 70)
	if result.ExactCount != 1 {
		t.Errorf("ExactCount = %d, want 1", result.ExactCount)
	}
	// (1,3) と (2,3) が同日同額ペア
	if result.SuspectedCount != 2 {
		t.Errorf("SuspectedCount = %d, want 2", result.SuspectedCount)
	}
	// 貸方 (決済口座) の共有だけでは 90 点にならない
	for _, pair := range result.Pairs {
		if pair.Score == 90 {
			t.Errorf("借方科目が異なるペアが90点: %+v", pair)
		}
	}
	// 借方科目が一致するペアは 90 点
	sameDebit := []model.JournalEntry{
		dupEntry(t, 20, "2025-06-01", "5140", 5_000),
		{
			ID: 21, FiscalYear: 2025, Date: mustDate(t, "2025-06-01"), Counterparty: "別の取引先",
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "5140", Amount: 5_000},
				{Side: model.SideCredit, AccountCode: "1001", Amount: 5_000},
			},
		},
	}
	partial := svc.FindDuplicatePairs(sameDebit, 90)
	if len(partial.Pairs) != 1 || partial.Pairs[0].Score != 90 {
		t.Errorf("借方一致は90点: %+v", partial.Pairs)
	}
	// 閾値を上げると exact のみ
	strict := svc.FindDuplicatePairs(entries, 100)
	if len(strict.Pairs) != 1 || strict.Pairs[0].Score != 100 {
		t.Errorf("閾値100: %+v", strict.Pairs)
	}
}

func TestAggregateConsumption(t *testing.T) {
	svc := NewStatementService()
	entries := []model.JournalEntry{
		// 課税売上 110,000 (10%)
		{
			ID: 1, FiscalYear: 2025, Date: mustDate(t, "2025-02-01"),
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "1002", Amount: 110_000},
				{Side: model.SideCredit, AccountCode: "4001", Amount: 110_000, TaxCategory: model.LineTaxTaxable10},
			},
		},
		// 売上値引 11,000 (借方の収益 → 控除)
		{
			ID: 2, FiscalYear: 2025, Date: mustDate(t, "2025-02-10"),
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "4001", Amount: 11_000, TaxCategory: model.LineTaxTaxable10},
				{Side: model.SideCredit, AccountCode: "1002", Amount: 11_000},
			},
		},
		// 課税仕入 22,000 (10%) — 明細の税区分未設定 → 科目既定 (課税) から補完
		{
			ID: 3, FiscalYear: 2025, Date: mustDate(t, "2025-03-01"),
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "5140", Amount: 22_000},
				{Side: model.SideCredit, AccountCode: "1001", Amount: 22_000},
			},
		},
		// 軽減税率の売上 108,000 (8%)
		{
			ID: 4, FiscalYear: 2025, Date: mustDate(t, "2025-04-01"),
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "1001", Amount: 108_000},
				{Side: model.SideCredit, AccountCode: "4001", Amount: 108_000, TaxCategory: model.LineTaxTaxable8},
			},
		},
	}
	agg, err := svc.AggregateConsumption(2025, testAccounts(), entries)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if agg.TaxableSales10 != 99_000 { // 110,000 − 11,000
		t.Errorf("TaxableSales10 = %d, want 99000", agg.TaxableSales10.Yen())
	}
	if agg.TaxableSales8 != 108_000 {
		t.Errorf("TaxableSales8 = %d, want 108000", agg.TaxableSales8.Yen())
	}
	if agg.TaxablePurchases10 != 22_000 {
		t.Errorf("TaxablePurchases10 = %d, want 22000", agg.TaxablePurchases10.Yen())
	}
	// 税区分を科目既定から補完した明細 (通信費) が1件
	if agg.DefaultedLineCount != 1 {
		t.Errorf("DefaultedLineCount = %d, want 1", agg.DefaultedLineCount)
	}
}

// 資産の貸方 (売却・償却) は課税仕入の減額にしない。免税売上は独立集計。
func TestAggregateConsumptionAssetAndExempt(t *testing.T) {
	svc := NewStatementService()
	accounts := append(testAccounts(),
		model.Account{Code: "1130", Name: "工具器具備品", Category: model.CategoryAsset,
			TaxCategory: model.ConsumptionTaxable, IsActive: true})
	entries := []model.JournalEntry{
		// 固定資産の取得 (借方) → 課税仕入
		{
			ID: 1, FiscalYear: 2025, Date: mustDate(t, "2025-05-01"),
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "1130", Amount: 330_000, TaxCategory: model.LineTaxTaxable10},
				{Side: model.SideCredit, AccountCode: "1002", Amount: 330_000},
			},
		},
		// 固定資産の貸方 (簿価の取崩し) → 課税仕入の減額にしない
		{
			ID: 2, FiscalYear: 2025, Date: mustDate(t, "2025-06-01"),
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "1002", Amount: 110_000},
				{Side: model.SideCredit, AccountCode: "1130", Amount: 110_000, TaxCategory: model.LineTaxTaxable10},
			},
		},
		// 免税売上 (輸出)
		{
			ID: 3, FiscalYear: 2025, Date: mustDate(t, "2025-07-01"),
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "1002", Amount: 200_000},
				{Side: model.SideCredit, AccountCode: "4001", Amount: 200_000, TaxCategory: model.LineTaxExempt},
			},
		},
	}
	agg, err := svc.AggregateConsumption(2025, accounts, entries)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if agg.TaxablePurchases10 != 330_000 {
		t.Errorf("TaxablePurchases10 = %d, want 330000 (貸方は減額しない)", agg.TaxablePurchases10.Yen())
	}
	if agg.ExemptSales != 200_000 {
		t.Errorf("ExemptSales = %d, want 200000", agg.ExemptSales.Yen())
	}
}

// 返品超過で負になる集計値は 0 に丸めてフラグを立てる。
func TestAggregateConsumptionNegativeClamp(t *testing.T) {
	svc := NewStatementService()
	entries := []model.JournalEntry{
		{
			ID: 1, FiscalYear: 2025, Date: mustDate(t, "2025-02-01"),
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "4001", Amount: 50_000, TaxCategory: model.LineTaxTaxable10},
				{Side: model.SideCredit, AccountCode: "1002", Amount: 50_000},
			},
		},
	}
	agg, err := svc.AggregateConsumption(2025, testAccounts(), entries)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if agg.TaxableSales10 != 0 || !agg.NegativeClamped {
		t.Errorf("負の集計は0に丸めてフラグ: sales=%d clamped=%t",
			agg.TaxableSales10.Yen(), agg.NegativeClamped)
	}
}
