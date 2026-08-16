package model

import (
	"encoding/json"
	"time"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

// AuditOperation は仕訳の訂正・削除操作の種別。
type AuditOperation string

const (
	AuditUpdate AuditOperation = "update"
	AuditDelete AuditOperation = "delete"
)

func (o AuditOperation) Validate() error {
	if o == AuditUpdate || o == AuditDelete {
		return nil
	}
	return apperrors.Newf(apperrors.CodeBadRequest, "不正な監査操作種別です: %q", string(o))
}

// JournalAuditLog は仕訳の訂正・削除履歴 (電子帳簿保存法施行規則5条5項1号イの
// 「訂正削除の事実及び内容を確認できる」要件に対応する)。
//
// Before/After は操作時点の仕訳全体の JSON スナップショット。
// 削除の場合 AfterSnapshot は空。
type JournalAuditLog struct {
	ID             int64
	JournalID      int64
	FiscalYear     FiscalYear
	Operation      AuditOperation
	BeforeSnapshot string
	AfterSnapshot  string
	CreatedAt      time.Time
}

func (l *JournalAuditLog) Validate() error {
	if l.JournalID <= 0 {
		return apperrors.New(apperrors.CodeBadRequest, "監査ログの仕訳IDが不正です")
	}
	if err := l.FiscalYear.Validate(); err != nil {
		return err
	}
	if err := l.Operation.Validate(); err != nil {
		return err
	}
	if l.BeforeSnapshot == "" || !json.Valid([]byte(l.BeforeSnapshot)) {
		return apperrors.New(apperrors.CodeBadRequest, "変更前スナップショットは有効な JSON である必要があります")
	}
	if l.Operation == AuditUpdate && (l.AfterSnapshot == "" || !json.Valid([]byte(l.AfterSnapshot))) {
		return apperrors.New(apperrors.CodeBadRequest, "訂正の監査ログには有効な JSON の変更後スナップショットが必須です")
	}
	return nil
}
