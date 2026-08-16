package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ZONO33LHD/kakutei/registry"
)

// newTestServer は実 SQLite + 全レイヤーを組み立てたテストサーバーを返す。
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	reg, err := registry.New(context.Background(), registry.Config{
		DBPath: filepath.Join(t.TempDir(), "rest.db"),
	})
	if err != nil {
		t.Fatalf("registry.New: %v", err)
	}
	t.Cleanup(func() { _ = reg.Close() })
	server := httptest.NewServer(reg.Router())
	t.Cleanup(server.Close)
	return server
}

// doJSON はリクエストを送り、ステータスとデコード済み body を返す。
func doJSON(t *testing.T, server *httptest.Server, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var decoded map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&decoded)
	return resp.StatusCode, decoded
}

// setupYear は年度 2025 を作成する。
func setupYear(t *testing.T, server *httptest.Server) {
	t.Helper()
	status, body := doJSON(t, server, "POST", "/api/fiscal-years", map[string]any{"Year": 2025})
	if status != http.StatusCreated {
		t.Fatalf("年度作成: status=%d body=%v", status, body)
	}
}

// salesJournal は売上仕訳のリクエスト body を作る。
func salesJournal(date string, amount int64) map[string]any {
	return map[string]any{
		"Entry": map[string]any{
			"FiscalYear":  2025,
			"Date":        date,
			"Description": "売上",
			"Lines": []map[string]any{
				{"Side": "debit", "AccountCode": "1002", "Amount": amount},
				{"Side": "credit", "AccountCode": "4001", "Amount": amount, "TaxCategory": "taxable_10"},
			},
			"Source": "manual",
		},
	}
}

func TestHealthAndFiscalYearFlow(t *testing.T) {
	server := newTestServer(t)

	status, body := doJSON(t, server, "GET", "/health", nil)
	if status != http.StatusOK || body["Status"] != "ok" {
		t.Fatalf("health: %d %v", status, body)
	}

	setupYear(t, server)

	// 重複作成は 409
	status, _ = doJSON(t, server, "POST", "/api/fiscal-years", map[string]any{"Year": 2025})
	if status != http.StatusConflict {
		t.Errorf("重複年度 = %d, want 409", status)
	}

	// 科目マスタが投入されている
	status, body = doJSON(t, server, "GET", "/api/accounts", nil)
	accounts, _ := body["Accounts"].([]any)
	if status != http.StatusOK || len(accounts) != 72 {
		t.Errorf("accounts: %d 件 (status %d)", len(accounts), status)
	}

	// 締め → 再開
	if status, _ = doJSON(t, server, "POST", "/api/fiscal-years/2025/close", nil); status != http.StatusOK {
		t.Errorf("close = %d", status)
	}
	if status, _ = doJSON(t, server, "POST", "/api/fiscal-years/2025/reopen", nil); status != http.StatusOK {
		t.Errorf("reopen = %d", status)
	}
}

func TestJournalEndpoints(t *testing.T) {
	server := newTestServer(t)
	setupYear(t, server)

	// 登録
	status, body := doJSON(t, server, "POST", "/api/journals", salesJournal("2025-04-01", 110000))
	if status != http.StatusCreated {
		t.Fatalf("仕訳登録: %d %v", status, body)
	}
	id := int64(body["ID"].(float64))

	// 完全一致は 409
	status, _ = doJSON(t, server, "POST", "/api/journals", salesJournal("2025-04-01", 110000))
	if status != http.StatusConflict {
		t.Errorf("重複仕訳 = %d, want 409", status)
	}

	// 取得
	status, body = doJSON(t, server, "GET", fmt.Sprintf("/api/journals/%d", id), nil)
	if status != http.StatusOK || body["Date"] != "2025-04-01" {
		t.Errorf("取得: %d %v", status, body)
	}

	// 検索
	status, body = doJSON(t, server, "GET", "/api/journals?year=2025&account_code=4001", nil)
	if status != http.StatusOK || body["TotalCount"].(float64) != 1 {
		t.Errorf("検索: %d %v", status, body)
	}

	// 訂正 → 監査ログ
	update := salesJournal("2025-04-02", 220000)["Entry"].(map[string]any)
	status, _ = doJSON(t, server, "PUT", fmt.Sprintf("/api/journals/%d", id), update)
	if status != http.StatusOK {
		t.Errorf("訂正 = %d", status)
	}
	status, body = doJSON(t, server, "GET", fmt.Sprintf("/api/journals/%d/audit-logs", id), nil)
	logs, _ := body["AuditLogs"].([]any)
	if status != http.StatusOK || len(logs) != 1 {
		t.Errorf("監査ログ: %d %v", status, body)
	}

	// 削除 → 404
	status, _ = doJSON(t, server, "DELETE", fmt.Sprintf("/api/journals/%d", id), nil)
	if status != http.StatusNoContent {
		t.Errorf("削除 = %d", status)
	}
	status, _ = doJSON(t, server, "GET", fmt.Sprintf("/api/journals/%d", id), nil)
	if status != http.StatusNotFound {
		t.Errorf("削除後の取得 = %d, want 404", status)
	}

	// 不正な body は 400
	status, _ = doJSON(t, server, "POST", "/api/journals", map[string]any{"Unknown": 1})
	if status != http.StatusBadRequest {
		t.Errorf("未知フィールド = %d, want 400", status)
	}
}

