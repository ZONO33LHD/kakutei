package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	// PRAGMA が効いていること
	var fk int
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d, err=%v (want 1)", fk, err)
	}
	var mode string
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q, err=%v (want wal)", mode, err)
	}
}

func TestOpenInvalidPath(t *testing.T) {
	// 存在しないディレクトリ配下は Ping で失敗する
	if _, err := Open(context.Background(), "/nonexistent-dir-xyz/sub/test.db"); err == nil {
		t.Error("不正なパスはエラーになるべき")
	}
}
