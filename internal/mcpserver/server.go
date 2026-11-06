// Package mcpserver 把豆包生图能力暴露成 MCP 工具（stdio 与 streamable HTTP 共用一套工具）。
package mcpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"doubao-image-mcp/internal/accountpool"
	"doubao-image-mcp/internal/config"
	"doubao-image-mcp/internal/doubao"
	"doubao-image-mcp/internal/loginwin"
)

// Service 持有配置与日志，工具执行时按当前配置创建客户端。
type Service struct {
	store *config.Store
	logf  func(format string, args ...any)
	pool  *accountpool.Pool

	mu      sync.Mutex
	running bool
}

// New 创建服务。
func New(store *config.Store, logf func(format string, args ...any)) *Service {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Service{
		store: store,
		logf:  logf,
		pool:  accountpool.New(store.EnabledAccounts),
	}
}

// ImageInput 是 doubao_image 的入参。
type ImageInput struct {
	Prompt         string   `json:"prompt" jsonschema:"要生成图片的画面描述，越具体越好"`
	Ratio          string   `json:"ratio,omitempty" jsonschema:"图片比例，如 1:1、16:9、9:16、4:3、3:4；留空用配置默认值"`
	Model          string   `json:"model,omitempty" jsonschema:"模型名：Seedream 5.0 Flash(免费推荐) / Seedream 4.5 / Seedream 4.0(默认) / Seedream 5.0 Pro(需订阅)"`
	Style          string   `json:"style,omitempty" jsonschema:"风格 option_key，如 skill_image_styles_film；留空不加风格"`
	RefImages      []string `json:"ref_images,omitempty" jsonschema:"参考图本地绝对路径列表；非空时走图生图"`
	OutDir         string   `json:"out_dir,omitempty" jsonschema:"图片保存目录；留空用配置里的输出目录"`
	FitRatio       *bool    `json:"fit_ratio,omitempty" jsonschema:"是否把结果裁剪/补边到请求比例，默认跟随配置"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty" jsonschema:"超时秒数，默认 180"`
	Inline         bool     `json:"inline,omitempty" jsonschema:"是否把图片本体以 base64 内联返回（体积大，默认 false 只给路径/链接）"`
}

// ImageItem 是单张结果图。
type ImageItem struct {
	Path   string `json:"path,omitempty"`
	URL    string `json:"url,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Format string `json:"format,omitempty"`
}

// ImageOutput 是 doubao_image 的返回值。
type ImageOutput struct {
	Prompt  string      `json:"prompt"`
	Model   string      `json:"model"`
	Ratio   string      `json:"ratio"`
	Channel string      `json:"channel"`
	Notice  string      `json:"notice,omitempty"`
	Images  []ImageItem `json:"images"`
}

// ModelsOutput 是 doubao_models 的返回值。
type ModelsOutput struct {
	Models       []doubao.Model `json:"models"`
	DefaultModel string         `json:"defaultModel"`
	Styles       []doubao.Style `json:"styles"`
	Ratios       []string       `json:"ratios"`
}

// AccountsInput 是 doubao_accounts 的入参。
type AccountsInput struct {
	Action string `json:"action,omitempty" jsonschema:"list(默认) / switch / remove"`
	Name   string `json:"name,omitempty" jsonschema:"switch 或 remove 时要操作的账号名"`
}

// AccountItem 是账号池里的一项。
type AccountItem struct {
	Name     string `json:"name"`
	Nickname string `json:"nickname,omitempty"`
	Enabled  bool   `json:"enabled"`
	Busy     bool   `json:"busy,omitempty"`
	Active   bool   `json:"active"`
	AddedAt  string `json:"addedAt,omitempty"`
	Remark   string `json:"remark,omitempty"`
}

// AccountsOutput 是 doubao_accounts 的返回值。
type AccountsOutput struct {
	Active   string        `json:"active"`
	Waiting  int           `json:"waiting"`
	Accounts []AccountItem `json:"accounts"`
}

// PingOutput 是 doubao_ping 的返回值。
type PingOutput struct {
	OK      bool   `json:"ok"`
	Account string `json:"account,omitempty"`
	Message string `json:"message"`
}

// LoginOutput 是 doubao_login 的返回值。
type LoginOutput struct {
	OK       bool   `json:"ok"`
	Account  string `json:"account,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Message  string `json:"message"`
}

