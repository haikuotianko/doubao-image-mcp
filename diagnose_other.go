//go:build !windows

package main

import (
	"fmt"
	"os"
	"runtime"
)

// platformOSInfo 在非 Windows 上给出系统与架构（macOS / Linux 都用这个）。
func platformOSInfo() string {
	name := runtime.GOOS
	switch runtime.GOOS {
	case "darwin":
		name = "macOS"
	case "linux":
		name = "Linux"
	}
	host, _ := os.Hostname()
	return fmt.Sprintf("%s %s（主机 %s）", name, runtime.GOARCH, host)
}

// proxySystemInfo 在非 Windows 上只认环境变量（GUI 启动时可能读不到 shell 的代理）。
func proxySystemInfo() string {
	return "不适用（非 Windows，只看环境的 HTTP_PROXY/HTTPS_PROXY）"
}

// attachConsole 在非 Windows 上没有「GUI 子系统」概念，控制台输出直接可见。
func attachConsole() {}

// messageBox 在非 Windows 上退化成往 stdout 打一行，不弹窗。
func messageBox(title, text string) {
	fmt.Printf("\n[%s]\n%s\n", title, text)
}
