// Package apperrors はアプリケーション全体で使うエラー型を提供する。
//
// ドメイン層・ユースケース層はこのパッケージのエラーコードで失敗理由を表現し、
// interface 層 (REST) が HTTP ステータスへ変換する。
package apperrors

import (
	"errors"
	"fmt"
)

// Code はエラー分類コード。
type Code string

const (
	CodeBadRequest Code = "BAD_REQUEST" // 入力不正
	CodeNotFound   Code = "NOT_FOUND"   // 対象が存在しない
	CodeConflict   Code = "CONFLICT"    // 重複・整合性違反
	CodeInternal   Code = "INTERNAL"    // サーバー内部エラー
)

// AppError はコード付きエラー。Err に原因を保持し errors.Is / As で辿れる。
type AppError struct {
	Code    Code
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Err }

// New はコードとメッセージから AppError を作る。
func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// Newf はフォーマット付きで AppError を作る。
func Newf(code Code, format string, args ...any) *AppError {
	return &AppError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap は原因エラーを保持した AppError を作る。
func Wrap(err error, code Code, message string) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

// CodeOf はエラーから Code を取り出す。AppError でなければ CodeInternal。
func CodeOf(err error) Code {
	if ae, ok := errors.AsType[*AppError](err); ok {
		return ae.Code
	}
	return CodeInternal
}
