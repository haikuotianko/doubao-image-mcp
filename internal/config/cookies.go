package config

import (
	"regexp"
	"strings"
)

var (
	curlHeaderRe = regexp.MustCompile(`(?i)(?:-H|--header)\s+(?:'([^']*)'|"([^"]*)")`)
	curlCookieRe = regexp.MustCompile(`(?i)(?:-b|--cookie)\s+(?:'([^']*)'|"([^"]*)")`)
)

// ParseCookies 把用户粘贴的内容解析成 cookie 表。
//
// 支持三种常见形态，都是「登录豆包网页版后从浏览器里复制」能直接拿到的：
//
//	sessionid=7f3c...; ttwid=1%7C...; s_v_web_id=...     ← Cookie 请求头
//	7f3c...                                              ← 只有 sessionid
//	curl 'https://www.doubao.com/...' -H 'cookie: ...'   ← DevTools 的 Copy as cURL
//
// 这样用户不用在几十个 cookie 里翻 sessionid，整段复制粘进来就行。
func ParseCookies(input string) map[string]string {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil
	}

	// 1) cURL 命令：把 cookie 头或 -b 参数抠出来
	if strings.Contains(strings.ToLower(s), "curl") || strings.Contains(s, "-H ") || strings.Contains(s, "-b ") {
		if v := firstHeaderCookie(s); v != "" {
			s = v
		} else if v := firstCurlCookie(s); v != "" {
			s = v
		}
	}

	// 2) 去掉可能残留的 "Cookie:" 前缀
	if len(s) >= 7 && strings.EqualFold(s[:7], "cookie:") {
		s = strings.TrimSpace(s[7:])
	}
	s = strings.Trim(s, `'"`)

	out := make(map[string]string)
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || value == "" {
			continue
		}
		out[name] = value
	}

	// 3) 裸 sessionid：没有等号，就当整串是 sessionid
	if len(out) == 0 && !strings.Contains(s, "=") {
		out["sessionid"] = s
	}
	return out
}

// firstHeaderCookie 从 cURL 的 -H/--header 里找 cookie 那条。
func firstHeaderCookie(s string) string {
	for _, m := range curlHeaderRe.FindAllStringSubmatch(s, -1) {
		v := m[1]
		if v == "" {
			v = m[2]
		}
		if len(v) >= 7 && strings.EqualFold(v[:7], "cookie:") {
			return strings.TrimSpace(v[7:])
		}
	}
	return ""
}

// firstCurlCookie 从 cURL 的 -b/--cookie 参数里取。
func firstCurlCookie(s string) string {
	if m := curlCookieRe.FindStringSubmatch(s); m != nil {
		if m[1] != "" {
			return m[1]
		}
		return m[2]
	}
	return ""
}
