package rest

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

// statusRecorder はレスポンスのステータスコードを記録する。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// AccessLog はリクエストのメソッド・パス・ステータス・所要時間を記録する。
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		slog.Info("access",
			"method", r.Method, "path", r.URL.Path,
			"status", rec.status, "duration", time.Since(start).String())
	})
}

// Recover は handler の panic を捕捉して 500 を返す。
// panic の詳細はログにのみ残す (クライアントへ内部情報を出さない)。
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("handler panic", "panic", rec, "path", r.URL.Path)
				writeError(w, apperrors.New(apperrors.CodeInternal, "サーバー内部エラーが発生しました"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Chain は middleware を左から順に適用する (左が最も外側)。
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// mutationMethods は状態変更を伴う HTTP メソッド。
var mutationMethods = map[string]bool{
	http.MethodPost: true, http.MethodPut: true,
	http.MethodPatch: true, http.MethodDelete: true,
}

// RequireJSONContentType は変更系リクエストに Content-Type: application/json を要求する。
//
// ブラウザの HTML フォームや no-cors fetch は application/json を送信できないため、
// 悪意ある Web ページからのクロスサイト書き込み (CSRF) を遮断できる。
// body を持たない DELETE 等は Content-Type 未指定を許容する。
func RequireJSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mutationMethods[r.Method] && r.ContentLength != 0 {
			ct := r.Header.Get("Content-Type")
			if ct != "" && !isJSONContentType(ct) {
				writeError(w, apperrors.New(apperrors.CodeBadRequest,
					"Content-Type は application/json を指定してください"))
				return
			}
			if ct == "" {
				writeError(w, apperrors.New(apperrors.CodeBadRequest,
					"変更系リクエストには Content-Type: application/json が必要です"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isJSONContentType(ct string) bool {
	// "application/json" または "application/json; charset=utf-8" を許容
	mediaType := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
	return strings.EqualFold(mediaType, "application/json")
}

// HostGuard は Host ヘッダをローカルホスト系に限定する (DNS rebinding 対策)。
// 追加で許可するホスト名は allowed で指定する。
func HostGuard(allowed ...string) func(http.Handler) http.Handler {
	permitted := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true, "[::1]": true}
	for _, h := range allowed {
		if h != "" {
			permitted[h] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			// ポート部を除去 (localhost:8080 → localhost、[::1]:8080 → ::1)
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			if !permitted[host] {
				writeError(w, apperrors.Newf(apperrors.CodeBadRequest, "許可されていない Host です: %q", r.Host))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
