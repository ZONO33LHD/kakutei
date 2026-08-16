package rest

import (
	"net/http"

	"github.com/ZONO33LHD/kakutei/domain/model"
	"github.com/ZONO33LHD/kakutei/usecase"
)

// materialHandler は年度スコープの申告資料の共通ハンドラ (ジェネリクス)。
// エンドポイント: POST/GET (一覧) /api/materials/{name}、
// GET/PUT/DELETE /api/materials/{name}/{id}
type materialHandler[T any] struct {
	uc usecase.MaterialUsecase[T]
}

func (h *materialHandler[T]) add(w http.ResponseWriter, r *http.Request) {
	record := new(T)
	if err := decodeBody(w, r, record); err != nil {
		writeError(w, err)
		return
	}
	if _, err := h.uc.Add(r.Context(), record); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (h *materialHandler[T]) list(w http.ResponseWriter, r *http.Request) {
	year, err := yearParam(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, err)
		return
	}
	records, err := h.uc.List(r.Context(), year)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"Records": records})
}

func (h *materialHandler[T]) get(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeError(w, err)
		return
	}
	record, err := h.uc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *materialHandler[T]) update(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeError(w, err)
		return
	}
	record := new(T)
	if err := decodeBody(w, r, record); err != nil {
		writeError(w, err)
		return
	}
	setRecordID(record, id)
	if err := h.uc.Update(r.Context(), record); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *materialHandler[T]) delete(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.uc.Delete(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// setRecordID はパスの {id} をレコードへ反映する。
// 全申告資料は ID フィールドを持つ (型スイッチで網羅、未知型は panic = 配線ミス)。
func setRecordID(record any, id int64) {
	switch r := record.(type) {
	case *model.WithholdingSlip:
		r.ID = id
	case *model.Dependent:
		r.ID = id
	case *model.FurusatoDonation:
		r.ID = id
	case *model.DonationRecord:
		r.ID = id
	case *model.MedicalExpense:
		r.ID = id
	case *model.SocialInsuranceItem:
		r.ID = id
	case *model.InsurancePolicy:
		r.ID = id
	case *model.BusinessWithholding:
		r.ID = id
	case *model.LossCarryforward:
		r.ID = id
	case *model.HousingLoanDetail:
		r.ID = id
	case *model.FixedAsset:
		r.ID = id
	case *model.OtherIncome:
		r.ID = id
	default:
		panic("rest.setRecordID: 未対応の申告資料型です")
	}
}

// registerMaterial は申告資料の CRUD ルートを登録する。
func registerMaterial[T any](mux *http.ServeMux, name string, uc usecase.MaterialUsecase[T]) {
	h := &materialHandler[T]{uc: uc}
	base := "/api/materials/" + name
	mux.HandleFunc("POST "+base, h.add)
	mux.HandleFunc("GET "+base, h.list)
	mux.HandleFunc("GET "+base+"/{id}", h.get)
	mux.HandleFunc("PUT "+base+"/{id}", h.update)
	mux.HandleFunc("DELETE "+base+"/{id}", h.delete)
}

// SpouseHandler は配偶者情報 (年度1件) のエンドポイント。
type SpouseHandler struct {
	spouse usecase.SpouseUsecase
}

// NewSpouseHandler は SpouseHandler を生成する。
func NewSpouseHandler(spouse usecase.SpouseUsecase) *SpouseHandler {
	return &SpouseHandler{spouse: spouse}
}

// Set は PUT /api/materials/spouse。
func (h *SpouseHandler) Set(w http.ResponseWriter, r *http.Request) {
	var spouse model.Spouse
	if err := decodeBody(w, r, &spouse); err != nil {
		writeError(w, err)
		return
	}
	if err := h.spouse.Set(r.Context(), &spouse); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, spouse)
}

// Get は GET /api/materials/spouse?year=。未登録は 404。
func (h *SpouseHandler) Get(w http.ResponseWriter, r *http.Request) {
	year, err := yearParam(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, err)
		return
	}
	spouse, err := h.spouse.Get(r.Context(), year)
	if err != nil {
		writeError(w, err)
		return
	}
	if spouse == nil {
		writeJSON(w, http.StatusNotFound, errorBody{Error: errorDetail{
			Code: "NOT_FOUND", Message: "配偶者情報が登録されていません",
		}})
		return
	}
	writeJSON(w, http.StatusOK, spouse)
}

// Delete は DELETE /api/materials/spouse?year=。
func (h *SpouseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	year, err := yearParam(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.spouse.Delete(r.Context(), year); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