// NewServer 组装一个带全部工具的 MCP server（每次会话一个实例，但状态都在 store 里）。
func (s *Service) NewServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "doubao-image-mcp",
		Version: "1.0.0",
	}, &mcp.ServerOptions{
		Instructions: "豆包网页版生图工具。没有登录态时先调用 doubao_login 弹出扫码窗口，再用 doubao_image 生成图片；用 doubao_ping 检查登录态。",
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "doubao_login",
		Description: "弹出扫码登录窗口，让用户用豆包 App 扫码。没有账号、登录态失效、或 doubao_image 报「未登录」时调用它。",
	}, s.handleLogin)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "doubao_image",
		Description: "用豆包网页版生成图片（文生图；给了 ref_images 就是图生图）。返回本地路径与图片链接。没登录时先调 doubao_login。",
	}, s.handleImage)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "doubao_models",
		Description: "列出豆包可用的生图模型、风格 option_key 与常用比例。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, ModelsOutput, error) {
		cfg := s.store.Get()
		return nil, ModelsOutput{
			Models:       doubao.Models,
			DefaultModel: cfg.DefaultModel,
			Styles:       doubao.Styles,
			Ratios:       doubao.Ratios,
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "doubao_ping",
		Description: "检查当前账号登录态是否有效（顺带判断是否被 IP 风控）。",
	}, s.handlePing)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "doubao_accounts",
		Description: "查看/切换/移除已登录的豆包账号。",
	}, s.handleAccounts)

	return server
}

// handleLogin 直接从 MCP 进程弹窗扫码。
//
// stdio 模式下进程同样跑在用户桌面会话里，loginwin 又不依赖 Wails，
// 所以这里能直接开 Win32 + WebView2 窗口，扫完把账号写进配置。
func (s *Service) handleLogin(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, LoginOutput, error) {
	reply := func(out LoginOutput) (*mcp.CallToolResult, LoginOutput, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Message}}}, out, nil
	}

	if !loginwin.Supported() {
		return reply(LoginOutput{Message: "当前平台没有内置扫码登录窗口：请用 `doubao-image-mcp -set-session <sessionid>` 手动导入登录态"})
	}

	// 已经有可用账号就不打扰用户。
	for _, acc := range s.store.EnabledAccounts() {
		if acc.SessionID() != "" {
			return reply(LoginOutput{OK: true, Account: acc.Name, Nickname: acc.Nickname,
				Message: fmt.Sprintf("已经有可用账号 %s，无需登录", acc.Display())})
		}
	}

	// 已有账号的 sessionid 不算这次登录成功（清 cookie 有延迟）。
	skip := make(map[string]string)
	for _, acc := range s.store.Get().Accounts {
		if sid := acc.SessionID(); sid != "" {
			skip[sid] = acc.Display()
		}
	}

	s.logf("MCP 收到登录请求，打开扫码窗口")
	win, err := loginwin.Open(loginwin.Options{
		Logf:            s.logf,
		PollInterval:    1500 * time.Millisecond,
		ClearCookies:    true,
		SkipSessions:    skip,
	})
	if err != nil {
		return reply(LoginOutput{Message: "打不开登录窗口：" + err.Error()})
	}
	defer win.Close()

	// 留 3 分钟扫码时间；客户端取消请求时窗口也会关掉。
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cookies, err := win.WaitLogin(waitCtx)
	if err != nil {
		if errors.Is(err, loginwin.ErrClosed) {
			return reply(LoginOutput{Message: "扫码窗口被关掉了，没有完成登录"})
		}
		return reply(LoginOutput{Message: "没有等到扫码完成（" + err.Error() + "），可以再调一次 doubao_login"})
	}

	name := s.nameForSession(cookies)
	account := config.Account{Name: name, Cookies: cookies, AddedAt: time.Now()}
	if err := s.store.PutAccount(account); err != nil {
		return reply(LoginOutput{Message: "扫码成功但保存账号失败：" + err.Error()})
	}
	_ = s.store.SetActive(name)
	s.logf("扫码登录成功，已保存账号 %s（%d 项 cookie）", name, len(cookies))

	out := LoginOutput{OK: true, Account: name, Message: fmt.Sprintf("登录成功，已保存账号 %s", name)}
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer pingCancel()
	client := s.clientForAccount(account)
	if err := client.Ping(pingCtx); err != nil {
		out.Message += "（登录态校验未通过：" + doubao.FormatError(err) + "）"
		return reply(out)
	}
	if profile, err := client.FetchProfile(pingCtx); err == nil {
		_ = s.store.UpdateProfile(name, profile.Nickname, profile.Avatar)
		out.Nickname = profile.Nickname
		if profile.Nickname != "" {
			out.Message += "，昵称 " + profile.Nickname
		}
	}
	return reply(out)
}

