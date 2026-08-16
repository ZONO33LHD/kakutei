package bookkeeping

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

// テスト用の最小科目マスタ。
func testAccounts() []model.Account {
	return []model.Account{
		{Code: "1001", Name: "現金", Category: model.CategoryAsset, IsActive: true},
		{Code: "1002", Name: "普通預金", Category: model.CategoryAsset, IsActive: true},
		{Code: "2020", Name: "短期借入金", Category: model.CategoryLiability, IsActive: true},
		{Code: "3001", Name: "元入金", Category: model.CategoryEquity, IsActive: true},
		{Code: "4001", Name: "売上", Category: model.CategoryRevenue, TaxCategory: model.ConsumptionTaxable, IsActive: true},
		{Code: "5140", Name: "通信費", Category: model.CategoryExpense, TaxCategory: model.ConsumptionTaxable, IsActive: true},
	}
}

// 売上 110,000 (預金)、通信費 11,000 (現金)、借入 50,000 (預金) の3仕訳。
func testEntries(t *testing.T) []model.JournalEntry {
	t.Helper()
	return []model.JournalEntry{
		{
			ID: 1, FiscalYear: 2025, Date: mustDate(t, "2025-02-01"), Description: "売上入金",
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "1002", Amount: 110_000},
				{Side: model.SideCredit, AccountCode: "4001", Amount: 110_000, TaxCategory: model.LineTaxTaxable10, TaxAmount: 10_000},
			},
		},
		{
			ID: 2, FiscalYear: 2025, Date: mustDate(t, "2025-03-01"), Description: "通信費支払",
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "5140", Amount: 11_000, TaxCategory: model.LineTaxTaxable10, TaxAmount: 1_000},
				{Side: model.SideCredit, AccountCode: "1001", Amount: 11_000},
			},
		},
		{
			ID: 3, FiscalYear: 2025, Date: mustDate(t, "2025-01-15"), Description: "借入",
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "1002", Amount: 50_000},
				{Side: model.SideCredit, AccountCode: "2020", Amount: 50_000},
			},
		},
	}
}

func TestBuildTrialBalance(t *testing.T) {
	svc := NewStatementService()
	// 期首残高のみで当期異動のない科目 (定期預金) も行に含まれる
	openings := []model.OpeningBalance{
		{FiscalYear: 2025, AccountCode: "1001", Amount: 30_000},
		{FiscalYear: 2025, AccountCode: "3001", Amount: 30_000},
	}
	tb, err := svc.BuildTrialBalance(2025, testAccounts(), testEntries(t), openings)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !tb.Balanced() {
		t.Errorf("貸借不一致: debit=%d credit=%d", tb.TotalDebit.Yen(), tb.TotalCredit.Yen())
	}
	if tb.TotalDebit != 171_000 {
		t.Errorf("TotalDebit = %d, want 171000", tb.TotalDebit.Yen())
	}
	// 科目コード順に並ぶ
	var prev model.AccountCode
	for _, ab := range tb.Accounts {
		if ab.Account.Code <= prev {
			t.Errorf("科目コード順でない: %s の後に %s", prev, ab.Account.Code)
		}
		prev = ab.Account.Code
	}
	byCode := map[model.AccountCode]AccountBalance{}
	for _, ab := range tb.Accounts {
		byCode[ab.Account.Code] = ab
	}
	// 普通預金: 借方 160,000 (期首なし)
	if ab := byCode["1002"]; ab.DebitTotal != 160_000 || ab.ClosingBalance != 160_000 {
		t.Errorf("普通預金 debit=%d closing=%d", ab.DebitTotal.Yen(), ab.ClosingBalance.Yen())
	}
	// 現金: 期首 30,000 − 貸方 11,000 = 期末 19,000
	if ab := byCode["1001"]; ab.OpeningBalance != 30_000 || ab.ClosingBalance != 19_000 {
		t.Errorf("現金 opening=%d closing=%d", ab.OpeningBalance.Yen(), ab.ClosingBalance.Yen())
	}
	// 貸方科目は正常残高側 (貸方) を正とする
	if ab := byCode["2020"]; ab.ClosingBalance != 50_000 {
		t.Errorf("短期借入金 closing=%d, want 50000 (貸方正)", ab.ClosingBalance.Yen())
	}
	// 期首のみの科目 (元入金) も含まれる
	if ab := byCode["3001"]; ab.OpeningBalance != 30_000 || ab.ClosingBalance != 30_000 {
		t.Errorf("元入金 opening=%d closing=%d", ab.OpeningBalance.Yen(), ab.ClosingBalance.Yen())
	}
}

func TestBuildTrialBalanceUnknownAccount(t *testing.T) {
	svc := NewStatementService()
	entries := testEntries(t)
	entries[0].Lines[0].AccountCode = "9999"
	if _, err := svc.BuildTrialBalance(2025, testAccounts(), entries, nil); err == nil {
		t.Error("未知の科目コードはエラーになるべき")
	}
}

func TestBuildTrialBalanceYearMismatch(t *testing.T) {
	svc := NewStatementService()
	entries := testEntries(t)
	entries[0].FiscalYear = 2024
	if _, err := svc.BuildTrialBalance(2025, testAccounts(), entries, nil); err == nil {
		t.Error("年度不一致の仕訳はエラーになるべき")
	}
}

