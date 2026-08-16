// Package db は SQLite データベースの接続とマイグレーションを提供する。
//
// ドライバは modernc.org/sqlite (pure Go、CGO 不要) を使う。
// 確定申告データは単一ユーザーのローカルデータであり、
// 組み込み DB の SQLite が運用・バックアップの観点で適している。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"

	_ "modernc.org/sqlite" // database/sql ドライバ登録
)

// escapeURIPath は SQLite URI のパス部分に現れると誤解釈される文字を
// パーセントエンコードする (%, ?, # の順序が重要: % を最初に置換する)。
var escapeURIPath = strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23")

// Open は SQLite データベースを開き、整合性に必要な PRAGMA を設定する。
//
//   - foreign_keys=ON: 外部キー制約の有効化 (SQLite は既定で無効)
//   - journal_mode=WAL: 読み書きの並行性向上
//   - busy_timeout: ロック競合時の待機
//   - _txlock=immediate: トランザクション開始時に書き込みロックを取得し、
//     read→write 昇格時の SQLITE_BUSY_SNAPSHOT を回避する
//
// path はファイルパス、または ":memory:" (テスト用)。
func Open(ctx context.Context, path string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_txlock=immediate&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
		escapeURIPath.Replace(path))
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "SQLite のオープンに失敗しました")
	}
	// modernc.org/sqlite は単一コネクションでの利用が安全
	// (複数コネクションだと in-memory DB が分裂し、ファイル DB でもロック競合が増える)。
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, apperrors.Wrap(err, apperrors.CodeInternal, "SQLite への接続確認に失敗しました")
	}
	return sqlDB, nil
}
