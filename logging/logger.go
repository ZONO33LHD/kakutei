// Package logging は slog ベースのログ基盤 (横断的関心事として全層から利用可)。
//
// 環境変数でレベル・フォーマットを切り替え、context のリクエストIDを
// 全ログレコードへ自動付与するハンドラを提供する。
// エラーは Err で構造化し、apperrors のコードと failure のコールスタック起点を残す。
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/morikuni/failure"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

// Config はログ基盤の設定。
type Config struct {
	Level  string // debug / info / warn / error (既定: info)
	Format string // json / text (既定: json)
	Output io.Writer
}

// FromEnv は環境変数 (KAKUTEI_LOG_LEVEL / KAKUTEI_LOG_FORMAT) から設定を読む。
func FromEnv() Config {
	return Config{
		Level:  os.Getenv("KAKUTEI_LOG_LEVEL"),
		Format: os.Getenv("KAKUTEI_LOG_FORMAT"),
	}
}

// New は設定に基づいて *slog.Logger を構築する。
// 不正なレベル・フォーマット指定は既定値に落とす (起動を止めるほどではない)。
func New(cfg Config) *slog.Logger {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}

	var h slog.Handler
	if strings.EqualFold(cfg.Format, "text") {
		h = slog.NewTextHandler(out, opts)
	} else {
		h = slog.NewJSONHandler(out, opts)
	}
	return slog.New(&contextHandler{inner: h})
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type requestIDKey struct{}

// WithRequestID はリクエストIDを context に格納する。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFrom は context からリクエストIDを取り出す。未設定なら空文字。
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// contextHandler は context のリクエストIDを全レコードへ付与するハンドラ。
//
// 注意: WithGroup 派生後は request_id もそのグループ配下に入る。
// リクエスト相関が必要なログではグループ化したロガーを使わないこと。
type contextHandler struct {
	inner slog.Handler
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, rec slog.Record) error {
	if id := RequestIDFrom(ctx); id != "" {
		rec = rec.Clone() // slog.Handler の契約: Record を変更する前に複製する
		rec.AddAttrs(slog.String("request_id", id))
	}
	return h.inner.Handle(ctx, rec)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	if name == "" { // slog.Handler の契約: 空名はグループを作らない
		return h
	}
	return &contextHandler{inner: h.inner.WithGroup(name)}
}

// traceMaxBytes は trace 属性の上限。異常に深いチェーンでもログ1件が肥大しないようにする。
const traceMaxBytes = 8 * 1024

// traceEnabled は KAKUTEI_LOG_TRACE=off でトレース出力を無効化できる
// (trace には絶対パス・原因エラーの全メッセージが含まれるため、外部へログを
// 転送する運用では無効化を検討する)。キャッシュしないのはテスト容易性のため
// (エラーログ経路のみで呼ばれるため頻度も低い)。
func traceEnabled() bool {
	return !strings.EqualFold(os.Getenv("KAKUTEI_LOG_TRACE"), "off")
}

// Err はエラーを構造化したログ属性にする。1レコードで原因の特定に必要な情報が揃う:
// message は全文 (原因チェーン込み)、code は apperrors のコード、
// origin はエラーの発生地点 (最深部のコールスタックの先頭)、
// trace はラップ経路ごとのメッセージ・コード・ラップ地点と発生時の完全な
// コールスタック (failure の %+v 形式)。
func Err(err error) slog.Attr {
	if err == nil {
		// スキーマを他の分岐と揃える (集約基盤でのフィールド有無の揺れを防ぐ)
		return slog.Group("error",
			slog.String("message", "(nil)"), slog.String("code", ""))
	}
	attrs := []any{
		slog.String("message", err.Error()),
		slog.String("code", string(apperrors.CodeOf(err))),
	}
	if traceEnabled() {
		trace := fmt.Sprintf("%+v", err)
		if len(trace) > traceMaxBytes {
			cut := traceMaxBytes
			for cut > 0 && !utf8.RuneStart(trace[cut]) {
				cut-- // マルチバイト文字の途中で切らない
			}
			trace = trace[:cut] + "\n... (truncated)"
		}
		attrs = append(attrs, slog.String("trace", trace))
	}
	if cs, ok := failure.CallStackOf(err); ok {
		// 先頭フレームは apperrors ラッパー自身になるため、その外側の呼び出し元を探す
		for _, f := range cs.Frames() {
			if strings.HasSuffix(f.PkgPath(), "domain/apperrors") {
				continue
			}
			attrs = append(attrs, slog.String("origin",
				f.File()+":"+strconv.Itoa(f.Line())+" "+f.Func()))
			break
		}
	}
	return slog.Group("error", attrs...)
}
