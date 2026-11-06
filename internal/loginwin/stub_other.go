//go:build !windows

// Package loginwin 在非 Windows 平台提供同名的空实现：
// 内置扫码登录窗口依赖 Win32 + WebView2，其它平台没有对应实现，
// 但 MCP 主体（协议客户端 / 工具 / 账号池）都是纯 Go，照样能用——
// 只是登录要改成手动导入 sessionid（-set-session 或 GUI 里的输入框）。
package loginwin

import (
	"context"
	"errors"
	"time"
)

// DefaultURL 是登录入口（只有 Windows 的内置浏览器会用）。
const DefaultURL = "https://www.doubao.com/chat/"

// MinRuntimeVersion 是 Windows 上 WebView2 的最低版本要求（其它平台不适用）。
const MinRuntimeVersion = "100.0.0.0"

// ErrNotSupported 表示当前平台没有内置扫码登录窗口。
var ErrNotSupported = errors.New("当前平台没有内置扫码登录：请用 `-set-session <sessionid>` 或 GUI 里的「手动导入」")

// ErrClosed 与 Windows 版同名，表示窗口在登录完成前被关掉（这里只用于错误判断）。
var ErrClosed = errors.New("登录窗口已关闭")

// Options 与 Windows 版保持一致的字段，方便上层代码不分平台。
type Options struct {
	Title          string
	Width          int
	Height         int
	DataPath       string
	URL            string
	Logf           func(format string, args ...any)
	PollInterval   time.Duration
	ViewOnly       bool
	Cookies        map[string]string
	ClearCookies   bool
	SkipSessions   map[string]string
}

// Window 是空实现，只保证接口存在。
type Window struct{}

// Supported 表示当前平台是否支持内置扫码登录。
func Supported() bool { return false }

// RuntimeVersion 在非 Windows 上没有意义，返回空串。
func RuntimeVersion() string { return "" }

// RuntimeTooOld 在非 Windows 上恒为 false。
func RuntimeTooOld(string) bool { return false }

// Open 直接返回不支持。
func Open(Options) (*Window, error) { return nil, ErrNotSupported }

// WaitLogin 直接返回不支持。
func (w *Window) WaitLogin(context.Context) (map[string]string, error) { return nil, ErrNotSupported }

// Close 什么也不做。
func (w *Window) Close() {}
