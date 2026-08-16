package repository

import (
	"context"

	"github.com/ZONO33LHD/kakutei/domain/model"
)

// AccountRepository は勘定科目マスタの永続化契約。
type AccountRepository interface {
	// FindAll は全科目をコード順に返す。
	FindAll(ctx context.Context) ([]model.Account, error)

	// FindByCode は科目を1件取得する。存在しなければ CodeNotFound。
	FindByCode(ctx context.Context, code model.AccountCode) (*model.Account, error)

	// SaveAll は科目マスタを一括登録する (存在するコードは更新)。初期投入用。
	SaveAll(ctx context.Context, accounts []model.Account) error
}

// FiscalYearRepository は年度管理の永続化契約。
type FiscalYearRepository interface {
	// Create は年度を open 状態で作成する。既存ならば CodeConflict。
	Create(ctx context.Context, year model.FiscalYear) error

	// Find は年度を取得する。存在しなければ CodeNotFound。
	Find(ctx context.Context, year model.FiscalYear) (*model.FiscalYearStatus, error)

	// List は全年度を昇順で返す。
	List(ctx context.Context) ([]model.FiscalYearStatus, error)

	// UpdateState は年度の開閉状態を変更する。
	UpdateState(ctx context.Context, year model.FiscalYear, state model.FiscalYearState) error
}

// OpeningBalanceRepository は期首残高の永続化契約。
type OpeningBalanceRepository interface {
	// Upsert は (年度, 科目) をキーに期首残高を登録・更新する。
	Upsert(ctx context.Context, balance *model.OpeningBalance) error

	// ListByFiscalYear は年度の期首残高を科目コード順に返す。
	ListByFiscalYear(ctx context.Context, year model.FiscalYear) ([]model.OpeningBalance, error)

	// Delete は期首残高を1件削除する。存在しなければ CodeNotFound。
	Delete(ctx context.Context, id int64) error
}
