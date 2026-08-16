package apperrors

import (
	"errors"
	"fmt"
	"strings"
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

// Wrap/Wrapf は builtin と等価にコード・メッセージ・原因連鎖を保ち、
// コールスタックの先頭は facade でなく呼び出し元を指す。
func TestWrapCallStackPointsToCaller(t *testing.T) {
	cause := errors.New("db down")
	wrapped := Wrap(cause, CodeInternal, "保存に失敗しました")

	if CodeOf(wrapped) != CodeInternal || MessageOf(wrapped) != "保存に失敗しました" {
		t.Errorf("コード/メッセージが不正: %v / %q", CodeOf(wrapped), MessageOf(wrapped))
	}
	if !errors.Is(wrapped, cause) {
		t.Error("原因エラーへ辿れるべき")
	}
	cs, ok := failure.CallStackOf(wrapped)
	if !ok {
		t.Fatal("コールスタックが付与されるべき")
	}
	if f := cs.HeadFrame(); f.File() != "errors_test.go" {
		t.Errorf("先頭フレームは呼び出し元を指すべき: %s:%d", f.File(), f.Line())
	}

	wrappedF := Wrapf(New(CodeConflict, "重複"), "2 件目")
	cs, ok = failure.CallStackOf(wrappedF)
	if !ok {
		t.Fatal("コールスタックが付与されるべき")
	}
	// CallStackOf は最深部 (New の地点) を返す。最深部の先頭は facade 内になる
	if f := cs.HeadFrame(); f.File() != "errors.go" {
		t.Errorf("最深部のコールスタックが返るべき: %s", f.File())
	}
}

// Error() の文言順序が builtin (failure.Translate) と同じであることを固定する。
func TestWrapErrorFormat(t *testing.T) {
	wrapped := Wrap(errors.New("db down"), CodeInternal, "保存に失敗しました")
	if s := wrapped.Error(); !strings.Contains(s, "保存に失敗しました: code(INTERNAL): db down") {
		t.Errorf("Error() の順序が builtin と不一致: %q", s)
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
