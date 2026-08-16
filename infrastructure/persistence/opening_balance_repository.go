package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
)

// OpeningBalanceRepository は repository.OpeningBalanceRepository の SQLite 実装。
type OpeningBalanceRepository struct {
	db *sql.DB
}

var _ repository.OpeningBalanceRepository = (*OpeningBalanceRepository)(nil)

// NewOpeningBalanceRepository は OpeningBalanceRepository を生成する。
func NewOpeningBalanceRepository(db *sql.DB) *OpeningBalanceRepository {
	return &OpeningBalanceRepository{db: db}
}

// Upsert は (年度, 科目) をキーに期首残高を登録・更新する。
func (r *OpeningBalanceRepository) Upsert(ctx context.Context, balance *model.OpeningBalance) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := ensureYearOpen(ctx, tx, balance.FiscalYear); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO opening_balances (fiscal_year, account_code, amount) VALUES (?, ?, ?)
			ON CONFLICT(fiscal_year, account_code) DO UPDATE SET amount = excluded.amount`,
			int(balance.FiscalYear), balance.AccountCode, balance.Amount.Yen())
		if err != nil {
			return wrapInternal(err, "期首残高の保存")
		}
		return nil
	})
}

// FindByID は期首残高を1件取得する。
func (r *OpeningBalanceRepository) FindByID(ctx context.Context, id int64) (*model.OpeningBalance, error) {
	var b model.OpeningBalance
	var amount int64
	err := r.db.QueryRowContext(ctx,
		"SELECT id, fiscal_year, account_code, amount FROM opening_balances WHERE id = ?", id).
		Scan(&b.ID, &b.FiscalYear, &b.AccountCode, &amount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notFound(fmt.Sprintf("期首残高 (ID: %d)", id))
	}
	if err != nil {
		return nil, wrapInternal(err, "期首残高の取得")
	}
	b.Amount = model.Money(amount)
	return &b, nil
}

// ListByFiscalYear は年度の期首残高を科目コード順に返す。
func (r *OpeningBalanceRepository) ListByFiscalYear(ctx context.Context, year model.FiscalYear) ([]model.OpeningBalance, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, fiscal_year, account_code, amount FROM opening_balances
		WHERE fiscal_year = ? ORDER BY account_code`, int(year))
	if err != nil {
		return nil, wrapInternal(err, "期首残高の取得")
	}
	defer func() { _ = rows.Close() }()

	var balances []model.OpeningBalance
	for rows.Next() {
		var b model.OpeningBalance
		var amount int64
		if err := rows.Scan(&b.ID, &b.FiscalYear, &b.AccountCode, &amount); err != nil {
			return nil, wrapInternal(err, "期首残高の読み取り")
		}
		b.Amount = model.Money(amount)
		balances = append(balances, b)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapInternal(err, "期首残高の走査")
	}
	return balances, nil
}

// Delete は期首残高を1件削除する。締め済み年度のレコードは削除できない。
func (r *OpeningBalanceRepository) Delete(ctx context.Context, id int64) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		var year int
		err := tx.QueryRowContext(ctx,
			"SELECT fiscal_year FROM opening_balances WHERE id = ?", id).Scan(&year)
		if errors.Is(err, sql.ErrNoRows) {
			return notFound(fmt.Sprintf("期首残高 (ID: %d)", id))
		}
		if err != nil {
			return wrapInternal(err, "期首残高の取得")
		}
		if err := ensureYearOpen(ctx, tx, model.FiscalYear(year)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM opening_balances WHERE id = ?", id); err != nil {
			return wrapInternal(err, "期首残高の削除")
		}
		return nil
	})
}