func TestBuildProfitAndLoss(t *testing.T) {
	svc := NewStatementService()
	pl, err := svc.BuildProfitAndLoss(2025, testAccounts(), testEntries(t))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if pl.TotalRevenue != 110_000 {
		t.Errorf("TotalRevenue = %d, want 110000", pl.TotalRevenue.Yen())
	}
	if pl.TotalExpense != 11_000 {
		t.Errorf("TotalExpense = %d, want 11000", pl.TotalExpense.Yen())
	}
	if pl.NetIncome != 99_000 {
		t.Errorf("NetIncome = %d, want 99000", pl.NetIncome.Yen())
	}
}

func TestBuildBalanceSheet(t *testing.T) {
	svc := NewStatementService()
	openings := []model.OpeningBalance{
		{FiscalYear: 2025, AccountCode: "1001", Amount: 30_000},
		{FiscalYear: 2025, AccountCode: "3001", Amount: 30_000},
	}
	bs, err := svc.BuildBalanceSheet(2025, testAccounts(), testEntries(t), openings)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// 資産: 現金 30,000−11,000=19,000 + 預金 160,000 = 179,000
	if bs.TotalAssets != 179_000 {
		t.Errorf("TotalAssets = %d, want 179000", bs.TotalAssets.Yen())
	}
	// 負債: 借入 50,000
	if bs.TotalLiabilities != 50_000 {
		t.Errorf("TotalLiabilities = %d, want 50000", bs.TotalLiabilities.Yen())
	}
	// 純資産: 元入金 30,000 + 当期純利益 99,000 = 129,000
	if bs.NetIncome != 99_000 {
		t.Errorf("NetIncome = %d, want 99000", bs.NetIncome.Yen())
	}
	if bs.TotalEquity != 129_000 {
		t.Errorf("TotalEquity = %d, want 129000", bs.TotalEquity.Yen())
	}
	// 当期純利益が純資産の明細行として計上され、明細合計 = TotalEquity
	var equitySum model.Money
	netIncomeLineFound := false
	for _, line := range bs.Equity {
		equitySum += line.Amount
		if line.AccountCode == "3020" && line.Amount == 99_000 {
			netIncomeLineFound = true
		}
	}
	if !netIncomeLineFound {
		t.Error("当期純利益 (控除前所得金額) の明細行があるべき")
	}
	if equitySum != bs.TotalEquity {
		t.Errorf("純資産の明細合計 %d ≠ TotalEquity %d", equitySum.Yen(), bs.TotalEquity.Yen())
	}
	if !bs.Balanced() {
		t.Errorf("貸借対照表が不一致: 資産%d ≠ 負債%d+純資産%d",
			bs.TotalAssets.Yen(), bs.TotalLiabilities.Yen(), bs.TotalEquity.Yen())
	}
	// 期首
	if bs.OpeningTotalAssets != 30_000 || bs.OpeningTotalEquity != 30_000 {
		t.Errorf("期首: assets=%d equity=%d", bs.OpeningTotalAssets.Yen(), bs.OpeningTotalEquity.Yen())
	}
}

func TestBuildGeneralLedger(t *testing.T) {
	svc := NewStatementService()
	ledger, err := svc.BuildGeneralLedger(2025, "1002", testAccounts(), testEntries(t), 20_000)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(ledger.Lines) != 2 {
		t.Fatalf("行数 = %d, want 2", len(ledger.Lines))
	}
	// 日付順: 借入 (1/15) → 売上 (2/1)
	if ledger.Lines[0].JournalID != 3 || ledger.Lines[1].JournalID != 1 {
		t.Errorf("日付順でない: %d, %d", ledger.Lines[0].JournalID, ledger.Lines[1].JournalID)
	}
	// 累積残高: 20,000 + 50,000 = 70,000 → +110,000 = 180,000
	if ledger.Lines[0].Balance != 70_000 || ledger.Lines[1].Balance != 180_000 {
		t.Errorf("残高 = %d, %d", ledger.Lines[0].Balance.Yen(), ledger.Lines[1].Balance.Yen())
	}
	if ledger.ClosingBalance != 180_000 {
		t.Errorf("ClosingBalance = %d, want 180000", ledger.ClosingBalance.Yen())
	}
	// 相手勘定
	if ledger.Lines[0].CounterAccountCode != "2020" || ledger.Lines[0].CounterAccountName != "短期借入金" {
		t.Errorf("相手勘定 = %s %s", ledger.Lines[0].CounterAccountCode, ledger.Lines[0].CounterAccountName)
	}
}

func TestBuildGeneralLedgerCompositeCounter(t *testing.T) {
	svc := NewStatementService()
	entries := []model.JournalEntry{
		{
			ID: 10, FiscalYear: 2025, Date: mustDate(t, "2025-05-01"),
			Lines: []model.JournalLine{
				{Side: model.SideDebit, AccountCode: "5140", Amount: 8_000},
				{Side: model.SideDebit, AccountCode: "1001", Amount: 2_000},
				{Side: model.SideCredit, AccountCode: "1002", Amount: 10_000},
			},
		},
	}
	ledger, err := svc.BuildGeneralLedger(2025, "1002", testAccounts(), entries, 0)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if ledger.Lines[0].CounterAccountName != "諸口" {
		t.Errorf("複合仕訳の相手勘定 = %q, want 諸口", ledger.Lines[0].CounterAccountName)
	}
	// 貸方 10,000 → 資産の正常残高 (借方) 基準で -10,000
	if ledger.ClosingBalance != -10_000 {
		t.Errorf("ClosingBalance = %d, want -10000", ledger.ClosingBalance.Yen())
	}
}

func TestBuildGeneralLedgerUnknownAccount(t *testing.T) {
	svc := NewStatementService()
	if _, err := svc.BuildGeneralLedger(2025, "9999", testAccounts(), nil, 0); err == nil {
		t.Error("未知の科目はエラーになるべき")
	}
}
