package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
)

// JournalRepository は repository.JournalRepository の SQLite 実装。
type JournalRepository struct {
	db *sql.DB
}

var _ repository.JournalRepository = (*JournalRepository)(nil)

func NewJournalRepository(db *sql.DB) *JournalRepository {
	return &JournalRepository{db: db}
}

// queryer は *sql.DB / *sql.Tx の共通操作。
type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ensureYearOpen は年度が存在し open であることを検証する。
func ensureYearOpen(ctx context.Context, q queryer, year model.FiscalYear) error {
	var state string
	err := q.QueryRowContext(ctx, "SELECT state FROM fiscal_years WHERE year = ?", int(year)).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return apperrors.Newf(apperrors.CodeBadRequest,
			"年度 %d が作成されていません。先に年度を作成してください", int(year))
	}
	if err != nil {
		return wrapInternal(err, "年度状態の確認")
	}
	if model.FiscalYearState(state) == model.FiscalYearClosed {
		return apperrors.Newf(apperrors.CodeConflict, "年度 %d は締め済みのため変更できません", int(year))
	}
	return nil
}

// insertJournal は仕訳ヘッダと明細を挿入して ID を返す (トランザクション内で呼ぶ)。
func insertJournal(ctx context.Context, tx *sql.Tx, entry *model.JournalEntry) (int64, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO journals (fiscal_year, date, description, counterparty, content_hash,
			source, source_file, is_adjustment)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		int(entry.FiscalYear), entry.Date.String(), entry.Description, entry.Counterparty,
		entry.ContentHash(), entry.Source, entry.SourceFile, entry.IsAdjustment)
	if isUniqueViolation(err) {
		return 0, apperrors.New(apperrors.CodeConflict, "同一内容の仕訳が既に登録されています")
	}
	if err != nil {
		return 0, wrapInternal(err, "仕訳の保存")
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, wrapInternal(err, "仕訳IDの取得")
	}
	if err := insertLines(ctx, tx, id, entry.Lines); err != nil {
		return 0, err
	}
	return id, nil
}

