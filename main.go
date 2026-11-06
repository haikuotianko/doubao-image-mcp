package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"doubao-image-mcp/internal/config"
	"doubao-image-mcp/internal/loginwin"
	"doubao-image-mcp/internal/mcpserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	stdioMode := flag.Bool("mcp", false, "以 MCP stdio 模式运行（供 MCP 客户端直接拉起，不开 GUI）")
	httpMode := flag.String("http", "", "只启动 MCP HTTP 服务（不开 GUI），可指定监听地址如 127.0.0.1:8770")
	diagMode := flag.Bool("diagnose", false, "打印环境自检报告（排查登录窗口白屏用）")
	quietMode := flag.Bool("quiet", false, "自检时不弹提示框（脚本用）")
	loginMode := flag.Bool("login", false, "只打开扫码登录窗口（不启动主界面，排查用）")
	loginURL := flag.String("login-url", "", "登录窗口打开的地址（默认豆包对话页，排查用）")
	viewMode := flag.Bool("view", false, "用当前账号的会话打开浏览器窗口（等同 GUI 的「查看会话」）")
	flag.Parse()

	if *diagMode {
		runDiagnose(*quietMode)
		return
	}
	if *loginMode {
		runLoginOnly(*loginURL)
		return
	}
	if *viewMode {
		runViewOnly()
		return
	}
	if *stdioMode {
		runStdio()
		return
	}
	if *httpMode != "" {
		runHTTP(*httpMode)
		return
	}
	runGUI()
}

// runViewOnly 用配置里当前账号的 cookie 打开一个浏览窗口，等同于 GUI 的「查看会话」。
func runViewOnly() {
	logf := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s %s\n", time.Now().Format("15:04:05"), line)
		fileLog(time.Now().Format("2006-01-02 15:04:05") + " " + line)
	}
	store, err := config.Load()
	if err != nil {
		logf("读取配置失败: %v", err)
	}
	account, ok := store.ActiveAccount()
	if !ok {
		logf("还没有登录任何账号，先在 GUI 里扫码登录")
		return
	}
	logf("用账号 %s 的会话打开窗口（%d 项 cookie）", account.Name, len(account.Cookies))
	win, err := loginwin.Open(loginwin.Options{
		Title:    "当前会话 · " + account.Name,
		Logf:     logf,
		ViewOnly: true,
		Cookies:  account.Cookies,
	})
	if err != nil {
		logf("打开会话窗口失败: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := win.WaitLogin(ctx); err != nil {
		logf("会话窗口结束: %v", err)
	}
}

// runLoginOnly 只开登录窗口。日志同时落到 %APPDATA%\doubao-image-mcp\app.log，
// 别人机器白屏时让他用这个模式跑一次，日志里就有加载结果。
func runLoginOnly(url string) {
	logf := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s %s\n", time.Now().Format("15:04:05"), line)
		fileLog(time.Now().Format("2006-01-02 15:04:05") + " " + line)
	}
	logf("登录窗口自检模式启动（WebView2 运行时 %s）", loginwin.RuntimeVersion())
	skip := make(map[string]string)
	if store, err := config.Load(); err == nil {
		for _, acc := range store.Get().Accounts {
			if sid := acc.SessionID(); sid != "" {
				skip[sid] = acc.Display()
			}
		}
	}
	win, err := loginwin.Open(loginwin.Options{
		Logf:         logf,
		URL:          url,
		ClearCookies: true,
		SkipSessions: skip,
	})
	if err != nil {
		logf("打开登录窗口失败: %v", err)
		messageBox("登录窗口打不开", err.Error()+"\r\n\r\n详细日志："+logFilePath())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cookies, err := win.WaitLogin(ctx)
	if err != nil {
		logf("登录未完成: %v", err)
		return
	}
	names := make([]string, 0, len(cookies))
	for k := range cookies {
		names = append(names, k)
	}
	logf("已取到登录 cookie: %v", names)
}

// runHTTP 只跑 HTTP 服务，方便把本机当常驻 MCP 服务用。
func runHTTP(addr string) {
	store, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
	}
	if addr == "" || addr == "default" {
		addr = store.Get().HTTPAddr
	}
	logf := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, time.Now().Format("15:04:05")+" "+format+"\n", args...)
	}
	svc := mcpserver.New(store, logf)
	if _, err := svc.StartHTTP(addr); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "MCP HTTP 已在 http://%s/mcp 监听，Ctrl+C 退出\n", addr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}

// runStdio 走 stdio 传输。注意：日志必须写 stderr，stdout 只能走 JSON-RPC。
func runStdio() {
	store, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
	}
	svc := mcpserver.New(store, func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, time.Now().Format("15:04:05")+" "+format+"\n", args...)
		if logFile := stdioLogFile(); logFile != nil {
			fmt.Fprintf(logFile, time.Now().Format("2006-01-02 15:04:05")+" "+format+"\n", args...)
		}
	})
	if err := svc.ServeStdio(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "MCP stdio 退出: %v\n", err)
		os.Exit(1)
	}
}

var logFileHandle *os.File

func stdioLogFile() *os.File {
	if logFileHandle != nil {
		return logFileHandle
	}
	path := filepath.Join(config.Dir(), "mcp.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil
	}
	logFileHandle = f
	return f
}

func runGUI() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "豆包生图 MCP",
		Width:  1080,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
