// kakutei バックエンドサーバーのエントリポイント。
//
// 環境変数:
//
//	KAKUTEI_DB_PATH:    SQLite ファイルパス (既定: kakutei.db)
//	KAKUTEI_ADDR:       待受アドレス (既定: 127.0.0.1:8080)
//	KAKUTEI_LOG_LEVEL:  ログレベル debug/info/warn/error (既定: info)
//	KAKUTEI_LOG_FORMAT: ログ形式 json/text (既定: json)
//	KAKUTEI_LOG_TRACE:  エラーログの詳細トレース (off で無効化、既定: 有効)
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
		stop() // 終了処理中のシグナルを既定動作 (即終了) に戻してから片付ける
		return err
	case <-ctx.Done():
		// シグナル捕捉を解除し、2発目のシグナルでは即終了 (デフォルト動作) できるようにする
		stop()
		slog.Info("シャットダウンします")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			// 猶予内に完了しない等で Shutdown が失敗したら残存接続を強制切断する
			// (直後に defer が DB を閉じるため、使用中の接続を残さない)
			if cerr := server.Close(); cerr != nil {
				slog.Error("残存接続の強制切断に失敗しました", "error", cerr)
			}
			return err
		}
		return nil
	}
}
