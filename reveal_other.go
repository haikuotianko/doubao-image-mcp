//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// revealInFolder 在访达 / 文件管理器里定位到这个文件。
func revealInFolder(path string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", path).Start()
	}
	// Linux 没有统一的「选中文件」，退化成打开所在目录。
	return exec.Command("xdg-open", filepath.Dir(path)).Start()
}

// openFolder 用系统文件管理器打开目录。
func openFolder(dir string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", dir).Start()
	}
	return exec.Command("xdg-open", dir).Start()
}
