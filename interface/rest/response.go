// Package rest は確定申告バックエンドの REST API (interface 層)。
//
// usecase 層を呼び出し、結果を JSON で返す。ドメインの検証エラー
// (apperrors.Code) を HTTP ステータスへ変換する責務を持つ。
//
// API 規約:
//   - リクエスト/レスポンスの JSON フィールド名は Go の公開フィールド名 (PascalCase)
//   - 金額は円単位の整数、日付は "YYYY-MM-DD" 文字列
//   - エラーは {"Error": {"Code": ..., "Message": ...}} 形式
package rest

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ZONO33LHD/kakutei/domain/apperrors"
	"github.com/ZONO33LHD/kakutei/domain/model"
)

// maxRequestBodyBytes は 1 リクエストの最大 body サイズ (バッチ仕訳を考慮して 5MiB)。
const maxRequestBodyBytes = 5 << 20

// errorBody はエラーレスポンスの形式。
type errorBody struct {
	Error errorDetail
}

type errorDetail struct {
	Code    apperrors.Code
	Message string
}

// writeJSON は 200/201 等の正常レスポンスを書き出す。
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("レスポンスの書き込みに失敗しました", "error", err)
	}
}

// writeError は apperrors.Code を HTTP ステータスへ変換して書き出す。
// INTERNAL の詳細はログにのみ残し、クライアントへは一般化したメッセージを返す。
func writeError(w http.ResponseWriter, err error) {
	code := apperrors.CodeOf(err)
	status := http.StatusInternalServerError
	message := err.Error()
	switch code {
	case apperrors.CodeBadRequest:
		status = http.StatusBadRequest
	case apperrors.CodeNotFound:
		status = http.StatusNotFound
	case apperrors.CodeConflict:
		status = http.StatusConflict
	case apperrors.CodePayloadTooLarge:
		status = http.StatusRequestEntityTooLarge
	case apperrors.CodeInternal:
		slog.Error("内部エラー", "error", err)
		message = "サーバー内部エラーが発生しました"
	}
	var ae *apperrors.AppError
	if errors.As(err, &ae) && code != apperrors.CodeInternal {
		message = ae.Message
	}
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// decodeBody はリクエスト body を JSON デコードする。
// 未知フィールド・末尾の余分なデータは拒否し、サイズ超過は 413 相当のエラーを返す。
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return wrapDecodeError(err)
	}
	// 末尾に余分なデータがないことを確認する
	// (2つ目の JSON や巨大なゴミの黙殺による意図しない副作用を防ぐ)
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return apperrors.New(apperrors.CodeBadRequest, "リクエスト body に余分なデータがあります")
		}
		return wrapDecodeError(err)
	}
	return nil
}

// decodeOptionalBody は body が空 (省略) の場合は dst をゼロ値のままにする。
// Content-Length に依存せず、実際の読み取りで空を判定する (chunked 転送対応)。
func decodeOptionalBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if errors.Is(err, io.EOF) {
		return nil // 空 body は既定値
	}
	if err != nil {
		return wrapDecodeError(err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apperrors.New(apperrors.CodeBadRequest, "リクエスト body に余分なデータがあります")
	}
	return nil
}

// wrapDecodeError は JSON デコードエラーを HTTP エラーへ変換する。
func wrapDecodeError(err error) error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return apperrors.Newf(apperrors.CodePayloadTooLarge,
			"リクエスト body が大きすぎます (上限 %d バイト)", maxErr.Limit)
	}
	return apperrors.Wrap(err, apperrors.CodeBadRequest, "リクエスト body の JSON が不正です")
}

// yearParam はクエリまたはパスの year を FiscalYear として取り出す。
func yearParam(value string) (model.FiscalYear, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, apperrors.Newf(apperrors.CodeBadRequest, "year が不正です: %q", value)
	}
	year := model.FiscalYear(n)
	if err := year.Validate(); err != nil {
		return 0, err
	}
	return year, nil
}

// idParam はパスの {id} を int64 として取り出す。
func idParam(r *http.Request) (int64, error) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperrors.Newf(apperrors.CodeBadRequest, "id が不正です: %q", raw)
	}
	return id, nil
}
