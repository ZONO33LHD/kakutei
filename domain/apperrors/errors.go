// Package apperrors はエラーコードの定義と morikuni/failure の薄いラッパーを提供する。
//
// エラーは全て failure ベースで生成され、コード・メッセージ・コールスタックを保持する。
// ドメイン層・ユースケース層はエラーコードで失敗理由を表現し、
// interface 層 (REST) が CodeOf で取り出して HTTP ステータスへ変換する。
// failure.Is / failure.CodeOf 等の API もそのまま利用できる。
package apperrors

import (
	"github.com/morikuni/failure"
)

// Code はエラー分類コード (failure.Code を実装し、文字列比較も可能な具象型)。
type Code string

// ErrorCode は failure.Code インターフェースの実装。
func (c Code) ErrorCode() string { return string(c) }

const (
	CodeBadRequest      Code = "BAD_REQUEST"       // 入力不正
	CodeNotFound        Code = "NOT_FOUND"         // 対象が存在しない
	CodeConflict        Code = "CONFLICT"          // 重複・整合性違反
	CodePayloadTooLarge Code = "PAYLOAD_TOO_LARGE" // リクエスト body が大きすぎる
	CodeInternal        Code = "INTERNAL"          // サーバー内部エラー
)

var _ failure.Code = Code("")

// New はコード付きエラーを生成する。
func New(code Code, message string) error {
	return failure.New(code, failure.Message(message))
}

// Newf はフォーマット付きでコード付きエラーを生成する。
func Newf(code Code, format string, args ...any) error {
	return failure.New(code, failure.Messagef(format, args...))
}

// Wrap は原因エラーをコード付きでラップする (コードの付け替え/付与)。
// コールスタックはこの関数の呼び出し元を指す (%+v のトレースでラップ地点が分かる)。
func Wrap(err error, code Code, message string) error {
	// wrappers は並び順どおり外→内に適用されるため、builtin の failure.Translate と
	// 同じ "message: code(...): cause" の順になるよう Message を外側にする
	return failure.Custom(failure.Custom(err, failure.Message(message), failure.WithCode(code)),
		failure.WithFormatter(), failure.WithCallStackSkip(1))
}

// Wrapf は原因エラーのコードを保ったまま文脈メッセージを追加してラップする。
// コールスタックはこの関数の呼び出し元を指す (%+v のトレースでラップ地点が分かる)。
func Wrapf(err error, format string, args ...any) error {
	return failure.Custom(failure.Custom(err, failure.Messagef(format, args...)),
		failure.WithFormatter(), failure.WithCallStackSkip(1))
}

// CodeOf はエラーからコードを取り出す。コード無しエラーは CodeInternal 扱い。
func CodeOf(err error) Code {
	code, ok := failure.CodeOf(err)
	if !ok {
		return CodeInternal
	}
	if c, ok := code.(Code); ok {
		return c
	}
	return Code(code.ErrorCode())
}

// MessageOf はエラーから利用者向けメッセージを取り出す。
// メッセージ無し (想定外のエラー) の場合は空文字を返す。
func MessageOf(err error) string {
	if msg, ok := failure.MessageOf(err); ok {
		return msg
	}
	return ""
}
