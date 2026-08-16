package bookkeeping

import (
	"fmt"
	"sort"

	"github.com/ZONO33LHD/kakutei/domain/model"
)

// DuplicateMatchKind は重複判定の種別。
type DuplicateMatchKind string

const (
	MatchExact   DuplicateMatchKind = "exact"   // 内容ハッシュ完全一致 → 登録をブロック
	MatchSimilar DuplicateMatchKind = "similar" // 同一日付・同一金額 → 警告のみ
)

// 重複スコア。
const (
	scoreExact          = 100 // ハッシュ完全一致
	scoreSameDateAmount = 90  // 同日・同額かつ科目が部分一致
	scoreSimilar        = 70  // 同日・同額のみ
)

// DuplicateWarning は登録時の重複警告。
type DuplicateWarning struct {
	Kind              DuplicateMatchKind
	Score             int
	ExistingJournalID int64
	Reason            string
}

// DuplicateService は仕訳の重複検出を行うドメインサービス。
type DuplicateService struct{}

// NewDuplicateService は DuplicateService を生成する。
func NewDuplicateService() *DuplicateService { return &DuplicateService{} }

// CheckOnInsert は登録候補の仕訳を既存仕訳 (同一年度) と照合する。
//
// candidates には内容ハッシュ一致または同一日付の既存仕訳を渡す
// (絞り込みはリポジトリ側で行い、判定ロジックはここに集約する)。
// 完全一致 (ハッシュ) → MatchExact、同日・同借方合計 → MatchSimilar。
// 重複がなければ nil を返す。
func (s *DuplicateService) CheckOnInsert(
	entry *model.JournalEntry, candidates []model.JournalEntry,
) *DuplicateWarning {
	hash := entry.ContentHash()
	totalDebit := entry.TotalDebit()

	// 完全一致を優先して検出
	for i := range candidates {
		if candidates[i].ContentHash() == hash {
			return &DuplicateWarning{
				Kind:              MatchExact,
				Score:             scoreExact,
				ExistingJournalID: candidates[i].ID,
				Reason:            fmt.Sprintf("完全一致する仕訳が既に存在します (ID: %d)", candidates[i].ID),
			}
		}
	}
	for i := range candidates {
		c := &candidates[i]
		if c.Date == entry.Date && c.TotalDebit() == totalDebit {
			return &DuplicateWarning{
				Kind:              MatchSimilar,
				Score:             scoreSimilar,
				ExistingJournalID: c.ID,
				Reason: fmt.Sprintf("同一日付・同一金額の仕訳が存在します (ID: %d, %q)",
					c.ID, c.Description),
			}
		}
	}
	return nil
}

// DuplicatePair は申告前チェックで検出した重複疑いのペア。
type DuplicatePair struct {
	JournalIDA int64
	JournalIDB int64
	Score      int
	Reason     string
}

// DuplicateCheckResult は年度全体の重複スキャン結果。
type DuplicateCheckResult struct {
	Pairs          []DuplicatePair
	ExactCount     int // score 100 のペア数
	SuspectedCount int // score < 100 のペア数
}

// FindDuplicatePairs は年度内の全仕訳を走査して重複疑いのペアを返す。
//
// スコア: 100 = ハッシュ完全一致 / 90 = 同日・同額・科目が部分一致 / 70 = 同日・同額。
// threshold 以上のスコアのペアのみ返す。
func (s *DuplicateService) FindDuplicatePairs(
	entries []model.JournalEntry, threshold int,
) *DuplicateCheckResult {
	result := &DuplicateCheckResult{}

	// 同日・同借方合計のグループにまとめてから比較する (全ペア比較の回避)。
	type groupKey struct {
		date  model.Date
		total model.Money
	}
	groups := map[groupKey][]int{}
	for i := range entries {
		key := groupKey{entries[i].Date, entries[i].TotalDebit()}
		groups[key] = append(groups[key], i)
	}

	for _, indices := range groups {
		for a := 0; a < len(indices); a++ {
			for b := a + 1; b < len(indices); b++ {
				ea, eb := &entries[indices[a]], &entries[indices[b]]
				score, reason := scorePair(ea, eb)
				if score < threshold {
					continue
				}
				result.Pairs = append(result.Pairs, DuplicatePair{
					JournalIDA: ea.ID,
					JournalIDB: eb.ID,
					Score:      score,
					Reason:     reason,
				})
				if score == scoreExact {
					result.ExactCount++
				} else {
					result.SuspectedCount++
				}
			}
		}
	}
	// map 走査による非決定性を排除し、結果を仕訳IDの昇順で安定させる
	sort.Slice(result.Pairs, func(i, j int) bool {
		a, b := result.Pairs[i], result.Pairs[j]
		if a.JournalIDA != b.JournalIDA {
			return a.JournalIDA < b.JournalIDA
		}
		return a.JournalIDB < b.JournalIDB
	})
	return result
}

// scorePair は同日・同借方合計のペアのスコアを判定する。
func scorePair(a, b *model.JournalEntry) (int, string) {
	if a.ContentHash() == b.ContentHash() {
		return scoreExact, "仕訳内容が完全に一致しています"
	}
	if sharesAccount(a, b) {
		return scoreSameDateAmount, "同一日付・同一金額で勘定科目が部分一致しています"
	}
	return scoreSimilar, "同一日付・同一金額の仕訳です"
}

// sharesAccount は2つの仕訳が借方側で同じ勘定科目を含むかどうか。
// 貸方は決済口座 (普通預金等) の共有が常態のため比較対象にしない —
// 「何に使ったか (借方)」が一致する場合のみ高スコアとする。
func sharesAccount(a, b *model.JournalEntry) bool {
	codes := map[model.AccountCode]struct{}{}
	for i := range a.Lines {
		if a.Lines[i].Side == model.SideDebit {
			codes[a.Lines[i].AccountCode] = struct{}{}
		}
	}
	for i := range b.Lines {
		if b.Lines[i].Side != model.SideDebit {
			continue
		}
		if _, ok := codes[b.Lines[i].AccountCode]; ok {
			return true
		}
	}
	return false
}
