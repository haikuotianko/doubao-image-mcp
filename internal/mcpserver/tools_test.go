package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"doubao-image-mcp/internal/config"
)

// TestToolsDeclareAnnotations 保证每个工具都声明了 MCP 规范要求的四个 hint。
// 目录站点（M8ven 等）和部分宿主会因为缺 hint 拒收或降级工具。
func TestToolsDeclareAnnotations(t *testing.T) {
	store, err := config.LoadAt(t.TempDir())
	if err != nil {
		t.Fatalf("准备配置失败: %v", err)
	}
	svc := New(store, func(string, ...any) {})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverT, clientT := mcp.NewInMemoryTransports()
	go func() { _ = svc.NewServer().Run(ctx, serverT) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer session.Close()

	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("列出工具失败: %v", err)
	}

	wantReadOnly := map[string]bool{
		"doubao_login":    false,
		"doubao_verify":   false,
		"doubao_image":    false,
		"doubao_models":   true,
		"doubao_ping":     true,
		"doubao_accounts": false,
	}

	if len(res.Tools) != len(wantReadOnly) {
		t.Errorf("工具数量 %d，期望 %d", len(res.Tools), len(wantReadOnly))
	}

	for _, tool := range res.Tools {
		want, ok := wantReadOnly[tool.Name]
		if !ok {
			t.Errorf("出现了未预期的工具 %q", tool.Name)
			continue
		}
		a := tool.Annotations
		if a == nil {
			t.Errorf("%s: 没有声明 annotations", tool.Name)
			continue
		}
		if a.DestructiveHint == nil {
			t.Errorf("%s: 缺 destructiveHint", tool.Name)
		}
		if a.OpenWorldHint == nil {
			t.Errorf("%s: 缺 openWorldHint", tool.Name)
		}
		if a.ReadOnlyHint != want {
			t.Errorf("%s: readOnlyHint=%v，期望 %v", tool.Name, a.ReadOnlyHint, want)
		}
	}
}
