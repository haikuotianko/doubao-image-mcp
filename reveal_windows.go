//go:build windows

package main

import "os/exec"

// revealInFolder 在资源管理器里选中这个文件。
func revealInFolder(path string) error {
	return exec.Command("explorer", "/select,"+path).Start()
}

// openFolder 用资源管理器打开目录。
func openFolder(dir string) error {
	return exec.Command("explorer", dir).Start()
}
