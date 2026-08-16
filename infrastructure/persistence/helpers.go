package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

// isUniqueViolation は SQLite の一意制約違反かどうかを判定する。
// modernc.org/sqlite のエラーは文字列にコード (2067 = SQLITE_CONSTRAINT_UNIQUE,
// 1555 = SQLITE_CONSTRAINT_PRIMARYKEY) を含む。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}

// wrapInternal は予期しない DB エラーを CodeInternal でラップする。
func wrapInternal(err error, operation string) error {
	return apperrors.Wrap(err, apperrors.CodeInternal, operation+"に失敗しました")
}

// notFound は対象が存在しないエラーを返す。
func notFound(what string) error {
	return apperrors.Newf(apperrors.CodeNotFound, "%sが見つかりません", what)
}

// inTx はトランザクション内で fn を実行する。fn がエラーを返すとロールバックする。
func inTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return wrapInternal(err, "トランザクション開始")
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return fmt.Errorf("%w (rollback も失敗: %v)", err, rbErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return wrapInternal(err, "コミット")
	}
	return nil
}
