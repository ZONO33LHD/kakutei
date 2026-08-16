// Package persistence は domain/repository の契約を SQLite で実装する。
package persistence

import (
	"context"
	"database/sql"

	_ "embed"
	"fmt"
	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

//go:embed schema.sql
var schemaSQL string

// migrations はスキーマバージョン順の適用 SQL。
// バージョン N の SQL は migrations[N-1]。将来のスキーマ変更は
// このスライスへの追記のみで行う (適用済みバージョンはスキップされる)。
var migrations = []string{
	schemaSQL, // v1: 初期スキーマ
}

// Migrate はスキーマを現行バージョンまで適用する (冪等)。
// 各バージョンはトランザクション内で適用し、PRAGMA user_version で管理する。
func Migrate(ctx context.Context, db *sql.DB) error {
	var current int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, "スキーマバージョンの取得に失敗しました")
	}
	for v := current; v < len(migrations); v++ {
		if err := applyMigration(ctx, db, v+1, migrations[v]); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, version int, ddl string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, fmt.Sprintf("マイグレーション v%d の開始に失敗しました", version))
	}
	if _, err := tx.ExecContext(ctx, ddl); err != nil {
		_ = tx.Rollback()
		return apperrors.Wrap(err, apperrors.CodeInternal, fmt.Sprintf("マイグレーション v%d の適用に失敗しました", version))
	}
	// PRAGMA はプレースホルダを使えないため整数を直接埋め込む (外部入力ではない)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		_ = tx.Rollback()
		return apperrors.Wrap(err, apperrors.CodeInternal, fmt.Sprintf("スキーマバージョン v%d の記録に失敗しました", version))
	}
	if err := tx.Commit(); err != nil {
		return apperrors.Wrap(err, apperrors.CodeInternal, fmt.Sprintf("マイグレーション v%d のコミットに失敗しました", version))
	}
	return nil
}
