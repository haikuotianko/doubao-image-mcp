package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"doubao-image-mcp/internal/config"
	"doubao-image-mcp/internal/doubao"
	"doubao-image-mcp/internal/loginwin"
	"doubao-image-mcp/internal/mcpserver"
)

// App struct
type App struct {
	ctx context.Context

	store *config.Store
	svc   *mcpserver.Service

	httpMu     sync.Mutex
	httpServer *http.Server

	loginMu sync.Mutex
	login   *loginwin.Window

	logMu sync.Mutex
	logs  []string
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.logf("启动 豆包生图 MCP")

	store, err := config.Load()
	if err != nil {
		a.logf("读取配置失败: %v", err)
	}
	a.store = store
	a.svc = mcpserver.New(store, a.logf)

	cfg := store.Get()
	a.logf("配置目录: %s", config.Dir())
	a.logf("图片输出目录: %s", cfg.OutDir)
	a.logf("WebView2 运行时: %s", webViewRuntime())

	if cfg.AutoStartHTTP {
		if err := a.StartHTTP(); err != nil {
			a.logf("自动启动 HTTP 失败: %v", err)
		}
	}

	// 老账号可能还没有头像/昵称（早先版本没抓），后台补一次。
	a.refreshMissingProfiles()

	// 默认账号被停用时清掉，界面和调度都按「自动（第一个启用账号）」走。
	if cfg.Active != "" {
		if acc, ok := store.Account(cfg.Active); ok && !acc.Enabled() {
			_ = store.SetActive("")
			a.logf("默认账号 %s 已停用，改回自动选择", acc.Display())
		}
	}
}

// shutdown 关闭窗口/服务，避免留下孤儿进程。
func (a *App) shutdown(ctx context.Context) {
	a.CancelLogin()
	_ = a.StopHTTP()
}

// ---------------------------------------------------------------- 状态与设置

