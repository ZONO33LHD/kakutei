package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

// EntrySide は仕訳明細の借方/貸方。
type EntrySide string

const (
	SideDebit  EntrySide = "debit"  // 借方
	SideCredit EntrySide = "credit" // 貸方
)

// Validate は定義済みの側かを検証する。
func (s EntrySide) Validate() error {
	if s == SideDebit || s == SideCredit {
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "side は debit / credit のいずれかです: %q", string(s))
}

// LineTaxCategory は仕訳明細単位の消費税区分 (税率を含む)。
type LineTaxCategory string

const (
	LineTaxTaxable10  LineTaxCategory = "taxable_10" // 課税 標準税率10%
	LineTaxTaxable8   LineTaxCategory = "taxable_8"  // 課税 軽減税率8%
	LineTaxNone       LineTaxCategory = "non_taxable"
	LineTaxExempt     LineTaxCategory = "exempt"
	LineTaxOutOfScope LineTaxCategory = "out_of_scope"
)

// Validate は定義済みの区分かを検証する。
func (c LineTaxCategory) Validate() error {
	switch c {
	case LineTaxTaxable10, LineTaxTaxable8, LineTaxNone, LineTaxExempt, LineTaxOutOfScope:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な明細消費税区分です: %q", string(c))
}

// JournalSource は仕訳の入力元。
type JournalSource string

const (
	SourceManual     JournalSource = "manual"     // 手入力
	SourceCSVImport  JournalSource = "csv_import" // CSV取り込み
	SourceOCR        JournalSource = "ocr"        // 領収書・請求書等の読み取り
	SourceAdjustment JournalSource = "adjustment" // 決算整理仕訳
)

// Validate は定義済みの入力元かを検証する。
func (s JournalSource) Validate() error {
	switch s {
	case SourceManual, SourceCSVImport, SourceOCR, SourceAdjustment:
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な仕訳入力元です: %q", string(s))
}

// JournalLine は仕訳明細 (借方または貸方の1行)。
type JournalLine struct {
	ID          int64
	Side        EntrySide
	AccountCode AccountCode
	Amount      Money           // 正の円額
	TaxCategory LineTaxCategory // 空 = 区分未設定
	TaxAmount   Money           // 内消費税額 (参考値)
}

// Validate は明細単体の検証を行う。
func (l *JournalLine) Validate() error {
	if err := l.Side.Validate(); err != nil {
		return err
	}
	if err := l.AccountCode.Validate(); err != nil {
		return err
	}
	if l.Amount <= 0 {
		return apperrors.Newf(apperrors.CodeBadRequest, "明細金額は正の整数 (円) です: %d", l.Amount.Yen())
	}
	if !l.Amount.ValidateAmountRange() {
		return apperrors.Newf(apperrors.CodeBadRequest,
			"明細金額が上限 (%d 円) を超えています: %d", MaxAmount.Yen(), l.Amount.Yen())
	}
	if l.TaxCategory != "" {
		if err := l.TaxCategory.Validate(); err != nil {
			return err
		}
	}
	if l.TaxAmount < 0 || l.TaxAmount > l.Amount {
		return apperrors.Newf(apperrors.CodeBadRequest,
			"内消費税額は 0 以上かつ明細金額以下です: %d", l.TaxAmount.Yen())
	}
	if l.TaxAmount > 0 && l.TaxCategory != LineTaxTaxable10 && l.TaxCategory != LineTaxTaxable8 {
		return apperrors.New(apperrors.CodeBadRequest,
			"内消費税額を設定できるのは課税区分 (taxable_10 / taxable_8) の明細のみです")
	}
	return nil
}

// JournalEntry は複式簿記の仕訳1件 (ヘッダ + 明細)。
type JournalEntry struct {
	ID           int64
	FiscalYear   FiscalYear
	Date         Date
	Description  string // 摘要。空 = 未設定
	Counterparty string // 取引先。空 = 未設定
	Lines        []JournalLine
	Source       JournalSource
	SourceFile   string // 取り込み元ファイル。空 = 未設定
	IsAdjustment bool   // 決算整理仕訳かどうか
}

const (
	// maxJournalLines は 1 仕訳あたりの明細数の上限 (異常入力の遮断)。
	maxJournalLines = 100
	// maxDescriptionLen は摘要・取引先の文字数上限 (rune 数)。
	maxDescriptionLen = 500
)

// Validate は仕訳の自己検証を行う。
//
//   - 日付が課税年度内であること
//   - 明細が 2 行以上 maxJournalLines 以下であること
//   - 借方合計 = 貸方合計 (貸借一致の原則)
//   - 各明細が単体検証を通ること
func (e *JournalEntry) Validate() error {
	if err := e.FiscalYear.Validate(); err != nil {
		return err
	}
	if e.Date.IsZero() {
		return apperrors.New(apperrors.CodeBadRequest, "仕訳日付は必須です")
	}
	if !e.FiscalYear.Contains(e.Date) {
		return apperrors.Newf(apperrors.CodeBadRequest,
			"仕訳日付 %s が課税年度 %d の期間外です", e.Date, int(e.FiscalYear))
	}
	if len([]rune(e.Description)) > maxDescriptionLen {
		return apperrors.Newf(apperrors.CodeBadRequest, "摘要は %d 文字以内にしてください", maxDescriptionLen)
	}
	if len([]rune(e.Counterparty)) > maxDescriptionLen {
		return apperrors.Newf(apperrors.CodeBadRequest, "取引先は %d 文字以内にしてください", maxDescriptionLen)
	}
	if e.Source != "" {
		if err := e.Source.Validate(); err != nil {
			return err
		}
	}
	// Source と IsAdjustment の二重管理の矛盾を防ぐ:
	// 入力元が決算整理 (adjustment) なのにフラグが立っていない状態は不正。
	if e.Source == SourceAdjustment && !e.IsAdjustment {
		return apperrors.New(apperrors.CodeBadRequest,
			"source が adjustment の仕訳は is_adjustment を true にしてください")
	}
	if len(e.Lines) < 2 {
		return apperrors.New(apperrors.CodeBadRequest, "仕訳明細は借方・貸方あわせて 2 行以上必要です")
	}
	if len(e.Lines) > maxJournalLines {
		return apperrors.Newf(apperrors.CodeBadRequest, "仕訳明細は %d 行以内にしてください", maxJournalLines)
	}
	for i := range e.Lines {
		if err := e.Lines[i].Validate(); err != nil {
			return apperrors.Wrap(err, apperrors.CodeBadRequest, fmt.Sprintf("明細 %d 行目が不正です", i+1))
		}
	}
	debit, credit := e.TotalDebit(), e.TotalCredit()
	if debit != credit {
		return apperrors.Newf(apperrors.CodeBadRequest,
			"貸借が一致しません: 借方合計 %d 円, 貸方合計 %d 円", debit.Yen(), credit.Yen())
	}
	return nil
}

// TotalDebit は借方合計を返す。
func (e *JournalEntry) TotalDebit() Money {
	return e.totalBySide(SideDebit)
}

// TotalCredit は貸方合計を返す。
func (e *JournalEntry) TotalCredit() Money {
	return e.totalBySide(SideCredit)
}

func (e *JournalEntry) totalBySide(side EntrySide) Money {
	var total Money
	for i := range e.Lines {
		if e.Lines[i].Side == side {
			total += e.Lines[i].Amount
		}
	}
	return total
}

// ContentHash は重複検出用の内容ハッシュ (SHA-256) を返す。
//
// 日付 + 正規化 (ソート) した明細 (side, 科目, 金額, 税区分, 内税額) から計算する。
// 摘要・取引先は意図的に含めない — 同一取引が異なる摘要で二重登録されても
// 検出できるようにするため。税区分は含める — 同日・同科目・同額でも税区分が
// 異なる明細は別取引 (例: 軽減税率の食料品と標準税率の消耗品) であり得るため、
// 完全一致ブロックの誤爆を避ける。税区分だけ異なる真の重複は「類似」警告側で拾う。
func (e *JournalEntry) ContentHash() string {
	type key struct {
		side      EntrySide
		code      AccountCode
		amount    Money
		taxCat    LineTaxCategory
		taxAmount Money
	}
	keys := make([]key, 0, len(e.Lines))
	for i := range e.Lines {
		l := &e.Lines[i]
		keys = append(keys, key{l.Side, l.AccountCode, l.Amount, l.TaxCategory, l.TaxAmount})
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.side != b.side {
			return a.side < b.side
		}
		if a.code != b.code {
			return a.code < b.code
		}
		if a.amount != b.amount {
			return a.amount < b.amount
		}
		if a.taxCat != b.taxCat {
			return a.taxCat < b.taxCat
		}
		return a.taxAmount < b.taxAmount
	})
	var b strings.Builder
	b.WriteString(e.Date.String())
	for _, k := range keys {
		fmt.Fprintf(&b, "|%s:%s:%d:%s:%d", k.side, k.code, k.amount.Yen(), k.taxCat, k.taxAmount.Yen())
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
