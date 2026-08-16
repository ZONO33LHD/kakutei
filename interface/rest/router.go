package rest

import (
	"net/http"

	"github.com/ZONO33LHD/kakutei/usecase"
)

// Usecases はルーター組み立てに必要なアプリケーションサービスの束。
type Usecases struct {
	FiscalYears usecase.FiscalYearUsecase
	Journals    usecase.JournalUsecase
	Reports     usecase.ReportUsecase
	Materials   *usecase.Materials
	Filing      usecase.FilingUsecase
}

// NewRouter は全エンドポイントを登録した http.Handler を返す。
//
// allowedHosts は HostGuard で追加許可するホスト名 (DNS rebinding 対策。
// localhost / 127.0.0.1 / ::1 は常に許可)。
func NewRouter(uc Usecases, allowedHosts ...string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"Status": "ok"})
	})

	// 未登録パスも JSON エラーで応答する (API 規約の一貫性)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, errorBody{Error: errorDetail{
			Code: "NOT_FOUND", Message: "エンドポイントが見つかりません: " + r.URL.Path,
		}})
	})

	fiscalYears := NewFiscalYearHandler(uc.FiscalYears)
	mux.HandleFunc("POST /api/fiscal-years", fiscalYears.Create)
	mux.HandleFunc("GET /api/fiscal-years", fiscalYears.List)
	mux.HandleFunc("POST /api/fiscal-years/{year}/close", fiscalYears.Close)
	mux.HandleFunc("POST /api/fiscal-years/{year}/reopen", fiscalYears.Reopen)

	journals := NewJournalHandler(uc.Journals)
	mux.HandleFunc("POST /api/journals", journals.Add)
	mux.HandleFunc("POST /api/journals/batch", journals.AddBatch)
	mux.HandleFunc("GET /api/journals", journals.Search)
	// ServeMux は具体的なパターンを優先するため、静的な duplicates と {id} は共存できる
	mux.HandleFunc("GET /api/journals/duplicates", journals.CheckDuplicates)
	mux.HandleFunc("GET /api/journals/{id}", journals.Get)
	mux.HandleFunc("PUT /api/journals/{id}", journals.Update)
	mux.HandleFunc("DELETE /api/journals/{id}", journals.Delete)
	mux.HandleFunc("GET /api/journals/{id}/audit-logs", journals.AuditLogs)

	reports := NewReportHandler(uc.Reports)
	mux.HandleFunc("GET /api/accounts", reports.ListAccounts)
	mux.HandleFunc("GET /api/reports/trial-balance", reports.TrialBalance)
	mux.HandleFunc("GET /api/reports/profit-and-loss", reports.ProfitAndLoss)
	mux.HandleFunc("GET /api/reports/balance-sheet", reports.BalanceSheet)
	mux.HandleFunc("GET /api/reports/general-ledger", reports.GeneralLedger)
	mux.HandleFunc("POST /api/opening-balances", reports.SetOpeningBalance)
	mux.HandleFunc("GET /api/opening-balances", reports.ListOpeningBalances)
	mux.HandleFunc("DELETE /api/opening-balances/{id}", reports.DeleteOpeningBalance)

	registerMaterials(mux, uc.Materials)

	filing := NewFilingHandler(uc.Filing)
	mux.HandleFunc("POST /api/filing/income-tax", filing.IncomeTax)
	mux.HandleFunc("POST /api/filing/consumption-tax", filing.ConsumptionTax)
	mux.HandleFunc("POST /api/filing/sanity-check", filing.SanityCheck)
	mux.HandleFunc("POST /api/filing/furusato-summary", filing.FurusatoSummary)
	mux.HandleFunc("GET /api/filing/depreciation", filing.Depreciation)

	// AccessLog を最も外側に置き、panic 時 (Recover が 500 を返すケース) も記録する。
	return Chain(mux, AccessLog, Recover, HostGuard(allowedHosts...), RequireJSONContentType)
}

// registerMaterials は申告資料の全 CRUD ルートを登録する。
func registerMaterials(mux *http.ServeMux, m *usecase.Materials) {
	// 配偶者 (年度1件) は年度キーの upsert のため専用ルート
	spouse := NewSpouseHandler(m.Spouse)
	mux.HandleFunc("PUT /api/materials/spouse", spouse.Set)
	mux.HandleFunc("GET /api/materials/spouse", spouse.Get)
	mux.HandleFunc("DELETE /api/materials/spouse", spouse.Delete)

	registerMaterial(mux, "withholding-slips", m.WithholdingSlips)
	registerMaterial(mux, "dependents", m.Dependents)
	registerMaterial(mux, "furusato-donations", m.FurusatoDonations)
	registerMaterial(mux, "donations", m.Donations)
	registerMaterial(mux, "medical-expenses", m.MedicalExpenses)
	registerMaterial(mux, "social-insurances", m.SocialInsurances)
	registerMaterial(mux, "insurance-policies", m.InsurancePolicies)
	registerMaterial(mux, "business-withholdings", m.BusinessWithholdings)
	registerMaterial(mux, "loss-carryforwards", m.LossCarryforwards)
	registerMaterial(mux, "housing-loans", m.HousingLoans)
	registerMaterial(mux, "fixed-assets", m.FixedAssets)
	registerMaterial(mux, "other-incomes", m.OtherIncomes)
}
