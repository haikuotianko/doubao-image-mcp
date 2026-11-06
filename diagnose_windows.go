//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// platformOSInfo 读注册表里的 Windows 版本。
func platformOSInfo() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "Windows（版本读取失败）"
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

// proxySystemInfo 读 IE / 系统代理设置。
func proxySystemInfo() string {
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
