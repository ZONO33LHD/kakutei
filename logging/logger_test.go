package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
)

func TestNewJSONWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Level: "info", Format: "json", Output: &buf})

	ctx := WithRequestID(context.Background(), "req-123")
	logger.InfoContext(ctx, "hello", "key", "value")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("JSON 出力ではない: %v (%s)", err, buf.String())
	}
	if rec["request_id"] != "req-123" {
		t.Errorf("request_id = %v", rec["request_id"])
	}
	if rec["key"] != "value" || rec["msg"] != "hello" {
		t.Errorf("属性が欠落: %v", rec)
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Level: "error", Output: &buf})
	logger.Info("捨てられる")
	if buf.Len() != 0 {
		t.Errorf("error レベル設定で info が出力された: %s", buf.String())
	}
	logger.Error("出力される")
	if buf.Len() == 0 {
		t.Error("error が出力されない")
	}
}

func TestTextFormatAndDefaults(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Format: "text", Level: "不正な値", Output: &buf})
	logger.Info("x")
	if strings.HasPrefix(strings.TrimSpace(buf.String()), "{") {
		t.Errorf("text 指定なのに JSON: %s", buf.String())
	}
}

// 派生ロガーを含む並行出力で行が壊れないこと (-race での検出用)。
func TestConcurrentLogging(t *testing.T) {
	var buf syncBuffer
	logger := New(Config{Output: &buf})
	derived := logger.With("component", "worker").WithGroup("g")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			logger.Info("base", "n", n)
			derived.Info("derived", "n", n)
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 40 {
		t.Fatalf("出力行数 = %d, want 40", len(lines))
	}
	for _, line := range lines {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("壊れた JSON 行: %q", line)
		}
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// WithGroup 派生後の request_id はグループ配下に入る (contextHandler の明文化した仕様)。
func TestRequestIDUnderGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Output: &buf})
	ctx := WithRequestID(context.Background(), "req-g")

	logger.InfoContext(ctx, "top")
	var top map[string]any
	if err := json.Unmarshal([]byte(strings.SplitN(buf.String(), "\n", 2)[0]), &top); err != nil {
		t.Fatal(err)
	}
	if top["request_id"] != "req-g" {
		t.Errorf("トップレベルに request_id がない: %v", top)
	}

	buf.Reset()
	logger.WithGroup("g").InfoContext(ctx, "grouped", "k", "v")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	group, _ := rec["g"].(map[string]any)
	if group == nil || group["request_id"] != "req-g" {
		t.Errorf("グループ配下に request_id が入る仕様: %v", rec)
	}
}

func TestErrAttr(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Output: &buf})
	err := apperrors.New(apperrors.CodeConflict, "重複しています")
	logger.Error("失敗", Err(err))

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("JSON 出力ではない: %v", err)
	}
	group, _ := rec["error"].(map[string]any)
	if group == nil {
		t.Fatalf("error グループがない: %v", rec)
	}
	if group["code"] != "CONFLICT" {
		t.Errorf("code = %v", group["code"])
	}
	if msg, _ := group["message"].(string); !strings.Contains(msg, "重複しています") {
		t.Errorf("message = %v", group["message"])
	}
	if origin, _ := group["origin"].(string); !strings.Contains(origin, "logger_test.go") {
		t.Errorf("origin にエラー発生地点がない: %v", group["origin"])
	}
	trace, _ := group["trace"].(string)
	if !strings.Contains(trace, "[CallStack]") || !strings.Contains(trace, "重複しています") {
		t.Errorf("trace に完全なトレースがない: %v", trace)
	}
}

// KAKUTEI_LOG_TRACE=off で trace 属性が出力されない。
func TestErrAttrTraceDisabled(t *testing.T) {
	t.Setenv("KAKUTEI_LOG_TRACE", "off")
	var buf bytes.Buffer
	logger := New(Config{Output: &buf})
	logger.Error("失敗", Err(apperrors.New(apperrors.CodeConflict, "x")))

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	group, _ := rec["error"].(map[string]any)
	if _, ok := group["trace"]; ok {
		t.Error("off 設定時は trace が出力されないべき")
	}
	if group["code"] != "CONFLICT" || group["origin"] == nil {
		t.Errorf("trace 以外の属性は維持されるべき: %v", group)
	}
}

// ラップを重ねたエラーの trace には、各ラップ地点とメッセージが一連で含まれる。
func TestErrAttrWrappedChain(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Config{Output: &buf})
	inner := apperrors.New(apperrors.CodeConflict, "既に登録されています")
	outer := apperrors.Wrapf(inner, "2 件目")
	logger.Error("失敗", Err(outer))

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	group, _ := rec["error"].(map[string]any)
	trace, _ := group["trace"].(string)
	for _, want := range []string{
		"2 件目",
		"既に登録されています",
		"code(CONFLICT)",
		"[CallStack]",
	} {
		if !strings.Contains(trace, want) {
			t.Errorf("trace に %q がない:\n%s", want, trace)
		}
	}
	// 先頭行 (最も外側のラップ地点) は facade でなく呼び出し元 (このテスト) を指す
	firstLine := strings.SplitN(trace, "\n", 2)[0]
	if !strings.Contains(firstLine, "logger_test.go") || strings.Contains(firstLine, "errors.go") {
		t.Errorf("trace の先頭行がラップ元を指していない: %q", firstLine)
	}
}
