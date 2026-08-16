package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
)

// AccountRepository は repository.AccountRepository の SQLite 実装。
type AccountRepository struct {
	db *sql.DB
}

var _ repository.AccountRepository = (*AccountRepository)(nil)

func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

const accountColumns = "code, name, category, sub_category, tax_category, is_active, sort_order"

func scanAccount(scan func(dest ...any) error) (model.Account, error) {
	var a model.Account
	err := scan(&a.Code, &a.Name, &a.Category, &a.SubCategory, &a.TaxCategory, &a.IsActive, &a.SortOrder)
	return a, err
}

// FindAll は全科目をコード順に返す。
func (r *AccountRepository) FindAll(ctx context.Context) ([]model.Account, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+accountColumns+" FROM accounts ORDER BY code")
	if err != nil {
		return nil, wrapInternal(err, "勘定科目の取得")
	}
	defer func() { _ = rows.Close() }()

	var accounts []model.Account
	for rows.Next() {
		a, err := scanAccount(rows.Scan)
		if err != nil {
			return nil, wrapInternal(err, "勘定科目の読み取り")
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapInternal(err, "勘定科目の走査")
	}
	return accounts, nil
}

func (r *AccountRepository) FindByCode(ctx context.Context, code model.AccountCode) (*model.Account, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+accountColumns+" FROM accounts WHERE code = ?", code)
	a, err := scanAccount(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notFound("勘定科目 " + string(code))
	}
	if err != nil {
		return nil, wrapInternal(err, "勘定科目の取得")
	}
	return &a, nil
}

// SaveAll は科目マスタを一括登録する (存在するコードは更新)。
func (r *AccountRepository) SaveAll(ctx context.Context, accounts []model.Account) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO accounts (`+accountColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET
				name = excluded.name, category = excluded.category,
				sub_category = excluded.sub_category, tax_category = excluded.tax_category,
				is_active = excluded.is_active, sort_order = excluded.sort_order`)
		if err != nil {
			return wrapInternal(err, "勘定科目の保存準備")
		}
		defer func() { _ = stmt.Close() }()
		for i := range accounts {
			a := &accounts[i]
			if _, err := stmt.ExecContext(ctx,
				a.Code, a.Name, a.Category, a.SubCategory, a.TaxCategory, a.IsActive, a.SortOrder); err != nil {
				return wrapInternal(err, "勘定科目の保存")
			}
		}
		return nil
	})
}
