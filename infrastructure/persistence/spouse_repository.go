package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
)

// spouseKind は filing_records 上の配偶者情報の kind (年度に1件、部分一意インデックス対象)。
const spouseKind = "spouse"

// SpouseRepository は repository.SpouseRepository の SQLite 実装。
type SpouseRepository struct {
	db *sql.DB
}

var _ repository.SpouseRepository = (*SpouseRepository)(nil)

func NewSpouseRepository(db *sql.DB) *SpouseRepository {
	return &SpouseRepository{db: db}
}

// Upsert は年度をキーに配偶者情報を登録・更新する。
func (r *SpouseRepository) Upsert(ctx context.Context, spouse *model.Spouse) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := ensureYearOpen(ctx, tx, spouse.FiscalYear); err != nil {
			return err
		}
		var id int64
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM filing_records WHERE fiscal_year = ? AND kind = ?",
			int(spouse.FiscalYear), spouseKind).Scan(&id)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			result, err := tx.ExecContext(ctx,
				"INSERT INTO filing_records (fiscal_year, kind, data) VALUES (?, ?, '{}')",
				int(spouse.FiscalYear), spouseKind)
			if err != nil {
				return wrapInternal(err, "配偶者情報の保存")
			}
			if id, err = result.LastInsertId(); err != nil {
				return wrapInternal(err, "配偶者情報のID取得")
			}
		case err != nil:
			return wrapInternal(err, "配偶者情報の取得")
		}

		spouse.ID = id
		data, err := json.Marshal(spouse)
		if err != nil {
			return wrapInternal(err, "配偶者情報のシリアライズ")
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE filing_records SET data = ?, updated_at = datetime('now') WHERE id = ?",
			string(data), id); err != nil {
			return wrapInternal(err, "配偶者情報の更新")
		}
		return nil
	})
}

// FindByFiscalYear は年度の配偶者情報を返す。未登録なら (nil, nil)。
func (r *SpouseRepository) FindByFiscalYear(ctx context.Context, year model.FiscalYear) (*model.Spouse, error) {
	var id int64
	var data string
	err := r.db.QueryRowContext(ctx,
		"SELECT id, data FROM filing_records WHERE fiscal_year = ? AND kind = ?",
		int(year), spouseKind).Scan(&id, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrapInternal(err, "配偶者情報の取得")
	}
	var spouse model.Spouse
	if err := json.Unmarshal([]byte(data), &spouse); err != nil {
		return nil, wrapInternal(err, "配偶者情報の復元")
	}
	// ID と年度は列の値を正とする (汎用 filingRepo と同じ方針)
	spouse.ID = id
	spouse.FiscalYear = year
	return &spouse, nil
}

// DeleteByFiscalYear は年度の配偶者情報を削除する。未登録でもエラーにしない。
func (r *SpouseRepository) DeleteByFiscalYear(ctx context.Context, year model.FiscalYear) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := ensureYearOpen(ctx, tx, year); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM filing_records WHERE fiscal_year = ? AND kind = ?",
			int(year), spouseKind); err != nil {
			return wrapInternal(err, "配偶者情報の削除")
		}
		return nil
	})
}
