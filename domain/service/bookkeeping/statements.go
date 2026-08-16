// Package bookkeeping は複式簿記の集計・検証を行うドメインサービス群。
//
// リポジトリから取得した仕訳・勘定科目・期首残高を入力として、
// 残高試算表・損益計算書・貸借対照表・総勘定元帳を純粋に計算する。
// I/O には依存しない。
package bookkeeping

import (
	"sort"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
)

// StatementService は財務諸表の組み立てを行うドメインサービス。
type StatementService struct{}

func NewStatementService() *StatementService { return &StatementService{} }

// AccountBalance は残高試算表の1行。
type AccountBalance struct {
	Account        model.Account
	OpeningBalance model.Money // 期首残高 (正常残高側が正)
	DebitTotal     model.Money // 当期借方合計
	CreditTotal    model.Money // 当期貸方合計
	ClosingBalance model.Money // 期末残高 (正常残高側が正) = 期首 + 当期増減
}

// TrialBalance は残高試算表 (繰越残高を含む)。
type TrialBalance struct {
	FiscalYear  model.FiscalYear
	Accounts    []AccountBalance
	TotalDebit  model.Money // 当期借方合計
	TotalCredit model.Money // 当期貸方合計
}

func (t *TrialBalance) Balanced() bool { return t.TotalDebit == t.TotalCredit }

func accountIndex(accounts []model.Account) map[model.AccountCode]model.Account {
	idx := make(map[model.AccountCode]model.Account, len(accounts))
	for _, a := range accounts {
		idx[a.Code] = a
	}
	return idx
}

func validateInputs(
	year model.FiscalYear, idx map[model.AccountCode]model.Account,
	entries []model.JournalEntry, openings []model.OpeningBalance,
) error {
	for i := range entries {
		if entries[i].FiscalYear != year {
			return apperrors.Newf(apperrors.CodeBadRequest,
				"仕訳 (ID: %d) の年度 %d が集計対象年度 %d と一致しません",
				entries[i].ID, int(entries[i].FiscalYear), int(year))
		}
		for j := range entries[i].Lines {
			if _, ok := idx[entries[i].Lines[j].AccountCode]; !ok {
				return apperrors.Newf(apperrors.CodeBadRequest,
					"仕訳 (ID: %d) に未知の勘定科目コードがあります: %s",
					entries[i].ID, entries[i].Lines[j].AccountCode)
			}
		}
	}
	for i := range openings {
		if openings[i].FiscalYear != year {
			return apperrors.Newf(apperrors.CodeBadRequest,
				"期首残高の年度 %d が集計対象年度 %d と一致しません",
				int(openings[i].FiscalYear), int(year))
		}
		if _, ok := idx[openings[i].AccountCode]; !ok {
			return apperrors.Newf(apperrors.CodeBadRequest,
				"期首残高に未知の勘定科目コードがあります: %s", openings[i].AccountCode)
		}
	}
	return nil
}

// BuildTrialBalance は仕訳と期首残高から残高試算表を組み立てる。
// 期首残高のみで当期異動のない科目も行に含める。
func (s *StatementService) BuildTrialBalance(
	year model.FiscalYear, accounts []model.Account,
	entries []model.JournalEntry, openings []model.OpeningBalance,
) (*TrialBalance, error) {
	idx := accountIndex(accounts)
	if err := validateInputs(year, idx, entries, openings); err != nil {
		return nil, err
	}

	type totals struct{ opening, debit, credit model.Money }
	sums := map[model.AccountCode]*totals{}
	get := func(code model.AccountCode) *totals {
		t := sums[code]
		if t == nil {
			t = &totals{}
			sums[code] = t
		}
		return t
	}

	for i := range openings {
		get(openings[i].AccountCode).opening += openings[i].Amount
	}
	for i := range entries {
		for j := range entries[i].Lines {
			line := &entries[i].Lines[j]
			t := get(line.AccountCode)
			if line.Side == model.SideDebit {
				t.debit += line.Amount
			} else {
				t.credit += line.Amount
			}
		}
	}

	codes := make([]model.AccountCode, 0, len(sums))
	for code := range sums {
		codes = append(codes, code)
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })

	tb := &TrialBalance{FiscalYear: year}
	for _, code := range codes {
		t := sums[code]
		account := idx[code]
		// 期末残高は正常残高側を正とする
		movement := t.debit - t.credit
		if account.Category.NormalSide() == model.SideCredit {
			movement = t.credit - t.debit
		}
		tb.Accounts = append(tb.Accounts, AccountBalance{
			Account:        account,
			OpeningBalance: t.opening,
			DebitTotal:     t.debit,
			CreditTotal:    t.credit,
			ClosingBalance: t.opening + movement,
		})
		tb.TotalDebit += t.debit
		tb.TotalCredit += t.credit
	}
	return tb, nil
}

// StatementLine は損益計算書・貸借対照表の1行。
type StatementLine struct {
	AccountCode model.AccountCode
	AccountName string
	Amount      model.Money
}

// ProfitAndLoss は損益計算書。
type ProfitAndLoss struct {
	FiscalYear   model.FiscalYear
	Revenues     []StatementLine // 収益 (貸方 − 借方)
	Expenses     []StatementLine // 費用 (借方 − 貸方)
	TotalRevenue model.Money
	TotalExpense model.Money
	NetIncome    model.Money // 青色申告特別控除前の所得金額
}

