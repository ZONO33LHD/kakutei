package model

import (
	"strings"
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

func validEntry() JournalEntry {
	d, _ := ParseDate("2025-04-01")
	return JournalEntry{
		FiscalYear:  2025,
		Date:        d,
		Description: "サービス売上",
		Lines: []JournalLine{
			{Side: SideDebit, AccountCode: "1002", Amount: 110_000},
			{Side: SideCredit, AccountCode: "4001", Amount: 110_000, TaxCategory: LineTaxTaxable10, TaxAmount: 10_000},
		},
		Source: SourceManual,
	}
}

func TestJournalEntryValidateOK(t *testing.T) {
	e := validEntry()
	if err := e.Validate(); err != nil {
		t.Fatalf("valid entry がエラー: %v", err)
	}
}

func TestJournalEntryValidateErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(e *JournalEntry)
	}{
		{"日付なし", func(e *JournalEntry) { e.Date = Date{} }},
		{"年度外の日付", func(e *JournalEntry) { e.Date, _ = ParseDate("2024-12-31") }},
		{"明細1行", func(e *JournalEntry) { e.Lines = e.Lines[:1] }},
		{"貸借不一致", func(e *JournalEntry) { e.Lines[0].Amount = 100_000 }},
		{"金額ゼロ", func(e *JournalEntry) {
			e.Lines[0].Amount = 0
			e.Lines[1].Amount = 0
		}},
		{"負の金額", func(e *JournalEntry) {
			e.Lines[0].Amount = -100
			e.Lines[1].Amount = -100
		}},
		{"不正な side", func(e *JournalEntry) { e.Lines[0].Side = "middle" }},
		{"不正な科目コード", func(e *JournalEntry) { e.Lines[0].AccountCode = "abc" }},
		{"内消費税が本体超過", func(e *JournalEntry) { e.Lines[1].TaxAmount = 200_000 }},
		{"不正な税区分", func(e *JournalEntry) { e.Lines[1].TaxCategory = "taxable_5" }},
		{"不正な入力元", func(e *JournalEntry) { e.Source = "email" }},
		{"不正な年度", func(e *JournalEntry) { e.FiscalYear = 25 }},
		{"摘要が長すぎる", func(e *JournalEntry) { e.Description = strings.Repeat("あ", 501) }},
		{"金額が上限超過", func(e *JournalEntry) {
			e.Lines[0].Amount = MaxAmount + 1
			e.Lines[1].Amount = MaxAmount + 1
			e.Lines[1].TaxAmount = 0
		}},
		{"非課税明細に内消費税", func(e *JournalEntry) {
			e.Lines[1].TaxCategory = LineTaxNone
			e.Lines[1].TaxAmount = 100
		}},
		{"adjustment入力元なのにフラグ不整合", func(e *JournalEntry) {
			e.Source = SourceAdjustment
			e.IsAdjustment = false
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validEntry()
			tt.mutate(&e)
			err := e.Validate()
			if err == nil {
				t.Fatal("エラーになるべき")
			}
			if apperrors.CodeOf(err) != apperrors.CodeBadRequest {
				t.Errorf("CodeBadRequest であるべき: %v", err)
			}
		})
	}
}

func TestJournalEntryTotals(t *testing.T) {
	e := JournalEntry{
		Lines: []JournalLine{
			{Side: SideDebit, AccountCode: "5140", Amount: 8_000},
			{Side: SideDebit, AccountCode: "5190", Amount: 2_000},
			{Side: SideCredit, AccountCode: "1002", Amount: 10_000},
		},
	}
	if got := e.TotalDebit(); got != 10_000 {
		t.Errorf("TotalDebit = %d", got)
	}
	if got := e.TotalCredit(); got != 10_000 {
		t.Errorf("TotalCredit = %d", got)
	}
}

// ContentHash の値は DB の content_hash 列 (一意制約) に保存済みのため、
// 正規化形式やハッシュ計算を変更すると既存データの重複検出が壊れる。
// この golden 値の変化は互換性破壊の合図。
func TestContentHashGolden(t *testing.T) {
	d, _ := ParseDate("2025-04-01")
	e := JournalEntry{
		FiscalYear: 2025, Date: d, Counterparty: "取引先A",
		Lines: []JournalLine{
			{Side: SideDebit, AccountCode: "1002", Amount: 110_000},
			{Side: SideCredit, AccountCode: "4001", Amount: 110_000, TaxCategory: LineTaxTaxable10, TaxAmount: 10_000},
		},
	}
	const want = "7d2cded217c9c7c30a27372e8899cf5f428b5e7d01ef2b852bcc802c3018af4a"
	if got := e.ContentHash(); got != want {
		t.Errorf("ContentHash = %s, want %s (形式変更は保存済みデータとの互換性を壊す)", got, want)
	}
}

func TestContentHashLineOrderIndependent(t *testing.T) {
	e1 := validEntry()
	e2 := validEntry()
	// 明細順序を入れ替えてもハッシュは同一
	e2.Lines[0], e2.Lines[1] = e2.Lines[1], e2.Lines[0]
	if e1.ContentHash() != e2.ContentHash() {
		t.Error("明細順序が違うだけの仕訳はハッシュ一致すべき")
	}
}

func TestContentHashIgnoresDescription(t *testing.T) {
	e1 := validEntry()
	e2 := validEntry()
	e2.Description = "別の摘要"
	if e1.ContentHash() != e2.ContentHash() {
		t.Error("摘要のみ異なる仕訳はハッシュ一致すべき (重複検出のため)")
	}
}

// 取引先が異なる仕訳は正当な別取引であり得るため、ハッシュは区別する。
func TestContentHashDiffersByCounterparty(t *testing.T) {
	e1 := validEntry()
	e1.Counterparty = "取引先A"
	e2 := validEntry()
	e2.Counterparty = "取引先B"
	if e1.ContentHash() == e2.ContentHash() {
		t.Error("取引先が異なる仕訳はハッシュが異なるべき")
	}
}

// 税区分が異なる明細は別取引であり得るため、ハッシュは区別する
// (完全一致ブロックの誤爆防止。真の重複は「類似」警告側で拾う)。
func TestContentHashDiffersByTaxCategory(t *testing.T) {
	e1 := validEntry()
	e2 := validEntry()
	e2.Lines[1].TaxCategory = LineTaxTaxable8
	if e1.ContentHash() == e2.ContentHash() {
		t.Error("税区分が異なる仕訳はハッシュが異なるべき")
	}
}

func TestContentHashDiffersByContent(t *testing.T) {
	e1 := validEntry()

	e2 := validEntry()
	e2.Lines[0].Amount = 220_000
	e2.Lines[1].Amount = 220_000
	if e1.ContentHash() == e2.ContentHash() {
		t.Error("金額が異なる仕訳はハッシュが異なるべき")
	}

	e3 := validEntry()
	e3.Date, _ = ParseDate("2025-04-02")
	if e1.ContentHash() == e3.ContentHash() {
		t.Error("日付が異なる仕訳はハッシュが異なるべき")
	}
}
