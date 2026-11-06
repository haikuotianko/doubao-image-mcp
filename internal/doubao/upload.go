package doubao

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// csrfToken 取 passport csrf token，上传图片时必须带在 x-tt-passport-csrf-token 头里。
func (c *Client) csrfToken() string {
	for _, key := range []string{"passport_csrf_token", "passport_csrf_token_default"} {
		if v := strings.TrimSpace(c.cookies[key]); v != "" {
			return v
		}
	}
	return ""
}

// UploadImage 把本地图片传成豆包文件（返回 uri，可作为 attachment 的 key）。
func (c *Client) UploadImage(ctx context.Context, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取参考图失败: %w", err)
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "" {
		ext = "png"
	}
	contentType := mime.TypeByExtension("." + ext)
	if contentType == "" {
		contentType = "image/png"
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="data"; filename="%s"`, filepath.Base(path))},
		"Content-Type":        {contentType},
	})
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := writer.WriteField("file_type", ext); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		buildURL("/samantha/pages/upload_image", samanthaParams()), &buf)
	if err != nil {
		return "", err
	}
	req.Header = c.commonHeaders()
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "*/*")
	req.Header.Del("Origin")
	req.Header.Set("Origin", BaseURL)
	if token := c.csrfToken(); token != "" {
		req.Header.Set("x-tt-passport-csrf-token", token)
	}

	c.logf("上传参考图 %s (%d bytes)", filepath.Base(path), len(data))
	resp, err := c.http.Do(req)
	if err != nil {
		return "", &Error{Kind: KindOther, Op: "上传参考图", Msg: "网络错误", Detail: err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", newErr(KindOther, "上传参考图", fmt.Sprintf("%d", resp.StatusCode),
			"上传失败", string(body))
	}

	m := jsonObject(string(body))
	if m == nil {
		return "", newErr(KindOther, "上传参考图", "", "响应不是 JSON", string(body))
	}
	if code := asInt(m["code"]); code != 0 {
		return "", newErr(classifyCode(fmt.Sprintf("%d", code)), "上传参考图",
			fmt.Sprintf("%d", code), asString(m["msg"]), string(body))
	}
	uri := asString(asMap(m["data"])["uri"])
	if uri == "" {
		return "", newErr(KindOther, "上传参考图", "", "响应里没有 data.uri", string(body))
	}
	return uri, nil
}
