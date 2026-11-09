package doubao

import (
	"strings"
	"testing"
)

func TestWithContextRule(t *testing.T) {
	prompt := "一只橘猫"

	if got := withContextRule(prompt, ContextNone); got != prompt {
		t.Errorf("none 应该原样返回，得到 %q", got)
	}

	always := withContextRule(prompt, ContextAlways)
	if !strings.Contains(always, "没有任何关联") || !strings.HasSuffix(always, prompt) {
		t.Errorf("always 应带隔离说明并以提示词结尾，得到 %q", always)
	}

	auto := withContextRule(prompt, ContextAuto)
	if !strings.Contains(auto, "如果") || !strings.HasSuffix(auto, prompt) {
		t.Errorf("auto 应带条件说明并以提示词结尾，得到 %q", auto)
	}

	// 未知值按 auto 处理，避免配置写错就变成不隔离
	if got := withContextRule(prompt, "???"); got != auto {
		t.Errorf("未知值应按 auto 处理，得到 %q", got)
	}
}

func TestNormalizeContextMode(t *testing.T) {
	cases := map[string]string{
		"":        ContextAuto,
		"auto":    ContextAuto,
		"ALWAYS":  ContextAlways,
		" always": ContextAlways,
		"none":    ContextNone,
		"bogus":   ContextAuto,
	}
	for in, want := range cases {
		if got := NormalizeContextMode(in); got != want {
			t.Errorf("NormalizeContextMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseChatID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"38451559467912706", "38451559467912706"},
		{" 38451559467912706 ", "38451559467912706"},
		{"https://www.doubao.com/chat/38451559467912706", "38451559467912706"},
		{"https://www.doubao.com/chat/38451559467912706/", "38451559467912706"},
		{"https://www.doubao.com/chat/38451559467912706?foo=1", "38451559467912706"},
		{"", ""},
		{"not-an-id", ""},
		{"https://www.doubao.com/chat/abc", ""},
	}
	for _, c := range cases {
		if got := ParseChatID(c.in); got != c.want {
			t.Errorf("ParseChatID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestChatURL(t *testing.T) {
	if got := ChatURL("123"); got != "https://www.doubao.com/chat/123" {
		t.Errorf("ChatURL(123) = %q", got)
	}
	if got := ChatURL(""); got != "" {
		t.Errorf("ChatURL(空) 应为空串，得到 %q", got)
	}
}
