package doubao

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Image 是一张生成结果。
type Image struct {
	Key      string `json:"key,omitempty"`
	ThumbURL string `json:"thumbUrl,omitempty"`
	OriURL   string `json:"oriUrl,omitempty"`
	RawURL   string `json:"rawUrl,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Format   string `json:"format,omitempty"`
	// LocalPath 是落盘后的绝对路径。
	LocalPath string `json:"localPath,omitempty"`
}

// Result 是一次生图的完整结果。
type Result struct {
	Prompt string  `json:"prompt"`
	Model  string  `json:"model"`
	Ratio  string  `json:"ratio"`
	Images []Image `json:"images"`
	// Notice 是服务端提示（如订阅墙），非空且无图时代表没出图。
	Notice string `json:"notice,omitempty"`
	// Channel 记录实际生效的通道：ability / samantha。
	Channel string `json:"channel"`
}

// Request 是一次生图请求。
type Request struct {
	Prompt string
	Ratio  string
	Model  string
	Style  string
	// RefImages 是本地参考图路径，非空时走图生图通道。
	RefImages []string
	// OutDir 为空时不落盘，只返回 URL。
	OutDir string
	// FitRatio 为真时把结果裁剪/补边到请求比例。
	FitRatio bool
	// Timeout 覆盖默认超时。
	Timeout time.Duration
}

// Client 是一个豆包账号的调用器，并发安全（不含可变状态）。
type Client struct {
	cookie  string
	cookies map[string]string
	http    *http.Client
	// Debug 非空时接收协议日志。
	Debug func(format string, args ...any)
}

// New 用 cookie 键值构造客户端。
func New(cookies map[string]string) *Client {
	keys := make([]string, 0, len(cookies))
	for k, v := range cookies {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+cookies[k])
	}
	return &Client{
		cookie:  strings.Join(parts, "; "),
		cookies: cookies,
		http:    &http.Client{Timeout: 0},
	}
}

func (c *Client) logf(format string, args ...any) {
	if c.Debug != nil {
		c.Debug(format, args...)
	}
}

// commonHeaders 是网页版请求的固定头。
func (c *Client) commonHeaders() http.Header {
	h := http.Header{}
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/"+chromeVersion+" Safari/537.36")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9")
	h.Set("Origin", BaseURL)
	h.Set("Referer", BaseURL+"/chat")
	if c.cookie != "" {
		h.Set("Cookie", c.cookie)
	}
	return h
}

// newEndpointParams 是 /chat/completion 与 /samantha 系列的安全参数（抓包真值）。
func newEndpointParams() map[string]string {
	return map[string]string{
		"aid":                    "497858",
		"real_aid":               "497858",
		"device_id":              defaultDevice,
		"tea_uuid":               defaultDevice,
		"web_id":                 defaultDevice,
		"device_platform":        "web",
		"doubao_device_platform": "web",
		"doubao_pc_version":      "3.41.4",
		"language":               "zh",
		"region":                 "CN",
		"sys_region":             "CN",
		"pkg_type":               "release_version",
		"version_code":           "20800",
		"pc_version":             "3.41.4",
		"chromium_version":       chromiumBuild,
		"client_platform":        "pc_client",
		"runtime":                "web",
		"runtime_version":        "3.5.4",
		"samantha_web":           "1",
		"use-olympus-account":    "1",
		"fp":                     "verify_mvfb2gzc_nST1AVAS_3Mpe_4uAR_8WAF_mxtjs",
		"web_tab_id":             uuid4(),
	}
}

// samanthaParams 是旧 Samanta 通道的参数（aid=582478，另一套设备指纹）。
func samanthaParams() map[string]string {
	return map[string]string{
		"aid":                 "582478",
		"real_aid":            "582478",
		"device_id":           "714003710229497",
		"tea_uuid":            "714003710229497",
		"web_id":              "7604137868021548590",
		"device_platform":     "web",
		"language":            "zh",
		"region":              "CN",
		"sys_region":          "CN",
		"pkg_type":            "release_version",
		"version_code":        "20800",
		"pc_version":          "2.1.7",
		"chromium_version":    chromiumBuild,
		"client_platform":     "pc_client",
		"runtime":             "web",
		"runtime_version":     "3.5.4",
		"samantha_web":        "1",
		"use-olympus-account": "1",
		"fp":                  "verify_mlcfw5f7_TPq0YmFD_NrsC_4RuQ_BJPg_M5W7i58I7wV0",
		"web_tab_id":          uuid4(),
	}
}

// buildURL 拼接带排序 query 的 URL。
func buildURL(path string, params map[string]string) string {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	return BaseURL + path + "?" + values.Encode()
}

// postJSON 发一个 JSON POST，返回状态码和响应体。
func (c *Client) postJSON(ctx context.Context, rawURL string, body any, timeout time.Duration) (int, []byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(string(data)))
	if err != nil {
		return 0, nil, err
	}
	req.Header = c.commonHeaders()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Agw-Js-Conv", "str")
	req.Header.Set("Referer", BaseURL+"/chat/")

	c.logf("POST %s (%d bytes)", rawURL, len(data))
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, &Error{Kind: KindOther, Op: "请求豆包", Msg: "网络错误", Detail: err.Error()}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, &Error{Kind: KindOther, Op: "读取响应", Msg: "读取中断", Detail: err.Error()}
	}
	c.logf("← %d (%d bytes)", resp.StatusCode, len(respBody))
	return resp.StatusCode, respBody, nil
}

func (c *Client) postBody(ctx context.Context, rawURL string, body any, timeout time.Duration) ([]byte, error) {
	status, respBody, err := c.postJSON(ctx, rawURL, body, timeout)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, newErr(classifyCode(extractCode(respBody)), "请求豆包",
			extractCode(respBody), fmt.Sprintf("HTTP %d", status), string(respBody))
	}
	return respBody, nil
}

// extractCode 尝试从响应体里取 error_code / code。
func extractCode(body []byte) string {
	m := jsonObject(string(body))
	if m == nil {
		return ""
	}
	for _, key := range []string{"error_code", "code"} {
		switch v := m[key].(type) {
		case float64:
			return fmt.Sprintf("%.0f", v)
		case string:
			return v
		}
	}
	return ""
}

// Ping 用一次轻量的会话列表请求验证登录态与 IP 风控状态。
func (c *Client) Ping(ctx context.Context) error {
	body, err := c.postBody(ctx, buildURL("/samantha/im/conversation/list", samanthaParams()), map[string]any{}, 30*time.Second)
	if err != nil {
		return err
	}
	m := jsonObject(string(body))
	if m == nil {
		return newErr(KindOther, "登录检查", "", "响应不是 JSON", string(body))
	}
	if code := asInt(m["code"]); code != 0 {
		kind := classifyCode(fmt.Sprintf("%d", code))
		if kind == KindOther {
			kind = KindLogin
		}
		return newErr(kind, "登录检查", fmt.Sprintf("%d", code), asString(m["msg"]), string(body))
	}
	return nil
}

// Generate 执行一次生图：文生图走新端点，带参考图或新端点失败时走旧通道。
func (c *Client) Generate(ctx context.Context, req Request) (*Result, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("prompt 不能为空")
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	result, err := c.generate(ctx, req, timeout)
	if err != nil {
		return nil, err
	}
	if req.OutDir != "" {
		if err := c.saveImages(ctx, result, req); err != nil {
			return result, err
		}
	}
	return result, nil
}

// generate 只负责拿到图片 URL，不落盘。
func (c *Client) generate(ctx context.Context, req Request, timeout time.Duration) (*Result, error) {
	if len(req.RefImages) > 0 {
		return c.generateImageToImage(ctx, req, timeout)
	}
	res, err := c.generateTextToImage(ctx, req, timeout)
	if err == nil {
		return res, nil
	}
	// 风控和登录问题不降级，直接报给用户。
	if IsKind(err, KindRisk) || IsKind(err, KindLogin) || IsKind(err, KindQuota) {
		return nil, err
	}
	c.logf("新端点失败，降级到旧通道：%v", err)
	res, err2 := c.generateSamantha(ctx, req, timeout)
	if err2 != nil {
		return nil, fmt.Errorf("新端点失败（%v），旧通道也失败（%w）", err, err2)
	}
	res.Channel = "samantha(fallback)"
	return res, nil
}

// saveImages 下载并发落盘，可选做比例修正。
func (c *Client) saveImages(ctx context.Context, res *Result, req Request) error {
	if err := os.MkdirAll(req.OutDir, 0o755); err != nil {
		return err
	}
	stamp := time.Now().Format("20060102_150405")
	for i := range res.Images {
		img := &res.Images[i]
		src := img.imageURL()
		if src == "" {
			continue
		}
		data, contentType, err := c.download(ctx, src)
		if err != nil {
			return err
		}
		ext := extFor(contentType, img.Format)
		name := fmt.Sprintf("doubao_%s_%d%s", stamp, i+1, ext)
		path := filepath.Join(req.OutDir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
		img.LocalPath = path
		// 服务端给的宽高是原图尺寸，比例修正后真实尺寸会变，以落盘文件为准。
		if cfg, _, derr := image.DecodeConfig(bytes.NewReader(data)); derr == nil {
			img.Width, img.Height = cfg.Width, cfg.Height
		}
		if req.FitRatio && req.Ratio != "" {
			if fixed, ferr := FitRatio(path, req.Ratio); ferr == nil && fixed != path {
				img.LocalPath = fixed
			}
		}
	}
	return nil
}

// download 取图：CDN 直链不需要 cookie，但要带 Referer。
func (c *Client) download(ctx context.Context, rawURL string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	h := c.commonHeaders()
	h.Del("Accept")
	h.Set("Referer", BaseURL+"/chat/")
	req.Header = h

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", &Error{Kind: KindOther, Op: "下载图片", Msg: "网络错误", Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", newErr(KindOther, "下载图片", "", fmt.Sprintf("HTTP %d", resp.StatusCode), rawURL)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// extFor 推断落盘扩展名。
func extFor(contentType, format string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		return ".jpg"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "png"):
		return ".png"
	}
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return ".jpg"
	case "webp":
		return ".webp"
	}
	return ".png"
}

// uuid4 生成一个 v4 UUID（不需要密码学强度，但要看起来像前端）。
func uuid4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
