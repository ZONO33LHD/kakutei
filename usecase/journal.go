package usecase

import (
	"context"
	"fmt"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
	"github.com/ZONO33LHD/kakutei/domain/service/bookkeeping"
)

type AddJournalResult struct {
	ID       int64
	Warnings []bookkeeping.DuplicateWarning // force 登録時の類似警告
}

type JournalUsecase interface {
	// Add は仕訳を1件登録する。
	// 完全一致の重複は CodeConflict で拒否し、類似 (同日同額) は
	// force=false なら CodeConflict、force=true なら警告付きで登録する。
	Add(ctx context.Context, entry *model.JournalEntry, force bool) (*AddJournalResult, error)

	// AddBatch は複数の仕訳を全件成功 or 全件失敗で登録する。
	AddBatch(ctx context.Context, entries []model.JournalEntry, force bool) ([]int64, []bookkeeping.DuplicateWarning, error)

	Get(ctx context.Context, id int64) (*model.JournalEntry, error)

	Search(ctx context.Context, q repository.JournalSearchQuery) ([]model.JournalEntry, int, error)

	// Update は仕訳を訂正する (監査ログ付き)。
	Update(ctx context.Context, entry *model.JournalEntry) error

	// Delete は仕訳を削除する (監査ログ付き)。
	Delete(ctx context.Context, id int64) error

	// ListAuditLogs は仕訳の訂正・削除履歴を返す。
	ListAuditLogs(ctx context.Context, journalID int64) ([]model.JournalAuditLog, error)

	// CheckDuplicates は年度内の重複疑いペアを返す (申告前チェック)。
	CheckDuplicates(ctx context.Context, year model.FiscalYear, threshold int) (*bookkeeping.DuplicateCheckResult, error)
}

type journalUsecase struct {
	journals  repository.JournalRepository
	audits    repository.JournalAuditRepository
	duplicate *bookkeeping.DuplicateService
}

func NewJournalUsecase(
	journals repository.JournalRepository,
	audits repository.JournalAuditRepository,
	duplicate *bookkeeping.DuplicateService,
) JournalUsecase {
	return &journalUsecase{journals: journals, audits: audits, duplicate: duplicate}
}

// checkDuplicate は登録候補の重複を検査する。
// 戻り値: force 登録時に添える警告 (nil = 重複なし)。
//
// 既知の制限: 候補検索と登録は同一トランザクションではないため、並行リクエスト間の
// 「類似」重複は検出できない場合がある (完全一致は DB の一意制約が最終防衛線)。
// 本アプリは単一ユーザーのローカル利用を想定しており、許容するトレードオフ。
func (u *journalUsecase) checkDuplicate(
	ctx context.Context, entry *model.JournalEntry, force bool,
) (*bookkeeping.DuplicateWarning, error) {
	candidates, err := u.journals.FindDuplicateCandidates(
		ctx, entry.FiscalYear, entry.ContentHash(), entry.Date)
	if err != nil {
		return nil, err
	}
	warn := u.duplicate.CheckOnInsert(entry, candidates)
	if warn == nil {
		return nil, nil
	}
	// 完全一致は force でもブロック (二重計上の防止)
	if warn.Kind == bookkeeping.MatchExact {
		return nil, apperrors.New(apperrors.CodeConflict, warn.Reason)
	}
	if !force {
		return nil, apperrors.New(apperrors.CodeConflict,
			warn.Reason+" (登録する場合は force を指定してください)")
	}
	return warn, nil
}

func (u *journalUsecase) Add(ctx context.Context, entry *model.JournalEntry, force bool) (*AddJournalResult, error) {
	if err := entry.Validate(); err != nil {
		return nil, err
	}
	warn, err := u.checkDuplicate(ctx, entry, force)
	if err != nil {
		return nil, err
	}
	id, err := u.journals.Create(ctx, entry)
	if err != nil {
		return nil, err
	}
	result := &AddJournalResult{ID: id}
	if warn != nil {
		result.Warnings = append(result.Warnings, *warn)
	}
	return result, nil
}