func TestReportAndFilingEndpoints(t *testing.T) {
	server := newTestServer(t)
	setupYear(t, server)

	if status, body := doJSON(t, server, "POST", "/api/journals", salesJournal("2025-02-01", 5500000)); status != http.StatusCreated {
		t.Fatalf("売上: %d %v", status, body)
	}

	// 財務諸表
	status, body := doJSON(t, server, "GET", "/api/reports/profit-and-loss?year=2025", nil)
	if status != http.StatusOK || body["TotalRevenue"].(float64) != 5500000 {
		t.Errorf("PL: %d %v", status, body)
	}
	if status, _ := doJSON(t, server, "GET", "/api/reports/trial-balance?year=2025", nil); status != http.StatusOK {
		t.Errorf("試算表 = %d", status)
	}
	if status, _ := doJSON(t, server, "GET", "/api/reports/balance-sheet?year=2025", nil); status != http.StatusOK {
		t.Errorf("BS = %d", status)
	}
	if status, _ := doJSON(t, server, "GET", "/api/reports/general-ledger?year=2025&account=1002", nil); status != http.StatusOK {
		t.Errorf("元帳 = %d", status)
	}

	// 申告資料 (汎用 CRUD)
	status, body = doJSON(t, server, "POST", "/api/materials/medical-expenses", map[string]any{
		"FiscalYear": 2025, "Date": "2025-05-01",
		"PatientName": "本人", "MedicalInstitution": "病院", "Amount": 300000,
	})
	if status != http.StatusCreated {
		t.Fatalf("医療費: %d %v", status, body)
	}
	status, body = doJSON(t, server, "GET", "/api/materials/medical-expenses?year=2025", nil)
	records, _ := body["Records"].([]any)
	if status != http.StatusOK || len(records) != 1 {
		t.Errorf("医療費一覧: %d %v", status, body)
	}

	// 配偶者 (年度1件)
	status, _ = doJSON(t, server, "PUT", "/api/materials/spouse", map[string]any{
		"FiscalYear": 2025, "Name": "花子", "BirthDate": "1990-05-01",
	})
	if status != http.StatusOK {
		t.Errorf("配偶者 = %d", status)
	}

	// 所得税計算
	status, body = doJSON(t, server, "POST", "/api/filing/income-tax?year=2025", map[string]any{
		"BlueReturnDeduction": 650000,
	})
	if status != http.StatusOK {
		t.Fatalf("所得税: %d %v", status, body)
	}
	// 事業所得 = 5,000,000 (税込売上を帳簿額のまま) − 65万 = 4,850,000
	if body["BusinessIncome"].(float64) != 4850000 {
		t.Errorf("BusinessIncome = %v", body["BusinessIncome"])
	}

	// 消費税計算 (2割特例)
	status, body = doJSON(t, server, "POST", "/api/filing/consumption-tax?year=2025", map[string]any{
		"Method": "special_20pct",
	})
	if status != http.StatusOK {
		t.Fatalf("消費税: %d %v", status, body)
	}
	result := body["Result"].(map[string]any)
	if result["TotalDue"].(float64) != 100000 {
		t.Errorf("消費税 TotalDue = %v, want 100000", result["TotalDue"])
	}

	// サニティチェック・ふるさと集計・減価償却の疎通
	if status, _ := doJSON(t, server, "POST", "/api/filing/sanity-check?year=2025", nil); status != http.StatusOK {
		t.Errorf("sanity-check = %d", status)
	}
	if status, _ := doJSON(t, server, "POST", "/api/filing/furusato-summary?year=2025", nil); status != http.StatusOK {
		t.Errorf("furusato-summary = %d", status)
	}
	if status, _ := doJSON(t, server, "GET", "/api/filing/depreciation?year=2025", nil); status != http.StatusOK {
		t.Errorf("depreciation = %d", status)
	}

	// 重複スキャン
	if status, _ := doJSON(t, server, "GET", "/api/journals/duplicates?year=2025", nil); status != http.StatusOK {
		t.Errorf("duplicates = %d", status)
	}
}