func insertLines(ctx context.Context, tx *sql.Tx, journalID int64, lines []model.JournalLine) error {
	for i := range lines {
		l := &lines[i]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO journal_lines (journal_id, side, account_code, amount, tax_category, tax_amount)
			VALUES (?, ?, ?, ?, ?, ?)`,
			journalID, l.Side, l.AccountCode, l.Amount.Yen(), l.TaxCategory, l.TaxAmount.Yen()); err != nil {
			return wrapInternal(err, "仕訳明細の保存")
		}
	}
	return nil
}

// Create は仕訳と明細をトランザクションで保存する。
func (r *JournalRepository) Create(ctx context.Context, entry *model.JournalEntry) (int64, error) {
	var id int64
	err := inTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := ensureYearOpen(ctx, tx, entry.FiscalYear); err != nil {
			return err
		}
		var err error
		id, err = insertJournal(ctx, tx, entry)
		return err
	})
	return id, err
}

// CreateBatch は複数の仕訳を単一トランザクションで保存する。
func (r *JournalRepository) CreateBatch(ctx context.Context, entries []model.JournalEntry) ([]int64, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(entries))
	err := inTx(ctx, r.db, func(tx *sql.Tx) error {
		// バッチ内の全年度を検証する (混在バッチによる締め済み年度への追加を防ぐ)
		checkedYears := map[model.FiscalYear]struct{}{}
		for i := range entries {
			year := entries[i].FiscalYear
			if _, ok := checkedYears[year]; ok {
				continue
			}
			if err := ensureYearOpen(ctx, tx, year); err != nil {
				return err
			}
			checkedYears[year] = struct{}{}
		}
		for i := range entries {
			id, err := insertJournal(ctx, tx, &entries[i])
			if err != nil {
				// 何件目で失敗したかを原因メッセージに前置し、エラーコードは原因のものを保つ
				msg := apperrors.MessageOf(err)
				if msg == "" {
					msg = "仕訳の登録に失敗しました"
				}
				return apperrors.Wrap(err, apperrors.CodeOf(err), fmt.Sprintf("%d 件目: %s", i+1, msg))
			}
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

type journalRow struct {
	id           int64
	fiscalYear   int
	date         string
	description  string
	counterparty string
	source       string
	sourceFile   string
	isAdjustment bool
}

const journalColumns = "id, fiscal_year, date, description, counterparty, source, source_file, is_adjustment"

func scanJournalRow(scan func(dest ...any) error) (journalRow, error) {
	var jr journalRow
	err := scan(&jr.id, &jr.fiscalYear, &jr.date, &jr.description, &jr.counterparty,
		&jr.source, &jr.sourceFile, &jr.isAdjustment)
	return jr, err
}

func (jr *journalRow) toEntry() (*model.JournalEntry, error) {
	date, err := model.ParseDate(jr.date)
	if err != nil {
		return nil, wrapInternal(err, "仕訳日付の復元")
	}
	return &model.JournalEntry{
		ID:           jr.id,
		FiscalYear:   model.FiscalYear(jr.fiscalYear),
		Date:         date,
		Description:  jr.description,
		Counterparty: jr.counterparty,
		Source:       model.JournalSource(jr.source),
		SourceFile:   jr.sourceFile,
		IsAdjustment: jr.isAdjustment,
	}, nil
}

// loadEntries は条件付きで仕訳を取得し、明細をまとめてロードする。
func loadEntries(ctx context.Context, q queryer, where string, args ...any) ([]model.JournalEntry, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT "+journalColumns+" FROM journals "+where, args...)
	if err != nil {
		return nil, wrapInternal(err, "仕訳の取得")
	}
	defer func() { _ = rows.Close() }()

	var entries []model.JournalEntry
	ids := []any{}
	indexByID := map[int64]int{}
	for rows.Next() {
		jr, err := scanJournalRow(rows.Scan)
		if err != nil {
			return nil, wrapInternal(err, "仕訳の読み取り")
		}
		entry, err := jr.toEntry()
		if err != nil {
			return nil, err
		}
		indexByID[entry.ID] = len(entries)
		entries = append(entries, *entry)
		ids = append(ids, entry.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapInternal(err, "仕訳の走査")
	}
	if len(entries) == 0 {
		return nil, nil
	}
	if err := attachLines(ctx, q, entries, indexByID, ids); err != nil {
		return nil, err
	}
	return entries, nil
}

// attachLinesChunkSize は IN 句の変数上限 (SQLite の既定 32,766) を超えないための分割単位。
const attachLinesChunkSize = 500

// attachLines は明細を一括取得して各仕訳に紐付ける。
// ID リストは SQLite のプレースホルダ上限を超えないようチャンク分割する。
func attachLines(ctx context.Context, q queryer, entries []model.JournalEntry, indexByID map[int64]int, ids []any) error {
	for start := 0; start < len(ids); start += attachLinesChunkSize {
		end := min(start+attachLinesChunkSize, len(ids))
		if err := attachLinesChunk(ctx, q, entries, indexByID, ids[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func attachLinesChunk(ctx context.Context, q queryer, entries []model.JournalEntry, indexByID map[int64]int, ids []any) error {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := q.QueryContext(ctx, `
		SELECT journal_id, id, side, account_code, amount, tax_category, tax_amount
		FROM journal_lines WHERE journal_id IN (`+placeholders+`) ORDER BY journal_id, id`, ids...)
	if err != nil {
		return wrapInternal(err, "仕訳明細の取得")
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var journalID int64
		var line model.JournalLine
		var amount, taxAmount int64
		if err := rows.Scan(&journalID, &line.ID, &line.Side, &line.AccountCode,
			&amount, &line.TaxCategory, &taxAmount); err != nil {
			return wrapInternal(err, "仕訳明細の読み取り")
		}
		line.Amount = model.Money(amount)
		line.TaxAmount = model.Money(taxAmount)
		idx := indexByID[journalID]
		entries[idx].Lines = append(entries[idx].Lines, line)
	}
	if err := rows.Err(); err != nil {
		return wrapInternal(err, "仕訳明細の走査")
	}
	return nil
}

// FindByID は仕訳を明細込みで取得する。
func (r *JournalRepository) FindByID(ctx context.Context, id int64) (*model.JournalEntry, error) {
	entries, err := loadEntries(ctx, r.db, "WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, notFound(fmt.Sprintf("仕訳 (ID: %d)", id))
	}
	return &entries[0], nil
}

func (r *JournalRepository) ListByFiscalYear(ctx context.Context, year model.FiscalYear) ([]model.JournalEntry, error) {
	return loadEntries(ctx, r.db, "WHERE fiscal_year = ? ORDER BY date, id", int(year))
}

// FindDuplicateCandidates は同一内容ハッシュまたは同一日付の既存仕訳を返す。
func (r *JournalRepository) FindDuplicateCandidates(
	ctx context.Context, year model.FiscalYear, contentHash string, date model.Date,
) ([]model.JournalEntry, error) {
	return loadEntries(ctx, r.db,
		"WHERE fiscal_year = ? AND (content_hash = ? OR date = ?) ORDER BY id",
		int(year), contentHash, date.String())
}

// Search は条件に合致する仕訳と総件数を返す。
// 件数とページを同一トランザクションで取得し、割り込み書き込みによる不整合を防ぐ。
func (r *JournalRepository) Search(ctx context.Context, q repository.JournalSearchQuery) ([]model.JournalEntry, int, error) {
	where, args := buildSearchWhere(q)

	var total int
	var entries []model.JournalEntry
	err := inTx(ctx, r.db, func(tx *sql.Tx) error {
		countArgs := make([]any, len(args))
		copy(countArgs, args)
		if err := tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM journals "+where, countArgs...).Scan(&total); err != nil {
			return wrapInternal(err, "仕訳件数の取得")
		}

		limit := q.Limit
		if limit <= 0 {
			limit = 100
		}
		pageArgs := append(append([]any{}, args...), limit, q.Offset)
		var err error
		entries, err = loadEntries(ctx, tx, where+" ORDER BY date, id LIMIT ? OFFSET ?", pageArgs...)
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func buildSearchWhere(q repository.JournalSearchQuery) (string, []any) {
	conds := []string{"fiscal_year = ?"}
	args := []any{int(q.FiscalYear)}

	if !q.DateFrom.IsZero() {
		conds = append(conds, "date >= ?")
		args = append(args, q.DateFrom.String())
	}
	if !q.DateTo.IsZero() {
		conds = append(conds, "date <= ?")
		args = append(args, q.DateTo.String())
	}
	if q.AccountCode != "" {
		conds = append(conds, "id IN (SELECT journal_id FROM journal_lines WHERE account_code = ?)")
		args = append(args, q.AccountCode)
	}
	if q.DescriptionContains != "" {
		conds = append(conds, "description LIKE ? ESCAPE '\\'")
		args = append(args, "%"+escapeLike(q.DescriptionContains)+"%")
	}
	if q.CounterpartyContains != "" {
		conds = append(conds, "counterparty LIKE ? ESCAPE '\\'")
		args = append(args, "%"+escapeLike(q.CounterpartyContains)+"%")
	}
	if q.AmountMin > 0 {
		conds = append(conds, "(SELECT COALESCE(SUM(amount),0) FROM journal_lines WHERE journal_id = journals.id AND side = 'debit') >= ?")
		args = append(args, q.AmountMin.Yen())
	}
	if q.AmountMax > 0 {
		conds = append(conds, "(SELECT COALESCE(SUM(amount),0) FROM journal_lines WHERE journal_id = journals.id AND side = 'debit') <= ?")
		args = append(args, q.AmountMax.Yen())
	}
	if q.Source != "" {
		conds = append(conds, "source = ?")
		args = append(args, q.Source)
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// snapshotJSON は監査ログ用の仕訳スナップショット (JSON) を生成する。
func snapshotJSON(entry *model.JournalEntry) (string, error) {
	b, err := json.Marshal(entry)
	if err != nil {
		return "", wrapInternal(err, "監査スナップショットの生成")
	}
	return string(b), nil
}

// loadEntryInTx はトランザクション内で仕訳を取得する (監査スナップショット用)。
func loadEntryInTx(ctx context.Context, tx *sql.Tx, id int64) (*model.JournalEntry, error) {
	entries, err := loadEntries(ctx, tx, "WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, notFound(fmt.Sprintf("仕訳 (ID: %d)", id))
	}
	return &entries[0], nil
}

// insertAuditLog は監査ログを記録する (トランザクション内で呼ぶ)。
func insertAuditLog(ctx context.Context, tx *sql.Tx, log *model.JournalAuditLog) error {
	if err := log.Validate(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO journal_audit_logs (journal_id, fiscal_year, operation, before_snapshot, after_snapshot)
		VALUES (?, ?, ?, ?, ?)`,
		log.JournalID, int(log.FiscalYear), log.Operation, log.BeforeSnapshot, log.AfterSnapshot); err != nil {
		return wrapInternal(err, "監査ログの記録")
	}
	return nil
}

