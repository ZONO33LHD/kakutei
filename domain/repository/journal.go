// Package repository はドメイン層が要求する永続化の契約 (インターフェース) を定義する。
//
// 実装は infrastructure/persistence が提供し、依存方向は
// usecase → domain/repository ← infrastructure/persistence となる。
package repository

import (
	"context"

	"github.com/ZONO33LHD/kakutei/domain/model"
)

// JournalSearchQuery は仕訳検索の条件。ゼロ値のフィールドは条件なしを意味する。
type JournalSearchQuery struct {
	FiscalYear           model.FiscalYear // 必須
	DateFrom             model.Date
	DateTo               model.Date
	AccountCode          model.AccountCode
	DescriptionContains  string
	CounterpartyContains string
	AmountMin            model.Money // 借方合計の下限 (0 = 条件なし)
	AmountMax            model.Money // 借方合計の上限 (0 = 条件なし)
	Source               model.JournalSource
	Limit                int // 0 = 既定値 (実装依存)
	Offset               int
}

// JournalRepository は仕訳の永続化契約。
//
// 内容ハッシュ (model.JournalEntry.ContentHash) は実装側で導出して保存し、
// (年度, ハッシュ) に一意制約を持つこと (重複登録の最終防衛線)。
// 訂正・削除は監査ログ (電帳法対応) と同一トランザクションで永続化すること。
type JournalRepository interface {
	// Create は仕訳と明細をトランザクションで保存し、採番された ID を返す。
	// 内容ハッシュが既存仕訳と衝突する場合は CodeConflict。
	Create(ctx context.Context, entry *model.JournalEntry) (int64, error)

	// CreateBatch は複数の仕訳を単一トランザクションで保存する (全件成功 or 全件失敗)。
	CreateBatch(ctx context.Context, entries []model.JournalEntry) ([]int64, error)

	// FindByID は仕訳を明細込みで取得する。存在しなければ CodeNotFound。
	FindByID(ctx context.Context, id int64) (*model.JournalEntry, error)

	// Search は条件に合致する仕訳と総件数 (Limit 適用前) を返す。
	Search(ctx context.Context, q JournalSearchQuery) (entries []model.JournalEntry, totalCount int, err error)

	// ListByFiscalYear は年度内の全仕訳を明細込みで返す (財務諸表・重複スキャン用)。
	ListByFiscalYear(ctx context.Context, year model.FiscalYear) ([]model.JournalEntry, error)

	// FindDuplicateCandidates は重複検出の候補 (同一内容ハッシュ または 同一日付) の
	// 既存仕訳を返す。判定ロジックは domain/service/bookkeeping.DuplicateService が担う。
	FindDuplicateCandidates(ctx context.Context, year model.FiscalYear, contentHash string, date model.Date) ([]model.JournalEntry, error)

	// Update は仕訳を明細ごと差し替え、監査ログを同一トランザクションで記録する。
	// 存在しなければ CodeNotFound。
	Update(ctx context.Context, entry *model.JournalEntry, audit *model.JournalAuditLog) error

	// Delete は仕訳を明細ごと削除し、監査ログを同一トランザクションで記録する。
	// 存在しなければ CodeNotFound。
	Delete(ctx context.Context, id int64, audit *model.JournalAuditLog) error
}

// JournalAuditRepository は仕訳の訂正・削除履歴の読み取り契約。
// 書き込みは JournalRepository.Update / Delete が原子的に行うため、追記 API は提供しない。
type JournalAuditRepository interface {
	ListByJournalID(ctx context.Context, journalID int64) ([]model.JournalAuditLog, error)
	ListByFiscalYear(ctx context.Context, year model.FiscalYear) ([]model.JournalAuditLog, error)
}
