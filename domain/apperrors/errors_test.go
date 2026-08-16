package apperrors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/morikuni/failure"
)

func TestNewAndWrap(t *testing.T) {
	e := New(CodeNotFound, "見つかりません")
	if MessageOf(e) != "見つかりません" {
		t.Errorf("MessageOf = %q", MessageOf(e))
	}
	if !failure.Is(e, CodeNotFound) {
		t.Error("failure.Is でコード照合できるべき")
	}

	cause := errors.New("db down")
	wrapped := Wrap(cause, CodeInternal, "保存に失敗しました")
	if CodeOf(wrapped) != CodeInternal {
		t.Errorf("CodeOf = %v", CodeOf(wrapped))
	}
	if MessageOf(wrapped) != "保存に失敗しました" {
		t.Errorf("MessageOf = %q", MessageOf(wrapped))
	}
	if !errors.Is(wrapped, cause) {
		t.Error("原因エラーへ辿れるべき")
	}
}

func TestWrapfKeepsCode(t *testing.T) {
	e := Wrapf(New(CodeBadRequest, "元のエラー"), "仕訳 %d 件目の検証", 3)
	if CodeOf(e) != CodeBadRequest {
		t.Errorf("Wrapf はコードを保つべき: %v", CodeOf(e))
	}
}

func TestNewf(t *testing.T) {
	e := Newf(CodeBadRequest, "値が不正です: %d", 42)
	if MessageOf(e) != "値が不正です: 42" {
		t.Errorf("MessageOf = %q", MessageOf(e))
	}
}

func TestCodeOf(t *testing.T) {
	if CodeOf(New(CodeConflict, "x")) != CodeConflict {
		t.Error("コードを取り出せるべき")
	}
	if CodeOf(fmt.Errorf("wrap: %w", New(CodeNotFound, "x"))) != CodeNotFound {
		t.Error("fmt.Errorf でラップされてもコードを取り出せるべき")
	}
	if CodeOf(errors.New("plain")) != CodeInternal {
		t.Error("コード無しは CodeInternal")
	}
	if CodeOf(nil) != CodeInternal {
		t.Error("nil は CodeInternal")
	}
}
