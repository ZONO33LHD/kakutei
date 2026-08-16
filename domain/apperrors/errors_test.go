package apperrors

import (
	"errors"
	"fmt"
	"testing"
)

func TestAppErrorMessage(t *testing.T) {
	e := New(CodeNotFound, "見つかりません")
	if e.Error() != "NOT_FOUND: 見つかりません" {
		t.Errorf("Error() = %q", e.Error())
	}

	cause := errors.New("db down")
	wrapped := Wrap(cause, CodeInternal, "保存に失敗しました")
	if wrapped.Error() != "INTERNAL: 保存に失敗しました: db down" {
		t.Errorf("Error() = %q", wrapped.Error())
	}
	if !errors.Is(wrapped, cause) {
		t.Error("Unwrap で原因エラーへ辿れるべき")
	}
}

func TestNewf(t *testing.T) {
	e := Newf(CodeBadRequest, "値が不正です: %d", 42)
	if e.Message != "値が不正です: 42" {
		t.Errorf("Message = %q", e.Message)
	}
}

func TestCodeOf(t *testing.T) {
	if CodeOf(New(CodeConflict, "x")) != CodeConflict {
		t.Error("AppError から Code を取り出せるべき")
	}
	if CodeOf(fmt.Errorf("wrap: %w", New(CodeNotFound, "x"))) != CodeNotFound {
		t.Error("ラップされた AppError からも Code を取り出せるべき")
	}
	if CodeOf(errors.New("plain")) != CodeInternal {
		t.Error("AppError 以外は CodeInternal")
	}
	if CodeOf(nil) != CodeInternal {
		t.Error("nil は CodeInternal")
	}
}