// nameForSession 复用同一 sessionid 的账号名，新账号则用时间戳并去重。
func (s *Service) nameForSession(cookies map[string]string) string {
	sid := cookies["sessionid"]
	for _, acc := range s.store.Get().Accounts {
		if sid != "" && acc.SessionID() == sid {
			return acc.Name
		}
	}
	base := "doubao-" + time.Now().Format("0102-150405")
	name := base
	for i := 2; ; i++ {
		taken := false
		for _, acc := range s.store.Get().Accounts {
			if acc.Name == name {
				taken = true
				break
			}
		}
		if !taken {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}

// clientFor 取当前账号并构造客户端；当前账号被停用或为空时退回第一个可调度账号。
func (s *Service) clientFor() (*doubao.Client, config.Account, error) {
	account, ok := s.store.ActiveAccount()
	if !ok || !account.Enabled() {
		if list := s.store.EnabledAccounts(); len(list) > 0 {
			account, ok = list[0], true
		}
	}
	if !ok {
		return nil, config.Account{}, fmt.Errorf("还没有登录任何豆包账号，请先在 GUI 里点「扫码登录」")
	}
	client := doubao.New(account.Cookies)
	client.Debug = func(format string, args ...any) {
		s.logf(format, args...)
	}
	return client, account, nil
}

// clientForAccount 用指定账号构造客户端。
func (s *Service) clientForAccount(account config.Account) *doubao.Client {
	client := doubao.New(account.Cookies)
	client.Debug = func(format string, args ...any) {
		s.logf(format, args...)
	}
	return client
}

// Pool 暴露账号调度器，GUI 的「测试生图」也走同一套排队。
func (s *Service) Pool() *accountpool.Pool { return s.pool }

func (s *Service) handleImage(ctx context.Context, req *mcp.CallToolRequest, in ImageInput) (*mcp.CallToolResult, ImageOutput, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return nil, ImageOutput{}, fmt.Errorf("prompt 不能为空")
	}

	// 从账号池里取一个空闲的已启用账号；都在忙就排队等，不直接失败。
	account, release, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, ImageOutput{}, err
	}
	defer release()
	client := s.clientForAccount(account)
	cfg := s.store.Get()

	ratio := firstNonEmpty(in.Ratio, cfg.DefaultRatio)
	model := firstNonEmpty(in.Model, cfg.DefaultModel)
	outDir := firstNonEmpty(in.OutDir, cfg.OutDir)
	fit := cfg.FitRatio
	if in.FitRatio != nil {
		fit = *in.FitRatio
	}
	timeout := time.Duration(in.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 180 * time.Second
	}

	s.logf("生成图片：账号=%s 模型=%s 比例=%s 风格=%q（排队中 %d）", account.Display(), model, ratio, in.Style, s.pool.Waiting())
	result, err := client.Generate(ctx, doubao.Request{
		Prompt:    in.Prompt,
		Ratio:     ratio,
		Model:     model,
		Style:     in.Style,
		RefImages: in.RefImages,
		OutDir:    outDir,
		FitRatio:  fit,
		Timeout:   timeout,
	})
	if err != nil {
		s.logf("生成失败: %v", err)
		return nil, ImageOutput{}, fmt.Errorf("%s", doubao.FormatError(err))
	}

	out := ImageOutput{
		Prompt:  result.Prompt,
		Model:   result.Model,
		Ratio:   result.Ratio,
		Channel: result.Channel,
		Notice:  result.Notice,
	}
	s.logf("生成完成：拿到 %d 张图（通道 %s）", len(result.Images), result.Channel)
	var contents []mcp.Content
	var lines []string
	for _, img := range result.Images {
		item := ImageItem{Path: img.LocalPath, URL: img.URL(), Width: img.Width, Height: img.Height, Format: img.Format}
		out.Images = append(out.Images, item)
		if item.Path != "" {
			lines = append(lines, item.Path)
		} else if item.URL != "" {
			lines = append(lines, item.URL)
		}
		if in.Inline && item.Path != "" {
			if data, mimeType, err := readImageForInline(item.Path); err == nil {
				contents = append(contents, &mcp.ImageContent{Data: data, MIMEType: mimeType})
			}
		}
	}

	summary := strings.Join(lines, "\n")
	switch {
	case summary == "":
		summary = "没有拿到图片"
	case len(lines) > 1:
		summary = fmt.Sprintf("共 %d 张：\n%s", len(lines), summary)
	}
	if out.Notice != "" {
		summary += "\n提示：" + out.Notice
	}
	contents = append([]mcp.Content{&mcp.TextContent{Text: summary}}, contents...)
	return &mcp.CallToolResult{Content: contents}, out, nil
}

