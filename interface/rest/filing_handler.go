package rest

import (
	"net/http"

	"github.com/ZONO33LHD/kakutei/usecase"
)

type FilingHandler struct {
	filing usecase.FilingUsecase
}

func NewFilingHandler(filing usecase.FilingUsecase) *FilingHandler {
	return &FilingHandler{filing: filing}
}

// IncomeTax は POST /api/filing/income-tax?year=。
// body は usecase.IncomeTaxOptions (省略時は全てゼロ値)。
func (h *FilingHandler) IncomeTax(w http.ResponseWriter, r *http.Request) {
	year, err := yearParam(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	var opts usecase.IncomeTaxOptions
	if err := decodeOptionalBody(w, r, &opts); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := h.filing.CalculateIncomeTax(r.Context(), year, opts)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

// ConsumptionTax は POST /api/filing/consumption-tax?year=。
// body は usecase.ConsumptionTaxOptions。
func (h *FilingHandler) ConsumptionTax(w http.ResponseWriter, r *http.Request) {
	year, err := yearParam(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	var opts usecase.ConsumptionTaxOptions
	if err := decodeBody(w, r, &opts); err != nil {
		writeError(w, r, err)
		return
	}
	outcome, err := h.filing.CalculateConsumptionTax(r.Context(), year, opts)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, outcome)
}

// SanityCheck は POST /api/filing/sanity-check?year=。
func (h *FilingHandler) SanityCheck(w http.ResponseWriter, r *http.Request) {
	year, err := yearParam(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	var opts usecase.IncomeTaxOptions
	if err := decodeOptionalBody(w, r, &opts); err != nil {
		writeError(w, r, err)
		return
	}
	outcome, err := h.filing.SanityCheck(r.Context(), year, opts)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, outcome)
}

// FurusatoSummary は POST /api/filing/furusato-summary?year=。
func (h *FilingHandler) FurusatoSummary(w http.ResponseWriter, r *http.Request) {
	year, err := yearParam(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	var opts usecase.IncomeTaxOptions
	if err := decodeOptionalBody(w, r, &opts); err != nil {
		writeError(w, r, err)
		return
	}
	summary, err := h.filing.SummarizeFurusato(r.Context(), year, opts)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, summary)
}

// Depreciation は GET /api/filing/depreciation?year=。
func (h *FilingHandler) Depreciation(w http.ResponseWriter, r *http.Request) {
	year, err := yearParam(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	outcome, err := h.filing.CalculateDepreciation(r.Context(), year)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, outcome)
}
