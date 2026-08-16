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
