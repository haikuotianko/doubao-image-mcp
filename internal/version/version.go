// Package version 只保存构建时注入的版本号。
//
// 发布时由打包脚本注入：
//
//	go build -ldflags "-X doubao-image-mcp/internal/version.Version=v1.2.0"
//
// 本地 `go build ./...` 不注入，值为 dev，此时不提示更新。
package version

// Version 是当前程序的版本号，形如 v1.1.0。默认 dev 表示开发构建。
var Version = "dev"

// Current 返回当前版本号。
func Current() string { return Version }

// IsDev 表示这是没注入版本号的开发构建。
func IsDev() bool { return Version == "" || Version == "dev" }
