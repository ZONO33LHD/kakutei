// kakutei バックエンドサーバーのエントリポイント。
//
// 環境変数:
//
//	KAKUTEI_DB_PATH:    SQLite ファイルパス (既定: kakutei.db)
//	KAKUTEI_ADDR:       待受アドレス (既定: 127.0.0.1:8080)
//	KAKUTEI_LOG_LEVEL:  ログレベル debug/info/warn/error (既定: info)
//	KAKUTEI_LOG_FORMAT: ログ形式 json/text (既定: json)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ZONO33LHD/kakutei/logging"
	"github.com/ZONO33LHD/kakutei/registry"
)

func main() {
	if err := run(); err != nil {
		slog.Error("サーバーが異常終了しました", "error", err)
		os.Exit(1)
	}
}

func run() error {
	slog.SetDefault(logging.New(logging.FromEnv()))

	cfg := registry.DefaultConfig()
	if v := os.Getenv("KAKUTEI_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("KAKUTEI_ADDR"); v != "" {
		cfg.Addr = v
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg, err := registry.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := reg.Close(); err != nil {
			slog.Error("リソースの解放に失敗しました", "error", err)
		}
	}()

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           reg.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second, // 低速 body 送信による接続占有の防止
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("サーバーを起動しました", "addr", cfg.Addr, "db", cfg.DBPath)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("シャットダウンします")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
