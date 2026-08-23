package registry

import (
	"context"
	"testing"
)

// 認証がないため、非ループバック待受は明示的なオプトインがない限り拒否する (fail closed)。
func TestEnsureLoopbackAddr(t *testing.T) {
	t.Setenv("KAKUTEI_ALLOW_NONLOOPBACK", "")
	ctx := context.Background()

	tests := []struct {
		addr string
		ok   bool
	}{
		{"127.0.0.1:8080", true},
		{"127.0.0.2:8080", true}, // 127/8 全体がループバック
		{"[::1]:8080", true},
		{"[::1%lo0]:8080", true},          // zone 付き IPv6
		{"[::ffff:127.0.0.1]:8080", true}, // IPv4-mapped IPv6
		{"localhost:8080", true},          // 名前解決の結果がループバックなら許可
		{"", false},                       // 解釈不能は拒否
		{"127.0.0.1", false},              // ポート欠落
		{"[::1", false},                   // 不正な括弧
		{"0.0.0.0:8080", false},
		{"[::]:8080", false},
		{"192.168.1.10:8080", false},
		{"[fe80::1]:8080", false},
		{"kakutei-nonexistent.invalid:8080", false}, // 解決不能なホスト名
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			err := ensureLoopbackAddr(ctx, tt.addr)
			if tt.ok && err != nil {
				t.Errorf("%q は許可されるべき: %v", tt.addr, err)
			}
			if !tt.ok && err == nil {
				t.Errorf("%q は拒否されるべき", tt.addr)
			}
		})
	}

	t.Run("オプトイン時は非ループバックを許可", func(t *testing.T) {
		t.Setenv("KAKUTEI_ALLOW_NONLOOPBACK", "1")
		if err := ensureLoopbackAddr(ctx, "0.0.0.0:8080"); err != nil {
			t.Errorf("オプトイン時は許可されるべき: %v", err)
		}
	})
	t.Run("オプトインでも構文検証は免除しない", func(t *testing.T) {
		t.Setenv("KAKUTEI_ALLOW_NONLOOPBACK", "1")
		for _, addr := range []string{"", "[::1", "127.0.0.1"} {
			if err := ensureLoopbackAddr(ctx, addr); err == nil {
				t.Errorf("%q はオプトインでも拒否されるべき", addr)
			}
		}
	})
}
