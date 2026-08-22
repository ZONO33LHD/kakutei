package rest

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/logging"
)

// captureLogs は slog のデフォルトロガーを一時的に差し替えてログを捕捉する。
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(logging.New(logging.Config{Output: &buf}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// X-Request-Id ヘッダとアクセスログの request_id が一致すること (相関の保証)。
func TestRequestIDCorrelatesWithAccessLog(t *testing.T) {
	buf := captureLogs(t)

	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, map[string]string{"Status": "ok"})
	}), RequestID, AccessLog)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	headerID := rec.Header().Get("X-Request-Id")
	if headerID == "" {
		t.Fatal("X-Request-Id がない")
	}
	var logRec map[string]any
	if err := json.Unmarshal([]byte(strings.SplitN(buf.String(), "\n", 2)[0]), &logRec); err != nil {
		t.Fatalf("アクセスログが JSON でない: %v (%s)", err, buf.String())
	}
	if logRec["request_id"] != headerID {
		t.Errorf("ヘッダ %q とログ %v の request_id が不一致", headerID, logRec["request_id"])
	}
}

// 許可リストにないコードは詳細を返さず 500 に落とす (fail closed)。
func TestWriteErrorUnknownCodeFailsClosed(t *testing.T) {
	buf := captureLogs(t)

	rec := httptest.NewRecorder()
	err := apperrors.New(apperrors.Code("UNKNOWN_CODE"), "内部の詳細情報")
	writeError(rec, httptest.NewRequest("GET", "/x", nil), err)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != apperrors.CodeInternal || strings.Contains(body.Error.Message, "内部の詳細情報") {
		t.Errorf("詳細が漏えいしている: %+v", body.Error)
	}
	if !strings.Contains(buf.String(), "UNKNOWN_CODE") {
		t.Error("詳細はログに残るべき")
	}
}

// panic は stack 付きで1回だけログされ、クライアントには一般化した 500 を返す。
func TestRecoverLogsStackOnce(t *testing.T) {
	buf := captureLogs(t)

	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), RequestID, Recover)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != apperrors.CodeInternal || strings.Contains(body.Error.Message, "boom") {
		t.Errorf("panic 内容が漏えいしている: %+v", body.Error)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Errorf("エラーログは1回だけであるべき: %d 行", len(lines))
	}
	if !strings.Contains(lines[0], "middleware_internal_test.go") {
		t.Error("ログに stack (panic 発生地点) が含まれるべき")
	}
}

// HostGuard の境界ケース (CSRF/DNS rebinding 対策の要のため網羅的に固定する)。
func TestHostGuard(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	guard := HostGuard("myhost.local")(okHandler)

	tests := []struct {
		host string
		want int
	}{
		{"localhost", http.StatusOK},
		{"localhost:8080", http.StatusOK},
		{"127.0.0.1", http.StatusOK},
		{"127.0.0.1:8080", http.StatusOK},
		{"[::1]", http.StatusOK},
		{"[::1]:8080", http.StatusOK},
		{"myhost.local", http.StatusOK},             // 追加許可ホスト
		{"myhost.local:8080", http.StatusOK},        // 追加許可ホスト (ポート付き)
		{"LOCALHOST", http.StatusOK},                // DNS 名は大文字小文字を区別しない
		{"evil.example.com", http.StatusBadRequest}, // DNS rebinding
		{"evil.example.com:8080", http.StatusBadRequest},
		{"127.0.0.1.evil.com", http.StatusBadRequest}, // 前方一致もどき
		{"localhost.evil.com", http.StatusBadRequest},
		{"", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run("host="+tt.host, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/x", nil)
			req.Host = tt.host
			rec := httptest.NewRecorder()
			guard.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("Host %q = %d, want %d", tt.host, rec.Code, tt.want)
			}
		})
	}
}

// RequireJSONContentType の境界ケース (simple content-type による CSRF 遮断)。
func TestRequireJSONContentType(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	guard := RequireJSONContentType(okHandler)

	tests := []struct {
		name   string
		method string
		ct     string
		body   string
		want   int
	}{
		{"JSON は許可", "POST", "application/json", "{}", http.StatusOK},
		{"charset 付きも許可", "POST", "application/json; charset=utf-8", "{}", http.StatusOK},
		{"大文字小文字は不問", "POST", "Application/JSON", "{}", http.StatusOK},
		{"text/plain は拒否", "POST", "text/plain", "{}", http.StatusBadRequest},
		{"form は拒否", "POST", "application/x-www-form-urlencoded", "a=1", http.StatusBadRequest},
		{"multipart は拒否", "PUT", "multipart/form-data", "x", http.StatusBadRequest},
		{"Content-Type なしは拒否", "DELETE", "", "{}", http.StatusBadRequest},
		{"GET は検査対象外", "GET", "text/plain", "x", http.StatusOK},
		{"body なしの POST は許容", "POST", "", "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body == "" {
				req = httptest.NewRequest(tt.method, "/x", nil)
			} else {
				req = httptest.NewRequest(tt.method, "/x", strings.NewReader(tt.body))
			}
			if tt.ct != "" {
				req.Header.Set("Content-Type", tt.ct)
			}
			rec := httptest.NewRecorder()
			guard.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("= %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