func (u *journalUsecase) AddBatch(
	ctx context.Context, entries []model.JournalEntry, force bool,
) ([]int64, []bookkeeping.DuplicateWarning, error) {
	if len(entries) == 0 {
		return nil, nil, nil
	}
	var warnings []bookkeeping.DuplicateWarning
	type dateAmount struct {
		date  model.Date
		total model.Money
	}
	seenHashes := map[string]int{}
	seenSimilar := map[dateAmount]int{}
	for i := range entries {
		if err := entries[i].Validate(); err != nil {
			return nil, nil, apperrors.Wrap(err, apperrors.CodeBadRequest,
				formatEntryIndex(i)+"が不正です")
		}
		// バッチ内の完全一致は常にブロック
		hash := entries[i].ContentHash()
		if prev, ok := seenHashes[hash]; ok {
			return nil, nil, apperrors.Newf(apperrors.CodeConflict,
				"%sがバッチ内の%sと重複しています", formatEntryIndex(i), formatEntryIndex(prev))
		}
		seenHashes[hash] = i

		// バッチ内の類似 (同日・同借方合計) は force なしでブロック、force ありで警告
		key := dateAmount{entries[i].Date, entries[i].TotalDebit()}
		if prev, ok := seenSimilar[key]; ok {
			if !force {
				return nil, nil, apperrors.Newf(apperrors.CodeConflict,
					"%sがバッチ内の%sと同日・同額です (登録する場合は force を指定してください)",
					formatEntryIndex(i), formatEntryIndex(prev))
			}
			warnings = append(warnings, bookkeeping.DuplicateWarning{
				Kind:  bookkeeping.MatchSimilar,
				Score: 70,
				Reason: fmt.Sprintf("%sがバッチ内の%sと同日・同額です",
					formatEntryIndex(i), formatEntryIndex(prev)),
			})
		} else {
			seenSimilar[key] = i
		}

		warn, err := u.checkDuplicate(ctx, &entries[i], force)
		if err != nil {
			return nil, nil, apperrors.Wrap(err, apperrors.CodeOf(err), formatEntryIndex(i))
		}
		if warn != nil {
			warnings = append(warnings, *warn)
		}
	}
	ids, err := u.journals.CreateBatch(ctx, entries)
	if err != nil {
		return nil, nil, err
	}
	return ids, warnings, nil
}

func formatEntryIndex(i int) string {
	return fmt.Sprintf("仕訳 %d 件目", i+1)
}

func (u *journalUsecase) Get(ctx context.Context, id int64) (*model.JournalEntry, error) {
	return u.journals.FindByID(ctx, id)
}

func (u *journalUsecase) Search(ctx context.Context, q repository.JournalSearchQuery) ([]model.JournalEntry, int, error) {
	if err := q.FiscalYear.Validate(); err != nil {
		return nil, 0, err
	}
	return u.journals.Search(ctx, q)
}

// Update は仕訳を訂正する。
// 完全一致の重複は DB の一意制約で拒否される。類似 (同日同額) の警告は
// 登録時のみで、訂正では行わない (訂正は既存取引の修正であり誤爆が多いため)。
func (u *journalUsecase) Update(ctx context.Context, entry *model.JournalEntry) error {
	if entry.ID <= 0 {
		return apperrors.New(apperrors.CodeBadRequest, "仕訳IDが不正です")
	}
	if err := entry.Validate(); err != nil {
		return err
	}
	return u.journals.Update(ctx, entry)
}

func (u *journalUsecase) Delete(ctx context.Context, id int64) error {
	return u.journals.Delete(ctx, id)
}

func (u *journalUsecase) ListAuditLogs(ctx context.Context, journalID int64) ([]model.JournalAuditLog, error) {
	return u.audits.ListByJournalID(ctx, journalID)
}

// defaultDuplicateThreshold は申告前重複チェックの既定閾値 (類似以上)。
const defaultDuplicateThreshold = 70

func (u *journalUsecase) CheckDuplicates(
	ctx context.Context, year model.FiscalYear, threshold int,
) (*bookkeeping.DuplicateCheckResult, error) {
	if err := year.Validate(); err != nil {
		return nil, err
	}
	if threshold <= 0 {
		threshold = defaultDuplicateThreshold
	}
	entries, err := u.journals.ListByFiscalYear(ctx, year)
	if err != nil {
		return nil, err
	}
	return u.duplicate.FindDuplicatePairs(entries, threshold), nil
}
