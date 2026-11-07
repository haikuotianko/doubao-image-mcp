// Package update 用 GitHub Releases 做版本更新检查，不需要自建服务器。
//
// 原理：仓库每次发版都会有一个 tag（如 v1.2.0）和对应的 Release。
// 客户端请求 GitHub 的「最新 Release」接口，取出 tag_name 与当前编译进去的版本号比较：
//
//	GET https://api.github.com/repos/<owner>/<repo>/releases/latest
//
// 未认证请求每小时 60 次（按 IP 计），对于「启动时查一次」的用法绰绰有余。
// 想彻底避开限流，也可以把版本信息放进仓库里的一个静态 JSON，
// 用 raw.githubusercontent.com 读取（不限流、可 CDN 缓存）。
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// Repo 是版本发布所在的仓库。
	Repo = "haikuotianko/doubao-image-mcp"
	// ReleasesPage 是给人看的下载页。
	ReleasesPage = "https://github.com/" + Repo + "/releases/latest"
	// DownloadPattern 拼出某个资产的直链（客户端自动更新时用）。
	DownloadPattern = "https://github.com/" + Repo + "/releases/latest/download/"

	apiLatest = "https://api.github.com/repos/" + Repo + "/releases/latest"
)

// Asset 是 Release 里的一个附件。
type Asset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"browser_download_url"`
}

// Release 是 GitHub 上的一个版本。
type Release struct {
	Tag         string    `json:"tag_name"`
	Name        string    `json:"name"`
	URL         string    `json:"html_url"`
	Notes       string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []Asset   `json:"assets"`
}

// Result 是「检查更新」的结果，字段直接给界面/命令行用。
type Result struct {
	Current   string `json:"current"`   // 当前版本
	Latest    string `json:"latest"`    // 线上最新版本
	Newer     bool   `json:"newer"`     // 是否有新版本
	URL       string `json:"url"`       // 下载页地址
	Name      string `json:"name"`      // Release 标题
	Published string `json:"published"` // 发布时间（本地时区，2006-01-02）
	Notes     string `json:"notes"`     // 更新说明（Markdown）
}

// Latest 拉取仓库最新 Release。
func Latest(ctx context.Context) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiLatest, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "doubao-image-mcp-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连不上 GitHub：%w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("仓库 %s 还没有发布任何 Release", Repo)
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, fmt.Errorf("GitHub 接口限流（未认证每小时 60 次），过一会儿再试")
	default:
		return nil, fmt.Errorf("GitHub 返回 %s", resp.Status)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("解析 GitHub 返回失败：%w", err)
	}
	if rel.URL == "" {
		rel.URL = ReleasesPage
	}
	return &rel, nil
}

// Check 拉取最新版本并与 current 比较。
func Check(ctx context.Context, current string) (Result, error) {
	res := Result{Current: current, URL: ReleasesPage}
	rel, err := Latest(ctx)
	if err != nil {
		return res, err
	}
	res.Latest = rel.Tag
	res.Newer = IsNewer(current, rel.Tag)
	res.URL = rel.URL
	res.Name = rel.Name
	res.Notes = rel.Notes
	if !rel.PublishedAt.IsZero() {
		res.Published = rel.PublishedAt.Local().Format("2006-01-02")
	}
	return res, nil
}

// IsNewer 判断 latest 是否比 current 新。
// current 为空或 dev（本地构建没注入版本号）时永远返回 false，避免开发版一直提示更新。
func IsNewer(current, latest string) bool {
	cur, ok := parse(current)
	if !ok {
		return false
	}
	next, ok := parse(latest)
	if !ok {
		return false
	}
	for i := range cur.num {
		if next.num[i] != cur.num[i] {
			return next.num[i] > cur.num[i]
		}
	}
	// 数字部分相同：正式版 > 预发布版（v1.2.0 比 v1.2.0-beta 新）
	return next.pre == "" && cur.pre != ""
}

type semver struct {
	num [3]int
	pre string // 预发布标记，如 beta.1；空表示正式版
}

// parse 解析 v1.2.3 / 1.2 / v1.2.3-beta.1 这类写法。
func parse(v string) (semver, bool) {
	var sv semver
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if s == "" || s == "dev" {
		return sv, false
	}
	if i := strings.IndexByte(s, '+'); i >= 0 { // 去掉 build metadata
		s = s[:i]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		sv.pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return sv, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return sv, false
		}
		sv.num[i] = n
	}
	return sv, true
}