// BuildProfitAndLoss は仕訳から損益計算書を組み立てる。
// 損益は当期仕訳のみから計算する (期首残高は関与しない)。
func (s *StatementService) BuildProfitAndLoss(
	year model.FiscalYear, accounts []model.Account, entries []model.JournalEntry,
) (*ProfitAndLoss, error) {
	tb, err := s.BuildTrialBalance(year, accounts, entries, nil)
	if err != nil {
		return nil, err
	}
	pl := &ProfitAndLoss{FiscalYear: year}
	for _, ab := range tb.Accounts {
		switch ab.Account.Category {
		case model.CategoryRevenue:
			amount := ab.CreditTotal - ab.DebitTotal
			if amount != 0 {
				pl.Revenues = append(pl.Revenues, StatementLine{ab.Account.Code, ab.Account.Name, amount})
				pl.TotalRevenue += amount
			}
		case model.CategoryExpense:
			amount := ab.DebitTotal - ab.CreditTotal
			if amount != 0 {
				pl.Expenses = append(pl.Expenses, StatementLine{ab.Account.Code, ab.Account.Name, amount})
				pl.TotalExpense += amount
			}
		}
	}
	pl.NetIncome = pl.TotalRevenue - pl.TotalExpense
	return pl, nil
}

// netIncomeLineCode は貸借対照表で当期純利益を計上する純資産科目
// (標準マスタの「控除前所得金額」)。マスタに存在しない場合も同コードで計上する。
const (
	netIncomeLineCode = model.AccountCode("3020")
	netIncomeLineName = "控除前所得金額"
)

// BalanceSheet は貸借対照表。
// 期末残高 = 期首残高 + 当期仕訳の増減。純資産には当期純利益を明細行として含める。
type BalanceSheet struct {
	FiscalYear       model.FiscalYear
	Assets           []StatementLine
	Liabilities      []StatementLine
	Equity           []StatementLine // 当期純利益 (控除前所得金額) の行を含む
	TotalAssets      model.Money
	TotalLiabilities model.Money
	TotalEquity      model.Money
	NetIncome        model.Money

	OpeningAssets           []StatementLine
	OpeningLiabilities      []StatementLine
	OpeningEquity           []StatementLine
	OpeningTotalAssets      model.Money
	OpeningTotalLiabilities model.Money
	OpeningTotalEquity      model.Money
}

func (b *BalanceSheet) Balanced() bool {
	return b.TotalAssets == b.TotalLiabilities+b.TotalEquity
}

func (s *StatementService) BuildBalanceSheet(
	year model.FiscalYear, accounts []model.Account,
	entries []model.JournalEntry, openings []model.OpeningBalance,
) (*BalanceSheet, error) {
	tb, err := s.BuildTrialBalance(year, accounts, entries, openings)
	if err != nil {
		return nil, err
	}

	bs := &BalanceSheet{FiscalYear: year}
	var netIncome model.Money
	for _, ab := range tb.Accounts {
		switch ab.Account.Category {
		case model.CategoryRevenue:
			netIncome += ab.CreditTotal - ab.DebitTotal
		case model.CategoryExpense:
			netIncome -= ab.DebitTotal - ab.CreditTotal
		default:
			appendBSLine(bs, &ab)
		}
	}
	bs.NetIncome = netIncome

	// 当期純利益を純資産の明細行として計上する (明細合計 = TotalEquity を保つ)
	if netIncome != 0 {
		name := netIncomeLineName
		if account, ok := accountIndex(accounts)[netIncomeLineCode]; ok {
			name = account.Name
		}
		bs.Equity = append(bs.Equity, StatementLine{netIncomeLineCode, name, netIncome})
		bs.TotalEquity += netIncome
	}
	return bs, nil
}

func appendBSLine(bs *BalanceSheet, ab *AccountBalance) {
	opening := ab.OpeningBalance
	closing := ab.ClosingBalance
	openLine := StatementLine{ab.Account.Code, ab.Account.Name, opening}
	closeLine := StatementLine{ab.Account.Code, ab.Account.Name, closing}
	switch ab.Account.Category {
	case model.CategoryAsset:
		if opening != 0 {
			bs.OpeningAssets = append(bs.OpeningAssets, openLine)
			bs.OpeningTotalAssets += opening
		}
		if closing != 0 {
			bs.Assets = append(bs.Assets, closeLine)
			bs.TotalAssets += closing
		}
	case model.CategoryLiability:
		if opening != 0 {
			bs.OpeningLiabilities = append(bs.OpeningLiabilities, openLine)
			bs.OpeningTotalLiabilities += opening
		}
		if closing != 0 {
			bs.Liabilities = append(bs.Liabilities, closeLine)
			bs.TotalLiabilities += closing
		}
	case model.CategoryEquity:
		if opening != 0 {
			bs.OpeningEquity = append(bs.OpeningEquity, openLine)
			bs.OpeningTotalEquity += opening
		}
		if closing != 0 {
			bs.Equity = append(bs.Equity, closeLine)
			bs.TotalEquity += closing
		}
	}
}
