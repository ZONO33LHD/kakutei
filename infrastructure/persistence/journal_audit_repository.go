package persistence

import (
	"context"
	"database/sql"
	"time"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
)

// JournalAuditRepository は repository.JournalAuditRepository の SQLite 実装 (読み取り専用)。
// 書き込みは JournalRepository.Update / Delete が同一トランザクションで行う。
type JournalAuditRepository struct {
	db *sql.DB
}

var _ repository.JournalAuditRepository = (*JournalAuditRepository)(nil)

// NewJournalAuditRepository は JournalAuditRepository を生成する。
func NewJournalAuditRepository(db *sql.DB) *JournalAuditRepository {
	return &JournalAuditRepository{db: db}
}

func (r *JournalAuditRepository) list(ctx context.Context, where string, args ...any) ([]model.JournalAuditLog, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, journal_id, fiscal_year, operation, before_snapshot, after_snapshot, created_at
		FROM journal_audit_logs `+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, wrapInternal(err, "監査ログの取得")
	}
	defer func() { _ = rows.Close() }()

	var logs []model.JournalAuditLog
	for rows.Next() {
		var l model.JournalAuditLog
		var createdAt string
		if err := rows.Scan(&l.ID, &l.JournalID, &l.FiscalYear, &l.Operation,
			&l.BeforeSnapshot, &l.AfterSnapshot, &createdAt); err != nil {
			return nil, wrapInternal(err, "監査ログの読み取り")
		}
		if t, err := time.Parse("2006-01-02 15:04:05", createdAt); err == nil {
			l.CreatedAt = t
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapInternal(err, "監査ログの走査")
	}
	return logs, nil
}

// ListByJournalID は仕訳の訂正・削除履歴を返す。
func (r *JournalAuditRepository) ListByJournalID(ctx context.Context, journalID int64) ([]model.JournalAuditLog, error) {
	return r.list(ctx, "WHERE journal_id = ?", journalID)
}

// ListByFiscalYear は年度内の全履歴を返す。
func (r *JournalAuditRepository) ListByFiscalYear(ctx context.Context, year model.FiscalYear) ([]model.JournalAuditLog, error) {
	return r.list(ctx, "WHERE fiscal_year = ?", int(year))
}
