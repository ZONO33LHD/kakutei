package usecase

import (
	"context"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
	"github.com/ZONO33LHD/kakutei/domain/service/bookkeeping"
)

// ReportUsecase は財務諸表・元帳・期首残高のアプリケーションサービス。
type ReportUsecase interface {
	TrialBalance(ctx context.Context, year model.FiscalYear) (*bookkeeping.TrialBalance, error)
	ProfitAndLoss(ctx context.Context, year model.FiscalYear) (*bookkeeping.ProfitAndLoss, error)
	BalanceSheet(ctx context.Context, year model.FiscalYear) (*bookkeeping.BalanceSheet, error)
	GeneralLedger(ctx context.Context, year model.FiscalYear, code model.AccountCode) (*bookkeeping.GeneralLedger, error)

	SetOpeningBalance(ctx context.Context, balance *model.OpeningBalance) error
	ListOpeningBalances(ctx context.Context, year model.FiscalYear) ([]model.OpeningBalance, error)
	DeleteOpeningBalance(ctx context.Context, id int64) error

	ListAccounts(ctx context.Context) ([]model.Account, error)
}

type reportUsecase struct {
	journals   repository.JournalRepository
	accounts   repository.AccountRepository
	openings   repository.OpeningBalanceRepository
	statements *bookkeeping.StatementService
}

func NewReportUsecase(
	journals repository.JournalRepository,
	accounts repository.AccountRepository,
	openings repository.OpeningBalanceRepository,
	statements *bookkeeping.StatementService,
) ReportUsecase {
	return &reportUsecase{
		journals: journals, accounts: accounts, openings: openings, statements: statements,
	}
}

// gather は財務諸表の共通材料 (科目・仕訳・期首残高) を取得する。
func (u *reportUsecase) gather(ctx context.Context, year model.FiscalYear) (
	[]model.Account, []model.JournalEntry, []model.OpeningBalance, error,
) {
	if err := year.Validate(); err != nil {
		return nil, nil, nil, err
	}
	accounts, err := u.accounts.FindAll(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	entries, err := u.journals.ListByFiscalYear(ctx, year)
	if err != nil {
		return nil, nil, nil, err
	}
	openings, err := u.openings.ListByFiscalYear(ctx, year)
	if err != nil {
		return nil, nil, nil, err
	}
	return accounts, entries, openings, nil
}

func (u *reportUsecase) TrialBalance(ctx context.Context, year model.FiscalYear) (*bookkeeping.TrialBalance, error) {
	accounts, entries, openings, err := u.gather(ctx, year)
	if err != nil {
		return nil, err
	}
	return u.statements.BuildTrialBalance(year, accounts, entries, openings)
}

func (u *reportUsecase) ProfitAndLoss(ctx context.Context, year model.FiscalYear) (*bookkeeping.ProfitAndLoss, error) {
	accounts, entries, _, err := u.gather(ctx, year)
	if err != nil {
		return nil, err
	}
	return u.statements.BuildProfitAndLoss(year, accounts, entries)
}

func (u *reportUsecase) BalanceSheet(ctx context.Context, year model.FiscalYear) (*bookkeeping.BalanceSheet, error) {
	accounts, entries, openings, err := u.gather(ctx, year)
	if err != nil {
		return nil, err
	}
	return u.statements.BuildBalanceSheet(year, accounts, entries, openings)
}

func (u *reportUsecase) GeneralLedger(
	ctx context.Context, year model.FiscalYear, code model.AccountCode,
) (*bookkeeping.GeneralLedger, error) {
	if err := code.Validate(); err != nil {
		return nil, err
	}
	accounts, entries, openings, err := u.gather(ctx, year)
	if err != nil {
		return nil, err
	}
	var opening model.Money
	for i := range openings {
		if openings[i].AccountCode == code {
			opening += openings[i].Amount
		}
	}
	return u.statements.BuildGeneralLedger(year, code, accounts, entries, opening)
}

func (u *reportUsecase) SetOpeningBalance(ctx context.Context, balance *model.OpeningBalance) error {
	if err := balance.Validate(); err != nil {
		return err
	}
	// 科目の存在検証
	if _, err := u.accounts.FindByCode(ctx, balance.AccountCode); err != nil {
		return err
	}
	return u.openings.Upsert(ctx, balance)
}

func (u *reportUsecase) ListOpeningBalances(ctx context.Context, year model.FiscalYear) ([]model.OpeningBalance, error) {
	if err := year.Validate(); err != nil {
		return nil, err
	}
	return u.openings.ListByFiscalYear(ctx, year)
}

func (u *reportUsecase) DeleteOpeningBalance(ctx context.Context, id int64) error {
	return u.openings.Delete(ctx, id)
}

func (u *reportUsecase) ListAccounts(ctx context.Context) ([]model.Account, error) {
	return u.accounts.FindAll(ctx)
}