// Update は仕訳を明細ごと差し替え、更新前スナップショットの監査ログを
// 同一トランザクションで記録する。
func (r *JournalRepository) Update(ctx context.Context, entry *model.JournalEntry) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		current, err := loadEntryInTx(ctx, tx, entry.ID)
		if err != nil {
			return err
		}
		if err := ensureYearOpen(ctx, tx, current.FiscalYear); err != nil {
			return err
		}
		if entry.FiscalYear != current.FiscalYear {
			return apperrors.New(apperrors.CodeBadRequest, "仕訳の年度は変更できません")
		}

		before, err := snapshotJSON(current)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE journals SET date = ?, description = ?, counterparty = ?, content_hash = ?,
				source = ?, source_file = ?, is_adjustment = ?, updated_at = datetime('now')
			WHERE id = ?`,
			entry.Date.String(), entry.Description, entry.Counterparty, entry.ContentHash(),
			entry.Source, entry.SourceFile, entry.IsAdjustment, entry.ID); err != nil {
			if isUniqueViolation(err) {
				return apperrors.New(apperrors.CodeConflict, "同一内容の仕訳が既に登録されています")
			}
			return wrapInternal(err, "仕訳の更新")
		}
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM journal_lines WHERE journal_id = ?", entry.ID); err != nil {
			return wrapInternal(err, "仕訳明細の差し替え")
		}
		if err := insertLines(ctx, tx, entry.ID, entry.Lines); err != nil {
			return err
		}

		// AfterSnapshot は実際に保存された状態 (明細IDの再採番を含む) を再読込して生成する
		stored, err := loadEntryInTx(ctx, tx, entry.ID)
		if err != nil {
			return err
		}
		after, err := snapshotJSON(stored)
		if err != nil {
			return err
		}

		return insertAuditLog(ctx, tx, &model.JournalAuditLog{
			JournalID:      entry.ID,
			FiscalYear:     current.FiscalYear,
			Operation:      model.AuditUpdate,
			BeforeSnapshot: before,
			AfterSnapshot:  after,
		})
	})
}

// Delete は仕訳を明細ごと削除し、監査ログを同一トランザクションで記録する。
func (r *JournalRepository) Delete(ctx context.Context, id int64) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		current, err := loadEntryInTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := ensureYearOpen(ctx, tx, current.FiscalYear); err != nil {
			return err
		}
		before, err := snapshotJSON(current)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM journals WHERE id = ?", id); err != nil {
			return wrapInternal(err, "仕訳の削除")
		}
		return insertAuditLog(ctx, tx, &model.JournalAuditLog{
			JournalID:      id,
			FiscalYear:     current.FiscalYear,
			Operation:      model.AuditDelete,
			BeforeSnapshot: before,
		})
	})
}
