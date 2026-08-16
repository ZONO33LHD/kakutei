package bookkeeping

import (
	"sort"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
)

// GeneralLedgerLine は総勘定元帳の1行。
type GeneralLedgerLine struct {
	JournalID          int64
	Date               model.Date
	Description        string
	Counterparty       string
	CounterAccountCode model.AccountCode // 相手勘定科目コード (複合仕訳は "****")
	CounterAccountName string            // 相手勘定科目名 (複合仕訳は「諸口」)
	Debit              model.Money
	Credit             model.Money
	Balance            model.Money // 累積残高 (正常残高側が正)
}

// GeneralLedger は総勘定元帳 (1科目分)。
type GeneralLedger struct {
	FiscalYear     model.FiscalYear
	Account        model.Account
	OpeningBalance model.Money
	Lines          []GeneralLedgerLine
	ClosingBalance model.Money
}

// 複合仕訳の相手勘定表示。
const (
	compositeCounterCode = model.AccountCode("****")
	compositeCounterName = "諸口"
)

// BuildGeneralLedger は指定科目の総勘定元帳を組み立てる。
// 仕訳は日付昇順 (同日は仕訳ID順) に並べ、累積残高を計算する。
func (s *StatementService) BuildGeneralLedger(
	year model.FiscalYear, code model.AccountCode, accounts []model.Account,
	entries []model.JournalEntry, openingBalance model.Money,
) (*GeneralLedger, error) {
	idx := accountIndex(accounts)
	account, ok := idx[code]
	if !ok {
		return nil, apperrors.Newf(apperrors.CodeNotFound, "勘定科目が見つかりません: %s", code)
	}
	if err := validateInputs(year, idx, entries, nil); err != nil {
		return nil, err
	}

	sorted := make([]model.JournalEntry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Date != sorted[j].Date {
			return sorted[i].Date.Before(sorted[j].Date)
		}
		return sorted[i].ID < sorted[j].ID
	})

	debitNormal := account.Category.NormalSide() == model.SideDebit
	ledger := &GeneralLedger{
		FiscalYear:     year,
		Account:        account,
		OpeningBalance: openingBalance,
	}
	balance := openingBalance

	for i := range sorted {
		entry := &sorted[i]
		if !containsAccount(entry, code) {
			continue
		}
		for j := range entry.Lines {
			line := &entry.Lines[j]
			if line.AccountCode != code {
				continue
			}
			counterCode, counterName := counterAccountOf(entry, line, idx)
			var debit, credit model.Money
			if line.Side == model.SideDebit {
				debit = line.Amount
			} else {
				credit = line.Amount
			}
			if debitNormal {
				balance += debit - credit
			} else {
				balance += credit - debit
			}
			ledger.Lines = append(ledger.Lines, GeneralLedgerLine{
				JournalID:          entry.ID,
				Date:               entry.Date,
				Description:        entry.Description,
				Counterparty:       entry.Counterparty,
				CounterAccountCode: counterCode,
				CounterAccountName: counterName,
				Debit:              debit,
				Credit:             credit,
				Balance:            balance,
			})
		}
	}
	ledger.ClosingBalance = balance
	return ledger, nil
}

func containsAccount(entry *model.JournalEntry, code model.AccountCode) bool {
	for i := range entry.Lines {
		if entry.Lines[i].AccountCode == code {
			return true
		}
	}
	return false
}

// counterAccountOf は明細の相手勘定を判定する。
// 相手勘定は仕訳内の「反対側」の明細から求め、1科目ならその科目、
// 複数科目なら「諸口」を返す。
// (例: 借方 通信費8,000・消耗品2,000 / 貸方 現金10,000 の通信費行の相手は現金)
func counterAccountOf(
	entry *model.JournalEntry, self *model.JournalLine, idx map[model.AccountCode]model.Account,
) (model.AccountCode, string) {
	counter := map[model.AccountCode]struct{}{}
	for i := range entry.Lines {
		line := &entry.Lines[i]
		if line.Side != self.Side {
			counter[line.AccountCode] = struct{}{}
		}
	}
	switch len(counter) {
	case 0:
		// 反対側の明細がない (貸借一致の検証を通っていれば起こらないが防御)
		return compositeCounterCode, compositeCounterName
	case 1:
		for code := range counter {
			return code, idx[code].Name
		}
		panic("unreachable")
	default:
		return compositeCounterCode, compositeCounterName
	}
}
