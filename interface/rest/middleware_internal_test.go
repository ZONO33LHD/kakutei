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
