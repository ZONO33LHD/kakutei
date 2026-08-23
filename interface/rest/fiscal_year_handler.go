package rest

import (
	"net/http"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/usecase"
)

type FiscalYearHandler struct {
	years usecase.FiscalYearUsecase
}

func NewFiscalYearHandler(years usecase.FiscalYearUsecase) *FiscalYearHandler {
	return &FiscalYearHandler{years: years}
}

type createFiscalYearRequest struct {
	Year model.FiscalYear
}

// Create は POST /api/fiscal-years。
func (h *FiscalYearHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createFiscalYearRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.years.Setup(r.Context(), req.Year); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, struct {
		Year model.FiscalYear
	}{req.Year})
}

// List は GET /api/fiscal-years。
func (h *FiscalYearHandler) List(w http.ResponseWriter, r *http.Request) {
	years, err := h.years.List(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, struct {
		Years []model.FiscalYearStatus
	}{years})
}

// Close は POST /api/fiscal-years/{year}/close。
func (h *FiscalYearHandler) Close(w http.ResponseWriter, r *http.Request) {
	year, err := yearParam(r.PathValue("year"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.years.Close(r.Context(), year); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, struct {
		Year  model.FiscalYear
		State model.FiscalYearState
	}{year, model.FiscalYearClosed})
}

// Reopen は POST /api/fiscal-years/{year}/reopen。
func (h *FiscalYearHandler) Reopen(w http.ResponseWriter, r *http.Request) {
	year, err := yearParam(r.PathValue("year"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.years.Reopen(r.Context(), year); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, struct {
		Year  model.FiscalYear
		State model.FiscalYearState
	}{year, model.FiscalYearOpen})
}
