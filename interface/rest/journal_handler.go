package rest

import (
	"net/http"
	"strconv"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/domain/repository"
	"github.com/ZONO33LHD/kakutei/usecase"
)

type JournalHandler struct {
	journals usecase.JournalUsecase
}

func NewJournalHandler(journals usecase.JournalUsecase) *JournalHandler {
	return &JournalHandler{journals: journals}
}

type addJournalRequest struct {
	Entry model.JournalEntry
	Force bool
}

// Add は POST /api/journals。
func (h *JournalHandler) Add(w http.ResponseWriter, r *http.Request) {
	var req addJournalRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := h.journals.Add(r.Context(), &req.Entry, req.Force)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, result)
}

type addJournalsBatchRequest struct {
	Entries []model.JournalEntry
	Force   bool
}

// AddBatch は POST /api/journals/batch。
func (h *JournalHandler) AddBatch(w http.ResponseWriter, r *http.Request) {
	var req addJournalsBatchRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	ids, warnings, err := h.journals.AddBatch(r.Context(), req.Entries, req.Force)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"IDs": ids, "Warnings": warnings})
}

// Get は GET /api/journals/{id}。
func (h *JournalHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	entry, err := h.journals.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, entry)
}

// Search は GET /api/journals?year=...&...。
func (h *JournalHandler) Search(w http.ResponseWriter, r *http.Request) {
	q, err := parseSearchQuery(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	entries, total, err := h.journals.Search(r.Context(), q)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"Journals": entries, "TotalCount": total})
}

func parseSearchQuery(r *http.Request) (repository.JournalSearchQuery, error) {
	var q repository.JournalSearchQuery
	values := r.URL.Query()

	year, err := yearParam(values.Get("year"))
	if err != nil {
		return q, err
	}
	q.FiscalYear = year

	if v := values.Get("date_from"); v != "" {
		if q.DateFrom, err = model.ParseDate(v); err != nil {
			return q, err
		}
	}
	if v := values.Get("date_to"); v != "" {
		if q.DateTo, err = model.ParseDate(v); err != nil {
			return q, err
		}
	}
	q.AccountCode = model.AccountCode(values.Get("account_code"))
	q.DescriptionContains = values.Get("description")
	q.CounterpartyContains = values.Get("counterparty")
	q.Source = model.JournalSource(values.Get("source"))
	if v := values.Get("amount_min"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return q, badQuery("amount_min", v)
		}
		q.AmountMin = model.Money(n)
	}
	if v := values.Get("amount_max"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return q, badQuery("amount_max", v)
		}
		q.AmountMax = model.Money(n)
	}
	if v := values.Get("limit"); v != "" {
		if q.Limit, err = strconv.Atoi(v); err != nil {
			return q, badQuery("limit", v)
		}
	}
	if v := values.Get("offset"); v != "" {
		if q.Offset, err = strconv.Atoi(v); err != nil {
			return q, badQuery("offset", v)
		}
	}
	return q, nil
}

// Update は PUT /api/journals/{id}。
func (h *JournalHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var entry model.JournalEntry
	if err := decodeBody(w, r, &entry); err != nil {
		writeError(w, r, err)
		return
	}
	entry.ID = id
	if err := h.journals.Update(r.Context(), &entry); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, entry)
}

// Delete は DELETE /api/journals/{id}。
func (h *JournalHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := h.journals.Delete(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusNoContent, nil)
}

// AuditLogs は GET /api/journals/{id}/audit-logs。
func (h *JournalHandler) AuditLogs(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	logs, err := h.journals.ListAuditLogs(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"AuditLogs": logs})
}

// CheckDuplicates は GET /api/journals/duplicates?year=&threshold=。
func (h *JournalHandler) CheckDuplicates(w http.ResponseWriter, r *http.Request) {
	year, err := yearParam(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	threshold := 0
	if v := r.URL.Query().Get("threshold"); v != "" {
		if threshold, err = strconv.Atoi(v); err != nil {
			writeError(w, r, badQuery("threshold", v))
			return
		}
	}
	result, err := h.journals.CheckDuplicates(r.Context(), year, threshold)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}
