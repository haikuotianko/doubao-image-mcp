package doubao

import (
	"encoding/json"
	"regexp"
	"strings"
)

// sseEvent 是一条命名 SSE 事件。
type sseEvent struct {
	Name string
	Data string
}

// parseSSE 把 SSE 原文按空行切块，取 event 名和 data 内容。
// 豆包会在同一条 data 里放很大的 JSON，所以不能用 Scanner 逐行限额读取。
func parseSSE(raw string) []sseEvent {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	chunks := strings.Split(raw, "\n\n")
	out := make([]sseEvent, 0, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		var ev sseEvent
		var data strings.Builder
		for _, line := range strings.Split(chunk, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				ev.Name = strings.TrimSpace(line[len("event:"):])
			case strings.HasPrefix(line, "data:"):
				data.WriteString(strings.TrimSpace(line[len("data:"):]))
			}
		}
		if data.Len() == 0 {
			continue
		}
		ev.Data = data.String()
		out = append(out, ev)
	}
	return out
}

// jsonObject 把 data 解析成对象，失败返回 nil。
func jsonObject(s string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}

// asMap 安全地取嵌套对象。
func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// asSlice 安全地取数组。
func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// asString 安全地取字符串。
func asString(v any) string {
	s, _ := v.(string)
	return s
}

// asInt 安全地取整数（JSON 数字在 Go 里是 float64）。
func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return 0
}

// imageFromItem 把服务端的一个 image 对象转成 Image。
//
// 字段优先级与参考实现一致：image_ori_raw（无水印）> image_ori > image_thumb。
func imageFromItem(item map[string]any) Image {
	ori := asMap(item["image_ori"])
	raw := asMap(item["image_ori_raw"])
	if len(raw) == 0 {
		// 旧通道里无水印字段叫 image_raw。
		raw = asMap(item["image_raw"])
	}
	thumb := asMap(item["image_thumb"])

	img := Image{
		Key:      asString(item["key"]),
		ThumbURL: asString(thumb["url"]),
		OriURL:   asString(ori["url"]),
		RawURL:   asString(raw["url"]),
		Width:    asInt(ori["width"]),
		Height:   asInt(ori["height"]),
		Format:   asString(ori["format"]),
	}
	if img.Width == 0 {
		img.Width = asInt(thumb["width"])
	}
	if img.Height == 0 {
		img.Height = asInt(thumb["height"])
	}
	if img.Format == "" {
		img.Format = asString(thumb["format"])
	}
	return img
}

// URL 返回该图最优先的下载地址。
func (i Image) URL() string { return i.imageURL() }

// imageURL 返回该图最优先的下载地址。
func (i Image) imageURL() string {
	for _, u := range []string{i.RawURL, i.OriURL, i.ThumbURL} {
		if u != "" {
			return u
		}
	}
	return ""
}

// creationImages 从 content_block 里收集 block_type=2074 的 creations。
func creationImages(blocks []any, into *[]Image) {
	for _, rawBlock := range blocks {
		block := asMap(rawBlock)
		if block == nil || asInt(block["block_type"]) != 2074 {
			continue
		}
		creation := asMap(asMap(block["content"])["creation_block"])
		for _, rawCreation := range asSlice(creation["creations"]) {
			img := asMap(asMap(rawCreation)["image"])
			if len(img) > 0 {
				*into = append(*into, imageFromItem(img))
			}
		}
	}
}

var genImageURLRe = regexp.MustCompile(`https://[^\s"<>]+rc_gen_image[^\s"<>]+`)
