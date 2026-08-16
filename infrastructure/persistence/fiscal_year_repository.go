package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
)

// FiscalYearRepository は repository.FiscalYearRepository の SQLite 実装。
type FiscalYearRepository struct {
	db *sql.DB
}

var _ repository.FiscalYearRepository = (*FiscalYearRepository)(nil)

// NewFiscalYearRepository は FiscalYearRepository を生成する。
func NewFiscalYearRepository(db *sql.DB) *FiscalYearRepository {
	return &FiscalYearRepository{db: db}
}

// Create は年度を open 状態で作成する。
func (r *FiscalYearRepository) Create(ctx context.Context, year model.FiscalYear) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO fiscal_years (year, state) VALUES (?, ?)", int(year), model.FiscalYearOpen)
	if isUniqueViolation(err) {
		return apperrors.Newf(apperrors.CodeConflict, "年度 %d は既に存在します", int(year))
	}
	if err != nil {
		return wrapInternal(err, "年度の作成")
	}
	return nil
}

func scanFiscalYear(scan func(dest ...any) error) (*model.FiscalYearStatus, error) {
	var s model.FiscalYearStatus
	var createdAt string
	if err := scan(&s.Year, &s.State, &createdAt); err != nil {
		return nil, err
	}
	if t, err := time.Parse("2006-01-02 15:04:05", createdAt); err == nil {
		s.CreatedAt = t
	}
	return &s, nil
}

// Find は年度を取得する。
func (r *FiscalYearRepository) Find(ctx context.Context, year model.FiscalYear) (*model.FiscalYearStatus, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT year, state, created_at FROM fiscal_years WHERE year = ?", int(year))
	s, err := scanFiscalYear(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.Newf(apperrors.CodeNotFound, "年度 %d が見つかりません", int(year))
	}
	if err != nil {
		return nil, wrapInternal(err, "年度の取得")
	}
	return s, nil
}

// List は全年度を昇順で返す。
func (r *FiscalYearRepository) List(ctx context.Context) ([]model.FiscalYearStatus, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT year, state, created_at FROM fiscal_years ORDER BY year")
	if err != nil {
		return nil, wrapInternal(err, "年度一覧の取得")
	}
	defer func() { _ = rows.Close() }()

	var years []model.FiscalYearStatus
	for rows.Next() {
		s, err := scanFiscalYear(rows.Scan)
		if err != nil {
			return nil, wrapInternal(err, "年度の読み取り")
		}
		years = append(years, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapInternal(err, "年度一覧の走査")
	}
	return years, nil
}

// UpdateState は年度の開閉状態を変更する。
func (r *FiscalYearRepository) UpdateState(ctx context.Context, year model.FiscalYear, state model.FiscalYearState) error {
	if err := state.Validate(); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx,
		"UPDATE fiscal_years SET state = ? WHERE year = ?", state, int(year))
	if err != nil {
		return wrapInternal(err, "年度状態の更新")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return wrapInternal(err, "年度状態の更新確認")
	}
	if affected == 0 {
		return apperrors.Newf(apperrors.CodeNotFound, "年度 %d が見つかりません", int(year))
	}
	return nil
}