func TestMaterialCRUDAndBatch(t *testing.T) {
	server := newTestServer(t)
	setupYear(t, server)

	// 汎用 CRUD の Get / Update / Delete
	status, body := doJSON(t, server, "POST", "/api/materials/dependents", map[string]any{
		"FiscalYear": 2025, "Name": "子", "Relationship": "子",
		"BirthDate": "2010-01-01", "Cohabiting": true,
	})
	if status != http.StatusCreated {
		t.Fatalf("扶養親族: %d %v", status, body)
	}
	id := int64(body["ID"].(float64))

	status, body = doJSON(t, server, "GET", fmt.Sprintf("/api/materials/dependents/%d", id), nil)
	if status != http.StatusOK || body["Name"] != "子" {
		t.Errorf("Get: %d %v", status, body)
	}

	status, _ = doJSON(t, server, "PUT", fmt.Sprintf("/api/materials/dependents/%d", id), map[string]any{
		"FiscalYear": 2025, "Name": "子", "Relationship": "子",
		"BirthDate": "2010-01-01", "Cohabiting": false,
	})
	if status != http.StatusOK {
		t.Errorf("Update = %d", status)
	}

	status, _ = doJSON(t, server, "DELETE", fmt.Sprintf("/api/materials/dependents/%d", id), nil)
	if status != http.StatusNoContent {
		t.Errorf("Delete = %d", status)
	}

	// 配偶者の削除
	if status, _ := doJSON(t, server, "PUT", "/api/materials/spouse", map[string]any{
		"FiscalYear": 2025, "Name": "花子", "BirthDate": "1990-05-01",
	}); status != http.StatusOK {
		t.Fatalf("配偶者登録 = %d", status)
	}
	if status, _ := doJSON(t, server, "DELETE", "/api/materials/spouse?year=2025", nil); status != http.StatusNoContent {
		t.Errorf("配偶者削除 = %d", status)
	}

	// バッチ登録
	status, body = doJSON(t, server, "POST", "/api/journals/batch", map[string]any{
		"Entries": []any{
			salesJournal("2025-06-01", 110000)["Entry"],
			salesJournal("2025-06-02", 220000)["Entry"],
		},
	})
	ids, _ := body["IDs"].([]any)
	if status != http.StatusCreated || len(ids) != 2 {
		t.Errorf("バッチ: %d %v", status, body)
	}

	// 年度一覧
	status, body = doJSON(t, server, "GET", "/api/fiscal-years", nil)
	years, _ := body["Years"].([]any)
	if status != http.StatusOK || len(years) != 1 {
		t.Errorf("年度一覧: %d %v", status, body)
	}

	// 期首残高の登録・一覧・削除
	status, body = doJSON(t, server, "POST", "/api/opening-balances", map[string]any{
		"FiscalYear": 2025, "AccountCode": "1001", "Amount": 100000,
	})
	if status != http.StatusCreated {
		t.Fatalf("期首残高: %d %v", status, body)
	}
	status, body = doJSON(t, server, "GET", "/api/opening-balances?year=2025", nil)
	balances, _ := body["OpeningBalances"].([]any)
	if status != http.StatusOK || len(balances) != 1 {
		t.Fatalf("期首残高一覧: %d %v", status, body)
	}
	obID := int64(balances[0].(map[string]any)["ID"].(float64))
	if status, _ := doJSON(t, server, "DELETE", fmt.Sprintf("/api/opening-balances/%d", obID), nil); status != http.StatusNoContent {
		t.Errorf("期首残高削除 = %d", status)
	}

	// 検索クエリの全パラメータ
	status, _ = doJSON(t, server, "GET",
		"/api/journals?year=2025&date_from=2025-01-01&date_to=2025-12-31&description=売上&counterparty=&amount_min=1&amount_max=999999999&source=manual&limit=10&offset=0", nil)
	if status != http.StatusOK {
		t.Errorf("フル検索 = %d", status)
	}
}