func (s *Service) handlePing(ctx context.Context, req *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, PingOutput, error) {
	client, account, err := s.clientFor()
	if err != nil {
		return nil, PingOutput{OK: false, Message: err.Error()}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		out := PingOutput{OK: false, Account: account.Name, Message: doubao.FormatError(err)}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Message}}}, out, nil
	}
	message := fmt.Sprintf("账号 %s 登录态正常", account.Name)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: message}}},
		PingOutput{OK: true, Account: account.Name, Message: message}, nil
}

func (s *Service) handleAccounts(ctx context.Context, req *mcp.CallToolRequest, in AccountsInput) (*mcp.CallToolResult, AccountsOutput, error) {
	action := strings.ToLower(strings.TrimSpace(in.Action))
	switch action {
	case "", "list":
	case "switch":
		if err := s.store.SetActive(in.Name); err != nil {
			return nil, AccountsOutput{}, err
		}
	case "remove":
		if err := s.store.RemoveAccount(in.Name); err != nil {
			return nil, AccountsOutput{}, err
		}
	case "enable", "disable":
		if strings.TrimSpace(in.Name) == "" {
			return nil, AccountsOutput{}, fmt.Errorf("%s 需要 name 参数", action)
		}
		if err := s.store.SetAccountEnabled(in.Name, action == "enable"); err != nil {
			return nil, AccountsOutput{}, err
		}
	default:
		return nil, AccountsOutput{}, fmt.Errorf("未知 action %q，可用：list / switch / enable / disable / remove", in.Action)
	}

	cfg := s.store.Get()
	busy := s.pool.Busy()
	out := AccountsOutput{Active: cfg.Active, Waiting: s.pool.Waiting(), Accounts: []AccountItem{}}
	for _, a := range cfg.Accounts {
		addedAt := ""
		if !a.AddedAt.IsZero() {
			addedAt = a.AddedAt.Format("2006-01-02 15:04")
		}
		out.Accounts = append(out.Accounts, AccountItem{
			Name:     a.Name,
			Nickname: a.Nickname,
			Enabled:  a.Enabled(),
			Busy:     busy[a.Name] > 0,
			Active:   a.Name == cfg.Active,
			AddedAt:  addedAt,
			Remark:   a.Remark,
		})
	}
	payload, _ := json.Marshal(out)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}}}, out, nil
}

// ServeStdio 以 stdio 方式阻塞运行（被 MCP 客户端进程直接拉起时使用）。
func (s *Service) ServeStdio(ctx context.Context) error {
	s.logf("MCP stdio 模式启动")
	return s.NewServer().Run(ctx, &mcp.StdioTransport{})
}

// HTTPHandler 返回 streamable HTTP handler（每个请求一个 server 实例）。
func (s *Service) HTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.NewServer()
	}, nil)
}

// StartHTTP 在 addr 上启动 streamable HTTP 服务，返回可用于关闭的 server。
func (s *Service) StartHTTP(addr string) (*http.Server, error) {
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.HTTPHandler())
	mux.Handle("/mcp/", s.HTTPHandler())
	httpServer := &http.Server{Addr: addr, Handler: mux}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil, fmt.Errorf("HTTP 服务已在运行")
	}
	s.running = true
	s.mu.Unlock()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return nil, err
	}
	go func() {
		s.logf("MCP HTTP 服务已启动: http://%s/mcp", addr)
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logf("MCP HTTP 服务退出: %v", err)
		}
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	return httpServer, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func readImageForInline(path string) ([]byte, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	mimeType := "image/png"
	switch strings.ToLower(path[strings.LastIndex(path, ".")+1:]) {
	case "jpg", "jpeg":
		mimeType = "image/jpeg"
	case "webp":
		mimeType = "image/webp"
	}
	return []byte(base64.StdEncoding.EncodeToString(raw)), mimeType, nil
}
