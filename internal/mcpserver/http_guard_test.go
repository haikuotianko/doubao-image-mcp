package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"doubao-image-mcp/internal/config"
)

func newSvc(t *testing.T) *Service {
	t.Helper()
	store, err := config.LoadAt(t.TempDir())
	if err != nil {
		t.Fatalf("准备配置失败: %v", err)
	}
	return New(store, func(string, ...any) {})
}

// guardHTTP 在没配令牌时必须放行，配了令牌时必须拦住没带/带错令牌的请求。
func TestGuardHTTPToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	svc := newSvc(t)
	h := svc.guardHTTP(ok)
	if code := do(t, h, ""); code != http.StatusOK {
		t.Errorf("未配置令牌时应放行，得到 %d", code)
	}

	if err := svc.store.Update(func(c *config.Config) { c.HTTPToken = "s3cret" }); err != nil {
		t.Fatal(err)
	}
	if code := do(t, h, ""); code != http.StatusUnauthorized {
		t.Errorf("没带令牌应 401，得到 %d", code)
	}
	if code := do(t, h, "Bearer wrong"); code != http.StatusUnauthorized {
		t.Errorf("令牌错误应 401，得到 %d", code)
	}
	if code := do(t, h, "Bearer s3cret"); code != http.StatusOK {
		t.Errorf("令牌正确应 200，得到 %d", code)
	}
}

// 限流按分钟计数，超过上限返回 429。
func TestGuardHTTPRateLimit(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	svc := newSvc(t)
	if err := svc.store.Update(func(c *config.Config) { c.HTTPRateLimit = 2 }); err != nil {
		t.Fatal(err)
	}
	h := svc.guardHTTP(ok)

	if code := do(t, h, ""); code != http.StatusOK {
		t.Errorf("第 1 次应通过，得到 %d", code)
	}
	if code := do(t, h, ""); code != http.StatusOK {
		t.Errorf("第 2 次应通过，得到 %d", code)
	}
	if code := do(t, h, ""); code != http.StatusTooManyRequests {
		t.Errorf("第 3 次应被限流（429），得到 %d", code)
	}
}

func do(t *testing.T, h http.Handler, auth string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}
