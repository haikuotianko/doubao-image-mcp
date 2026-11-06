package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"doubao-image-mcp/internal/config"
	"doubao-image-mcp/internal/loginwin"
)

// runDiagnose 打一份环境自检报告：别人机器上「登录窗口白屏」时，
// 让对方跑 doubao-image-mcp.exe -diagnose，把报告发回来就能判断是运行时、代理还是网络。
// quiet 为真时不弹提示框（脚本/自动化用）。
func runDiagnose(quiet bool) {
	attachConsole()

	var b strings.Builder
	say := func(format string, args ...any) {
		s := fmt.Sprintf(format, args...)
		fmt.Println(s)
		b.WriteString(s)
		b.WriteString("\r\n")
	}

	say("豆包生图 MCP 环境自检  %s", time.Now().Format("2006-01-02 15:04:05"))
	say("Go: %s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	say("Windows: %s", windowsInfo())
	say("程序路径: %s", exePath())
	say("stdio 配置示例:\n%s", stdioConfig())

	ver := loginwin.RuntimeVersion()
	switch {
	case ver == "":
		say("WebView2 运行时: 未检测到（登录窗口一定打不开，需安装 Microsoft Edge WebView2 Runtime）")
	case loginwin.RuntimeTooOld(ver):
		say("WebView2 运行时: %s（低于 %s，豆包登录页可能白屏，请更新）", ver, loginwin.MinRuntimeVersion)
	default:
		say("WebView2 运行时: %s", ver)
	}

	dir := config.Dir()
	say("配置目录: %s  可写=%v", dir, dirWritable(dir))
	say("WebView2 数据目录: %s  可写=%v", filepath.Join(dir, "webview"), dirWritable(filepath.Join(dir, "webview")))

	say("代理(环境变量): %s", envProxy())
	say("代理(IE 设置): %s", ieProxy())

	say("DNS www.doubao.com: %s", dnsCheck("www.doubao.com"))
	say("连接 www.doubao.com:443: %s", dialCheck("www.doubao.com:443"))
	say("GET https://www.doubao.com/chat/ : %s", httpsCheck("https://www.doubao.com/chat/"))

	report := filepath.Join(dir, "diag-"+time.Now().Format("20060102-150405")+".txt")
	if err := os.MkdirAll(dir, 0o755); err == nil {
		// 带 BOM 存，记事本/PowerShell 5.1 才不会把中文显示成乱码。
		data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(b.String())...)
		if err := os.WriteFile(report, data, 0o644); err != nil {
			say("写报告失败: %v", err)
			report = ""
		}
	}

	if report != "" {
		fmt.Println("\n报告已保存:", report)
		if !quiet {
			messageBox("环境自检完成", "报告已保存到：\r\n"+report+"\r\n\r\n把它发给开发者即可。\r\n\r\n若这里提示 WebView2 运行时缺失或过旧，请先安装/更新 Microsoft Edge WebView2 Runtime。")
		}
	}
}

// stdioConfig 生成与界面一致、按当前机器实际路径填好的 MCP stdio 配置。
func stdioConfig() string {
	type server struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	cfg := struct {
		MCPServers map[string]server `json:"mcpServers"`
	}{MCPServers: map[string]server{"doubao": {Command: exePath(), Args: []string{"-mcp"}}}}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "生成失败: " + err.Error()
	}
	return string(data)
}

// windowsInfo 读注册表里的系统版本。
func windowsInfo() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "读取失败"
	}
	defer k.Close()
	get := func(name string) string {
		v, _, err := k.GetStringValue(name)
		if err != nil {
			return "?"
		}
		return v
	}
	ubr, _, _ := k.GetIntegerValue("UBR")
	return fmt.Sprintf("%s %s (build %s.%d)", get("ProductName"), get("DisplayVersion"), get("CurrentBuildNumber"), ubr)
}

// dirWritable 检查目录能不能创建并写入。
func dirWritable(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".wtest")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func envProxy() string {
	var parts []string
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"} {
		if v := os.Getenv(name); v != "" {
			parts = append(parts, name+"="+v)
		}
	}
	if len(parts) == 0 {
		return "无"
	}
	return strings.Join(parts, ", ")
}

func ieProxy() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return "读取失败"
	}
	defer k.Close()
	on, _, _ := k.GetIntegerValue("ProxyEnable")
	server, _, _ := k.GetStringValue("ProxyServer")
	script, _, _ := k.GetStringValue("AutoConfigURL")
	if on == 0 && script == "" {
		return "未启用"
	}
	out := fmt.Sprintf("启用=%d ProxyServer=%s", on, server)
	if script != "" {
		out += " AutoConfigURL=" + script
	}
	return out
}

func dnsCheck(host string) string {
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Sprintf("解析失败 (%v)", err)
	}
	if len(addrs) > 3 {
		addrs = addrs[:3]
	}
	return strings.Join(addrs, ", ")
}

func dialCheck(host string) string {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", host, 8*time.Second)
	if err != nil {
		return fmt.Sprintf("失败 (%v)", err)
	}
	conn.Close()
	return fmt.Sprintf("成功 (%dms)", time.Since(start).Milliseconds())
}

func httpsCheck(url string) string {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Sprintf("失败 (%v)", err)
	}
	defer resp.Body.Close()
	return fmt.Sprintf("HTTP %s", resp.Status)
}

// attachConsole 让 GUI 子系统的程序也能把输出打到调用它的控制台。
func attachConsole() {
	k32 := windows.NewLazySystemDLL("kernel32.dll")
	const attachParentProcess = 0xFFFFFFFF
	k32.NewProc("AttachConsole").Call(attachParentProcess)
}

func messageBox(title, text string) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	const mbIconInformation = 0x00000040
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), mbIconInformation)
}