// 全エンドポイントの不正パラメータが 400 になること (エラー分岐の一括検証)。
func TestInvalidParamsSweep(t *testing.T) {
	server := newTestServer(t)
	setupYear(t, server)

	badYear := []struct{ method, path string }{
		{"GET", "/api/journals?year=abc"},
		{"GET", "/api/journals/duplicates?year=abc"},
		{"GET", "/api/reports/trial-balance?year=abc"},
		{"GET", "/api/reports/profit-and-loss?year=abc"},
		{"GET", "/api/reports/balance-sheet?year=abc"},
		{"GET", "/api/reports/general-ledger?year=abc"},
		{"GET", "/api/opening-balances?year=abc"},
		{"GET", "/api/materials/dependents?year=abc"},
		{"GET", "/api/materials/spouse?year=abc"},
		{"DELETE", "/api/materials/spouse?year=abc"},
		{"POST", "/api/fiscal-years/abc/close"},
		{"POST", "/api/fiscal-years/abc/reopen"},
		{"POST", "/api/filing/sanity-check?year=abc"},
		{"POST", "/api/filing/furusato-summary?year=abc"},
		{"GET", "/api/filing/depreciation?year=abc"},
		{"POST", "/api/filing/consumption-tax?year=abc"},
	}
	for _, tt := range badYear {
		status, _ := doJSON(t, server, tt.method, tt.path, nil)
		if status != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400", tt.method, tt.path, status)
		}
	}

	badID := []struct{ method, path string }{
		{"PUT", "/api/journals/0"},
		{"DELETE", "/api/journals/0"},
		{"GET", "/api/journals/0/audit-logs"},
		{"GET", "/api/materials/dependents/0"},
		{"PUT", "/api/materials/dependents/0"},
		{"DELETE", "/api/materials/dependents/0"},
		{"DELETE", "/api/opening-balances/0"},
	}
	for _, tt := range badID {
		status, _ := doJSON(t, server, tt.method, tt.path, nil)
		if status != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400", tt.method, tt.path, status)
		}
	}

	// バッチの不正 body
	if status, _ := doJSON(t, server, "POST", "/api/journals/batch", map[string]any{"Bad": 1}); status != http.StatusBadRequest {
		t.Errorf("batch bad body = %d, want 400", status)
	}
	if status, _ := doJSON(t, server, "PUT", "/api/materials/spouse", map[string]any{"Bad": 1}); status != http.StatusBadRequest {
		t.Errorf("spouse bad body = %d, want 400", status)
	}
}

// CSRF 対策: 変更系リクエストの Content-Type 検証と Host 検証。
func TestSecurityGuards(t *testing.T) {
	server := newTestServer(t)
	setupYear(t, server)

	// text/plain の変更系リクエストは拒否 (HTMLフォーム/no-cors fetch の遮断)
	req, _ := http.NewRequest("POST", server.URL+"/api/fiscal-years", bytes.NewReader([]byte(`{"Year":2026}`)))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("text/plain POST = %d, want 400", resp.StatusCode)
	}

	// Content-Type なしの変更系リクエストも拒否
	req2, _ := http.NewRequest("POST", server.URL+"/api/fiscal-years", bytes.NewReader([]byte(`{"Year":2026}`)))
	req2.Header.Del("Content-Type")
	resp2, err := server.Client().Do(req2)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("no content-type POST = %d, want 400", resp2.StatusCode)
	}

	// 許可外の Host は拒否 (DNS rebinding 対策)
	req3, _ := http.NewRequest("GET", server.URL+"/health", nil)
	req3.Host = "evil.example.com"
	resp3, err := server.Client().Do(req3)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Errorf("evil host = %d, want 400", resp3.StatusCode)
	}

	// 末尾に余分な JSON があるリクエストは拒否
	req4, _ := http.NewRequest("POST", server.URL+"/api/fiscal-years",
		bytes.NewReader([]byte(`{"Year":2026}{"Year":2027}`)))
	req4.Header.Set("Content-Type", "application/json")
	resp4, err := server.Client().Do(req4)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp4.Body.Close()
	if resp4.StatusCode != http.StatusBadRequest {
		t.Errorf("trailing JSON = %d, want 400", resp4.StatusCode)
	}

	// 未知のパスも JSON の 404
	resp5, err := server.Client().Get(server.URL + "/api/unknown")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp5.Body.Close() }()
	if resp5.StatusCode != http.StatusNotFound ||
		resp5.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Errorf("未知パス: %d %s", resp5.StatusCode, resp5.Header.Get("Content-Type"))
	}
}

func TestErrorMapping(t *testing.T) {
	server := newTestServer(t)
	setupYear(t, server)

	tests := []struct {
		name   string
		method string
		path   string
		body   any
		want   int
	}{
		{"不正な year", "GET", "/api/reports/trial-balance?year=abc", nil, http.StatusBadRequest},
		{"存在しない仕訳", "GET", "/api/journals/9999", nil, http.StatusNotFound},
		{"不正な id", "GET", "/api/journals/xyz", nil, http.StatusBadRequest},
		{"未対応年度の所得税", "POST", "/api/filing/income-tax?year=2024", map[string]any{}, http.StatusBadRequest},
		{"簡易課税で事業区分なし", "POST", "/api/filing/consumption-tax?year=2025",
			map[string]any{"Method": "simplified"}, http.StatusBadRequest},
		{"未登録の配偶者", "GET", "/api/materials/spouse?year=2025", nil, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := doJSON(t, server, tt.method, tt.path, tt.body)
			if status != tt.want {
				t.Errorf("status = %d, want %d (%v)", status, tt.want, body)
			}
		})
	}
}
