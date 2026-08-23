package rest

import (
	"net/http"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/usecase"
)

func badQuery(name, value string) error {
	return apperrors.Newf(apperrors.CodeBadRequest, "%s が不正です: %q", name, value)
}

// ReportHandler は財務諸表・期首残高・科目マスタのエンドポイント。
type ReportHandler struct {
	reports usecase.ReportUsecase
}

func NewReportHandler(reports usecase.ReportUsecase) *ReportHandler {
	return &ReportHandler{reports: reports}
}

func yearQuery(r *http.Request) (model.FiscalYear, error) {
	return yearParam(r.URL.Query().Get("year"))
}

// TrialBalance は GET /api/reports/trial-balance?year=。
func (h *ReportHandler) TrialBalance(w http.ResponseWriter, r *http.Request) {
	year, err := yearQuery(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	result, err := h.reports.TrialBalance(r.Context(), year)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

// ProfitAndLoss は GET /api/reports/profit-and-loss?year=。
func (h *ReportHandler) ProfitAndLoss(w http.ResponseWriter, r *http.Request) {
	year, err := yearQuery(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	result, err := h.reports.ProfitAndLoss(r.Context(), year)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

// BalanceSheet は GET /api/reports/balance-sheet?year=。
func (h *ReportHandler) BalanceSheet(w http.ResponseWriter, r *http.Request) {
	year, err := yearQuery(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	result, err := h.reports.BalanceSheet(r.Context(), year)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

// GeneralLedger は GET /api/reports/general-ledger?year=&account=。
func (h *ReportHandler) GeneralLedger(w http.ResponseWriter, r *http.Request) {
	year, err := yearQuery(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	code := model.AccountCode(r.URL.Query().Get("account"))
	result, err := h.reports.GeneralLedger(r.Context(), year, code)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

// ListAccounts は GET /api/accounts。
func (h *ReportHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.reports.ListAccounts(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, struct {
		Accounts []model.Account
	}{accounts})
}

// SetOpeningBalance は POST /api/opening-balances。
func (h *ReportHandler) SetOpeningBalance(w http.ResponseWriter, r *http.Request) {
	var balance model.OpeningBalance
	if err := decodeBody(w, r, &balance); err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.reports.SetOpeningBalance(r.Context(), &balance); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, balance)
}

// ListOpeningBalances は GET /api/opening-balances?year=。
func (h *ReportHandler) ListOpeningBalances(w http.ResponseWriter, r *http.Request) {
	year, err := yearQuery(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	balances, err := h.reports.ListOpeningBalances(r.Context(), year)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, struct {
		OpeningBalances []model.OpeningBalance
	}{balances})
}

// DeleteOpeningBalance は DELETE /api/opening-balances/{id}。
func (h *ReportHandler) DeleteOpeningBalance(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.reports.DeleteOpeningBalance(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusNoContent, nil)
}
