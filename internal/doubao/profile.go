package doubao

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Profile 是豆包账号的展示资料。
type Profile struct {
	Nickname string
	// Avatar 是 data URL（头像 CDN 带时效，存链接会过期，存图本体才稳）。
	Avatar string
}

// FetchProfile 尽力拿昵称与头像。
//
// 两步走：先问 passport 接口，拿不到再翻聊天页里的 SSR 数据。
// 都失败也不算致命错误——界面会用账号标识当名字、用首字母当头像。
func (c *Client) FetchProfile(ctx context.Context) (Profile, error) {
	var p Profile
	if v, err := c.fetchProfileAPI(ctx); err == nil {
		p = v
	}
	if p.Nickname == "" || p.Avatar == "" {
		if v, err := c.fetchProfilePage(ctx); err == nil {
			if p.Nickname == "" {
				p.Nickname = v.Nickname
			}
			if p.Avatar == "" {
				p.Avatar = v.Avatar
			}
		}
	}
	if strings.HasPrefix(p.Avatar, "http") {
		if data, err := c.fetchAvatar(ctx, p.Avatar); err == nil {
			p.Avatar = data
		} else {
			c.logf("下载头像失败: %v", err)
		}
	}
	if p.Nickname == "" && p.Avatar == "" {
		return p, fmt.Errorf("没能获取到账号资料")
	}
	return p, nil
}

// fetchProfileAPI 走 passport 接口（未登录时它会返回 session_expired）。
func (c *Client) fetchProfileAPI(ctx context.Context) (Profile, error) {
	url := buildURL("/passport/account/info", newEndpointParams())
	status, body, err := c.postJSON(ctx, url, map[string]any{}, 15*time.Second)
	if err != nil {
		return Profile{}, err
	}
	if status != http.StatusOK {
		return Profile{}, fmt.Errorf("HTTP %d", status)
	}
	nickname, avatar := scanProfileFields(body)
	if nickname == "" && avatar == "" {
		return Profile{}, fmt.Errorf("响应里没有资料字段")
	}
	return Profile{Nickname: nickname, Avatar: avatar}, nil
}

var (
	nicknameKeys = []string{"nickname", "nick_name", "user_name", "screen_name", "user_nickname", "display_name"}
	avatarKeys   = []string{"avatar_url", "avatar_thumb", "avatar_uri", "avatar", "avatar_big"}
	pageNickRe   = regexp.MustCompile(`"(?:nickname|nick_name|user_name|screen_name)"\s*:\s*"([^"]{1,40})"`)
	pageAvatarRe = regexp.MustCompile(`"(?:avatar_url|avatar_thumb|avatar_big|avatar_uri)"\s*:\s*"(https?://[^"]+)"`)
)

// scanProfileFields 在任意 JSON 结构里找昵称和头像，字段名容忍几种常见写法。
func scanProfileFields(body []byte) (nickname, avatar string) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return "", ""
	}
	var walk func(v any)
	walk = func(v any) {
		switch node := v.(type) {
		case map[string]any:
			for k, val := range node {
				key := strings.ToLower(k)
				if s, ok := val.(string); ok && s != "" {
					if nickname == "" && contains(nicknameKeys, key) && len([]rune(s)) <= 40 {
						nickname = s
					}
					if avatar == "" && contains(avatarKeys, key) && looksLikeAvatar(s) {
						avatar = s
					}
				}
				walk(val)
			}
		case []any:
			for _, item := range node {
				walk(item)
			}
		}
	}
	walk(root)
	return nickname, avatar
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func looksLikeAvatar(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "//") || strings.HasPrefix(s, "data:image/")
}

// fetchProfilePage 抓聊天页 HTML，从里面的 SSR 数据里抠昵称和头像。
func (c *Client) fetchProfilePage(ctx context.Context) (Profile, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+"/chat/", nil)
	if err != nil {
		return Profile{}, err
	}
	req.Header = c.commonHeaders()
	req.Header.Set("Accept", "text/html")
	resp, err := c.http.Do(req)
	if err != nil {
		return Profile{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Profile{}, err
	}
	html := string(body)

	var p Profile
	if m := pageNickRe.FindStringSubmatch(html); m != nil {
		p.Nickname = m[1]
	}
	if m := pageAvatarRe.FindStringSubmatch(html); m != nil {
		p.Avatar = m[1]
	}
	if p.Nickname == "" && p.Avatar == "" {
		return p, fmt.Errorf("页面里没有资料字段")
	}
	return p, nil
}

// fetchAvatar 下载头像并转成 data URL（超过 512KB 就不要了，头像不该有这么大）。
func (c *Client) fetchAvatar(ctx context.Context, url string) (string, error) {
	if strings.HasPrefix(url, "//") {
		url = "https:" + url
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header = c.commonHeaders()
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("空文件")
	}
	mime := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/png"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