// AccountView 是账号池里的一项（给前端用）。
type AccountView struct {
	Name        string `json:"name"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	Enabled     bool   `json:"enabled"`
	Busy        bool   `json:"busy"`
	Active      bool   `json:"active"`
	AddedAt     string `json:"addedAt"`
	Remark      string `json:"remark"`
	CookieCount int    `json:"cookieCount"`
}

// Status 是前端启动时拉取的完整状态。
type Status struct {
	ActiveAccount string         `json:"activeAccount"`
	Accounts      []AccountView  `json:"accounts"`
	Waiting       int            `json:"waiting"`
	HTTPRunning   bool           `json:"httpRunning"`
	HTTPAddr      string         `json:"httpAddr"`
	OutDir        string         `json:"outDir"`
	DefaultModel  string         `json:"defaultModel"`
	DefaultRatio  string         `json:"defaultRatio"`
	DefaultStyle  string         `json:"defaultStyle"`
	FitRatio      bool           `json:"fitRatio"`
	AutoStartHTTP bool           `json:"autoStartHttp"`
	ConfigPath    string         `json:"configPath"`
	ExePath       string         `json:"exePath"`
	Platform      string         `json:"platform"`
	LoginSupport  bool           `json:"loginSupported"`
	Models        []doubao.Model `json:"models"`
	Styles        []doubao.Style `json:"styles"`
	Ratios        []string       `json:"ratios"`
	Logs          []string       `json:"logs"`
}

// Status 返回当前状态。
func (a *App) Status() Status {
	cfg := a.store.Get()
	st := Status{
		ActiveAccount: cfg.Active,
		HTTPRunning:   a.HTTPRunning(),
		HTTPAddr:      cfg.HTTPAddr,
		OutDir:        cfg.OutDir,
		DefaultModel:  cfg.DefaultModel,
		DefaultRatio:  cfg.DefaultRatio,
		DefaultStyle:  cfg.DefaultStyle,
		FitRatio:      cfg.FitRatio,
		AutoStartHTTP: cfg.AutoStartHTTP,
		ConfigPath:    config.Path(),
		ExePath:       exePath(),
		Platform:      runtime.GOOS,
		LoginSupport:  loginwin.Supported(),
		Models:        doubao.Models,
		Styles:        doubao.Styles,
		Ratios:        doubao.Ratios,
		Logs:          a.Logs(),
		Waiting:       a.svc.Pool().Waiting(),
	}
	busy := a.svc.Pool().Busy()
	for _, acc := range cfg.Accounts {
		addedAt := ""
		if !acc.AddedAt.IsZero() {
			addedAt = acc.AddedAt.Format("2006-01-02 15:04")
		}
		st.Accounts = append(st.Accounts, AccountView{
			Name:        acc.Name,
			Nickname:    acc.Nickname,
			Avatar:      acc.Avatar,
			Enabled:     acc.Enabled(),
			Busy:        busy[acc.Name] > 0,
			Active:      acc.Name == cfg.Active,
			AddedAt:     addedAt,
			Remark:      acc.Remark,
			CookieCount: len(acc.Cookies),
		})
	}
	return st
}

// Settings 是前端可改的配置项。
type Settings struct {
	OutDir        string `json:"outDir"`
	DefaultModel  string `json:"defaultModel"`
	DefaultRatio  string `json:"defaultRatio"`
	DefaultStyle  string `json:"defaultStyle"`
	FitRatio      bool   `json:"fitRatio"`
	HTTPAddr      string `json:"httpAddr"`
	AutoStartHTTP bool   `json:"autoStartHttp"`
}

// SaveSettings 落盘配置。
func (a *App) SaveSettings(s Settings) error {
	return a.store.Update(func(cfg *config.Config) {
		if strings.TrimSpace(s.OutDir) != "" {
			cfg.OutDir = strings.TrimSpace(s.OutDir)
		}
		cfg.DefaultModel = s.DefaultModel
		cfg.DefaultRatio = s.DefaultRatio
		cfg.DefaultStyle = s.DefaultStyle
		cfg.FitRatio = s.FitRatio
		if strings.TrimSpace(s.HTTPAddr) != "" {
			cfg.HTTPAddr = strings.TrimSpace(s.HTTPAddr)
		}
		cfg.AutoStartHTTP = s.AutoStartHTTP
	})
}

// ChooseOutDir 弹出目录选择框。
func (a *App) ChooseOutDir() (string, error) {
	dir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:            "选择图片输出目录",
		DefaultDirectory: a.store.Get().OutDir,
	})
	if err != nil || dir == "" {
		return "", err
	}
	if err := a.store.Update(func(cfg *config.Config) { cfg.OutDir = dir }); err != nil {
		return "", err
	}
	return dir, nil
}

// OpenOutDir 在系统文件管理器里打开输出目录。
func (a *App) OpenOutDir() error {
	dir := a.store.Get().OutDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return openFolder(dir)
}

// ---------------------------------------------------------------- 登录

// LoginResult 是扫码登录的结果。
type LoginResult struct {
	Account  string   `json:"account"`
	Cookies  []string `json:"cookies"`
	Verified bool     `json:"verified"`
	Message  string   `json:"message"`
}

// LoginDialog 打开内嵌浏览器窗口等待扫码登录，成功后保存账号。
func (a *App) LoginDialog() (LoginResult, error) {
	a.loginMu.Lock()
	if a.login != nil {
		a.login.Close()
		a.login = nil
	}
	a.loginMu.Unlock()

	a.logf("打开登录窗口，请用豆包 App 扫码")
	win, err := loginwin.Open(loginwin.Options{
		Logf:         a.logf,
		PollInterval: 1500 * time.Millisecond,
		// 每登录一个账号都要先清掉浏览器里的旧登录态，否则打开就是上一个账号，
		// 会被立刻判成“登录成功”，第二个账号根本扫不进去。
		ClearCookies: true,
		SkipSessions: a.knownSessions(),
	})
	if err != nil {
		return LoginResult{}, err
	}
	a.loginMu.Lock()
	a.login = win
	a.loginMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cookies, err := win.WaitLogin(ctx)
	a.loginMu.Lock()
	a.login = nil
	a.loginMu.Unlock()
	if err != nil {
		if errors.Is(err, loginwin.ErrClosed) {
			return LoginResult{}, errors.New("登录窗口已关闭，未完成登录")
		}
		return LoginResult{}, err
	}

	name := a.accountNameFor(cookies)
	account := config.Account{Name: name, Cookies: cookies, AddedAt: time.Now()}
	if err := a.store.PutAccount(account); err != nil {
		return LoginResult{}, err
	}
	a.logf("已保存账号 %s（%d 项 cookie）", name, len(cookies))

	result := LoginResult{Account: name}
	for k := range cookies {
		result.Cookies = append(result.Cookies, k)
	}
	if err := a.store.SetActive(name); err != nil {
		return result, err
	}

	// 立刻验一次登录态，顺便发现 IP 风控。
	client := doubao.New(cookies)
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer pingCancel()
	if err := client.Ping(pingCtx); err != nil {
		result.Message = doubao.FormatError(err)
		a.logf("登录态校验失败: %s", result.Message)
		return result, nil
	}
	result.Verified = true
	result.Message = "登录成功，可以开始生图了"
	a.logf("账号 %s 登录态校验通过", name)

	// 同步把昵称和头像抓下来再返回：窗口一取到 sessionid 就关了，
	// 异步拿的话界面刷新时资料还没到，列表里就会是空的（要等下次打开才出现）。
	a.logf("正在获取账号资料…")
	profileCtx, profileCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer profileCancel()
	if profile, perr := client.FetchProfile(profileCtx); perr != nil {
		a.logf("账号 %s 资料获取失败: %v", name, perr)
	} else {
		if err := a.store.UpdateProfile(name, profile.Nickname, profile.Avatar); err != nil {
			a.logf("保存账号资料失败: %v", err)
		} else {
			a.logf("账号 %s 资料已更新: %s", name, firstNonEmpty(profile.Nickname, "(无昵称)"))
		}
	}
	return result, nil
}

// refreshMissingProfiles 给还没有头像/昵称的账号补一次资料（启动时后台跑）。
func (a *App) refreshMissingProfiles() {
	for _, acc := range a.store.Get().Accounts {
		if acc.Nickname != "" && acc.Avatar != "" {
			continue
		}
		go func(name string) {
			if err := a.RefreshProfile(name); err != nil {
				a.logf("账号 %s 资料获取失败: %v", name, err)
			}
		}(acc.Name)
	}
}

// RefreshProfile 重新拉一次某个账号的昵称和头像。
func (a *App) RefreshProfile(name string) error {
	account, ok := a.store.Account(name)
	if !ok {
		return fmt.Errorf("没有账号 %s", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	profile, err := doubao.New(account.Cookies).FetchProfile(ctx)
	if err != nil {
		return err
	}
	if err := a.store.UpdateProfile(name, profile.Nickname, profile.Avatar); err != nil {
		return err
	}
	a.logf("账号 %s 资料已刷新: %s", name, firstNonEmpty(profile.Nickname, "(无昵称)"))
	return nil
}

// SetAccountEnabled 启用 / 停用一个账号（停用后不参与自动排队）。
func (a *App) SetAccountEnabled(name string, enabled bool) error {
	if err := a.store.SetAccountEnabled(name, enabled); err != nil {
		return err
	}
	state := "启用"
	if !enabled {
		state = "停用"
	}
	a.logf("账号 %s 已%s", name, state)
	return nil
}

// knownSessions 返回已有账号的 sessionid → 账号名，用来防止把旧会话当成新登录。
func (a *App) knownSessions() map[string]string {
	out := make(map[string]string)
	for _, acc := range a.store.Get().Accounts {
		if sid := acc.SessionID(); sid != "" {
			out[sid] = acc.Display()
		}
	}
	return out
}

// accountNameFor 复用同一个 sessionid 已有账号的名字，避免重复登录攒出一堆账号。
func (a *App) accountNameFor(cookies map[string]string) string {
	if sid := cookies["sessionid"]; sid != "" {
		for _, acc := range a.store.Get().Accounts {
			if acc.Cookies["sessionid"] == sid {
				return acc.Name
			}
		}
	}
	// 名字要能区分同一分钟内连续登录的多个账号。
	base := fmt.Sprintf("doubao-%s", time.Now().Format("0102-150405"))
	name := base
	for i := 2; ; i++ {
		taken := false
		for _, acc := range a.store.Get().Accounts {
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

// CancelLogin 关掉正在等待的登录窗口。
func (a *App) CancelLogin() {
	a.loginMu.Lock()
	win := a.login
	a.loginMu.Unlock()
	if win != nil {
		win.Close()
	}
}

// ImportSession 手动导入登录态。
// 这是 mac / Linux 上的登录方式（那边没有内置 WebView2 扫码窗口）。
// 参数可以是一整段 Cookie（`sessionid=...; ttwid=...`）、cURL 命令，或者裸 sessionid。
func (a *App) ImportSession(sessionInput, ttwid string) (string, error) {
	cookies := config.ParseCookies(sessionInput)
	if len(cookies) == 0 {
		return "", errors.New("没解析到 cookie：请粘贴 sessionid，或整段 Cookie（sessionid=...; ttwid=...）")
	}
	if cookies["sessionid"] == "" {
		return "", errors.New("粘贴的内容里没有 sessionid：请复制整段 Cookie，或单独填 sessionid")
	}
	if v := strings.TrimSpace(ttwid); v != "" {
		cookies["ttwid"] = v
	}

	name := a.accountNameFor(cookies)
	if err := a.store.PutAccount(config.Account{Name: name, Cookies: cookies, AddedAt: time.Now()}); err != nil {
		return "", err
	}
	if err := a.store.SetActive(name); err != nil {
		return name, err
	}
	a.logf("已导入账号 %s（%d 项 cookie，手动粘贴）", name, len(cookies))

	client := doubao.New(cookies)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		return name, fmt.Errorf("账号已保存，但登录校验失败：%s", doubao.FormatError(err))
	}
	if profile, err := client.FetchProfile(ctx); err == nil {
		_ = a.store.UpdateProfile(name, profile.Nickname, profile.Avatar)
		a.logf("账号 %s 资料已更新: %s", name, firstNonEmpty(profile.Nickname, "(无昵称)"))
	}
	return name, nil
}

// SaveCookies 是 ImportSession 的旧名字，保留给老前端/脚本。
func (a *App) SaveCookies(sessionID, ttwid string) error {
	_, err := a.ImportSession(sessionID, ttwid)
	return err
}

// UseAccount 设置默认账号（空字符串 = 按列表顺序取第一个）。
func (a *App) UseAccount(name string) error { return a.store.SetActive(name) }

// ReorderAccounts 保存账号列表的新顺序（拖动排序后调用）。
func (a *App) ReorderAccounts(names []string) error {
	if err := a.store.ReorderAccounts(names); err != nil {
		return err
	}
	a.logf("账号顺序已更新（生图按这个顺序排队）")
	return nil
}

// RemoveAccount 删除账号。
func (a *App) RemoveAccount(name string) error { return a.store.RemoveAccount(name) }

// ViewSession 打开一个浏览器窗口，直接显示指定账号（留空用当前账号）在豆包网页版的会话，
// 用来肉眼确认「这个账号现在到底还登着吗」。
// 它会先把配置里的 cookie 写进浏览器，所以看到的就是 MCP 正在用的那份登录态。
func (a *App) ViewSession(name string) error {
	var account config.Account
	var ok bool
	if strings.TrimSpace(name) == "" {
		account, ok = a.store.ActiveAccount()
	} else {
		account, ok = a.store.Account(name)
	}
	if !ok || account.Name == "" {
		return errors.New("还没有登录任何账号，先点「扫码登录」")
	}

	a.loginMu.Lock()
	if a.login != nil {
		a.loginMu.Unlock()
		return errors.New("扫码登录窗口还开着，先把它关掉再查看会话")
	}
	a.loginMu.Unlock()

	a.logf("打开会话窗口：账号 %s（%d 项 cookie）", account.Display(), len(account.Cookies))
	win, err := loginwin.Open(loginwin.Options{
		Title:    "当前会话 · " + account.Display(),
		Logf:     a.logf,
		ViewOnly: true,
		Cookies:  account.Cookies,
		URL:      loginwin.DefaultURL,
	})
	if err != nil {
		return err
	}

	a.loginMu.Lock()
	a.login = win
	a.loginMu.Unlock()

	// 窗口一直开着，用户关掉就算结束。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	_, err = win.WaitLogin(ctx)

	a.loginMu.Lock()
	a.login = nil
	a.loginMu.Unlock()

	if errors.Is(err, loginwin.ErrClosed) {
		a.logf("会话窗口已关闭")
		return nil
	}
	return err
}

// CheckAccount 检查当前账号登录态。
func (a *App) CheckAccount() string {
	account, ok := a.store.ActiveAccount()
	if !ok {
		return "还没有登录任何豆包账号"
	}
	client := doubao.New(account.Cookies)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		return doubao.FormatError(err)
	}
	return fmt.Sprintf("账号 %s 登录态正常", account.Name)
}

// ---------------------------------------------------------------- HTTP 服务

// HTTPRunning 返回本地 MCP HTTP 服务是否在跑。
func (a *App) HTTPRunning() bool {
	a.httpMu.Lock()
	defer a.httpMu.Unlock()
	return a.httpServer != nil
}

// StartHTTP 启动 127.0.0.1 上的 streamable HTTP MCP 服务。
func (a *App) StartHTTP() error {
	a.httpMu.Lock()
	defer a.httpMu.Unlock()
	if a.httpServer != nil {
		return errors.New("HTTP 服务已在运行")
	}
	addr := a.store.Get().HTTPAddr
	server, err := a.svc.StartHTTP(addr)
	if err != nil {
		return err
	}
	a.httpServer = server
	a.logf("MCP HTTP: http://%s/mcp", addr)
	return nil
}

// StopHTTP 停止 HTTP 服务。
func (a *App) StopHTTP() error {
	a.httpMu.Lock()
	server := a.httpServer
	a.httpServer = nil
	a.httpMu.Unlock()
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a.logf("停止 MCP HTTP 服务")
	return server.Shutdown(ctx)
}

// ---------------------------------------------------------------- 测试生图

// TestResult 是 GUI 里测一次生图的结果。
type TestResult struct {
	OK      bool                  `json:"ok"`
	Message string                `json:"message"`
	Images  []mcpserver.ImageItem `json:"images"`
}

// TestImage 用一份短 prompt 试跑一次生图，验证账号/风控/写盘链路。
// account 为空时自动取空闲账号；指定了账号就只用它（同样走排队，避免和 MCP 请求抢）。
func (a *App) TestImage(prompt, ratio, model, style, account string) (TestResult, error) {
	cfg := a.store.Get()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	acc, release, err := a.svc.Pool().AcquireNamed(ctx, strings.TrimSpace(account))
	if err != nil {
		return TestResult{Message: err.Error()}, err
	}
	defer release()

	client := doubao.New(acc.Cookies)
	client.Debug = func(format string, args ...any) { a.logf(format, args...) }
	a.logf("测试生图：使用账号 %s", acc.Display())

	res, err := client.Generate(ctx, doubao.Request{
		Prompt:   prompt,
		Ratio:    firstNonEmpty(ratio, cfg.DefaultRatio),
		Model:    firstNonEmpty(model, cfg.DefaultModel),
		Style:    style,
		OutDir:   cfg.OutDir,
		FitRatio: cfg.FitRatio,
		Timeout:  3 * time.Minute,
	})
	if err != nil {
		a.logf("测试失败: %v", err)
		return TestResult{Message: doubao.FormatError(err)}, err
	}

	out := TestResult{OK: true, Message: fmt.Sprintf("拿到 %d 张图", len(res.Images))}
	for _, img := range res.Images {
		out.Images = append(out.Images, mcpserver.ImageItem{
			Path:   img.LocalPath,
			URL:    img.URL(),
			Width:  img.Width,
			Height: img.Height,
			Format: img.Format,
		})
	}
	if res.Notice != "" {
		out.Message += "\n" + res.Notice
	}
	a.logf("测试成功: %s", out.Message)
	return out, nil
}

// exePath 返回当前可执行文件路径，界面上生成 MCP 配置时要按每台机器的实际位置显示，
// 不能写死开发机的路径。
func exePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// ---------------------------------------------------------------- 图片

// SaveImageAs 把本地图片另存到用户选的位置。
func (a *App) SaveImageAs(src string) (string, error) {
	if strings.TrimSpace(src) == "" {
		return "", errors.New("这张图没有本地文件（生成时没开落盘）")
	}
	if st, err := os.Stat(src); err != nil || st.IsDir() {
		return "", fmt.Errorf("找不到本地文件: %s", src)
	}
	target, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "图片另存为",
		DefaultFilename: filepath.Base(src),
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "图片", Pattern: "*.png;*.jpg;*.jpeg;*.webp;*.heic"},
		},
	})
	if err != nil {
		return "", err
	}
	if target == "" {
		return "", nil // 用户取消
	}
	if err := copyFile(src, target); err != nil {
		return "", err
	}
	a.logf("图片已另存为 %s", target)
	return target, nil
}

// ShowInFolder 在系统文件管理器里定位到这张图。
func (a *App) ShowInFolder(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("这张图没有本地文件")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("找不到文件: %s", path)
	}
	return revealInFolder(path)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// ---------------------------------------------------------------- 日志

// Logs 返回当前日志缓冲。
func (a *App) Logs() []string {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	out := make([]string, len(a.logs))
	copy(out, a.logs)
	return out
}

// logf 写一条日志：GUI 里显示，同时推给前端，并落到 %APPDATA%\doubao-image-mcp\app.log。
func (a *App) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	line := time.Now().Format("15:04:05") + " " + msg
	fileLog(time.Now().Format("2006-01-02 15:04:05") + " " + msg)

	a.logMu.Lock()
	a.logs = append(a.logs, line)
	if len(a.logs) > 300 {
		a.logs = a.logs[len(a.logs)-300:]
	}
	a.logMu.Unlock()

	if a.ctx == nil {
		println(line)
		return
	}
	wailsruntime.EventsEmit(a.ctx, "log", line)
}

// logFilePath 是落盘日志的位置。别人机器出问题时，让他把这个文件发回来。
func logFilePath() string { return filepath.Join(config.Dir(), "app.log") }

// webViewRuntime 返回可读的 WebView2 运行时描述。
func webViewRuntime() string {
	v := loginwin.RuntimeVersion()
	switch {
	case v == "":
		return "未检测到（登录窗口会打不开，需要安装 WebView2 Runtime）"
	case loginwin.RuntimeTooOld(v):
		return v + "（过低，登录页可能白屏）"
	default:
		return v
	}
}

var (
	fileMu   sync.Mutex
	fileSink *os.File
)

// fileLog 追加一行到日志文件；超过 2MB 就滚一个 .1 出来，避免无限增长。
func fileLog(line string) {
	fileMu.Lock()
	defer fileMu.Unlock()

	if fileSink == nil {
		if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
			return
		}
		path := logFilePath()
		if st, err := os.Stat(path); err == nil && st.Size() > 2<<20 {
			_ = os.Rename(path, path+".1")
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		fileSink = f
	}
	fmt.Fprintln(fileSink, line)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
