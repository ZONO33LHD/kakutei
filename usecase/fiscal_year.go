// Package usecase は確定申告バックエンドのアプリケーションサービス層。
//
// リポジトリからの材料収集・トランザクションの調整・ドメインサービスの呼び出しを
// 担い、計算・検証のロジック自体は domain/service と domain/model に委譲する。
package usecase

import (
	"context"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
)

type FiscalYearUsecase interface {
	// Setup は年度を作成し、勘定科目マスタが未投入なら標準マスタを投入する。
	Setup(ctx context.Context, year model.FiscalYear) error

	List(ctx context.Context) ([]model.FiscalYearStatus, error)

	// Close は年度を締める (以後、仕訳・申告資料の変更を拒否する)。
	Close(ctx context.Context, year model.FiscalYear) error

	Reopen(ctx context.Context, year model.FiscalYear) error
}

type fiscalYearUsecase struct {
	years    repository.FiscalYearRepository
	accounts repository.AccountRepository
}

func NewFiscalYearUsecase(
	years repository.FiscalYearRepository,
	accounts repository.AccountRepository,
) FiscalYearUsecase {
	return &fiscalYearUsecase{years: years, accounts: accounts}
}

func (u *fiscalYearUsecase) Setup(ctx context.Context, year model.FiscalYear) error {
	if err := year.Validate(); err != nil {
		return err
	}
	// 勘定科目マスタの初期投入 (冪等)
	existing, err := u.accounts.FindAll(ctx)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		if err := u.accounts.SaveAll(ctx, model.DefaultAccounts()); err != nil {
			return err
		}
	}
	return u.years.Create(ctx, year)
}

func (u *fiscalYearUsecase) List(ctx context.Context) ([]model.FiscalYearStatus, error) {
	return u.years.List(ctx)
}

func (u *fiscalYearUsecase) Close(ctx context.Context, year model.FiscalYear) error {
	if err := year.Validate(); err != nil {
		return err
	}
	return u.years.UpdateState(ctx, year, model.FiscalYearClosed)
}

func (u *fiscalYearUsecase) Reopen(ctx context.Context, year model.FiscalYear) error {
	if err := year.Validate(); err != nil {
		return err
	}
	return u.years.UpdateState(ctx, year, model.FiscalYearOpen)
}
