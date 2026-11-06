package doubao

import (
	"regexp"
	"strings"
)

// 同一张图在服务端会有多个模板版本，靠 tplv 后缀区分：
//
//	~tplv-*-cgen_lwm3  带水印
//	~tplv-*-cgen_dwm3  去水印
//	~tplv-*-image_raw  原始图（最好）
//
// URL 的签名覆盖整个 path，不能把 lwm3 手改成 dwm3（会 400），只能用服务端给的。
const (
	rankNone = 0
	rankWm   = 1
	rankDwm  = 2
	rankRaw  = 3
)

var imageKeyRe = regexp.MustCompile(`rc_gen_image/([0-9a-zA-Z._-]+)`)

// imageKeyOf 从 URL 或 key 字段里取出这张图的稳定标识（rc_gen_image/<文件名>）。
func imageKeyOf(s string) string {
	if m := imageKeyRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// imageKey 返回这张图的稳定标识；拿不到就返回空。
func (i Image) imageKey() string {
	for _, s := range []string{i.Key, i.RawURL, i.OriURL, i.ThumbURL} {
		if k := imageKeyOf(s); k != "" {
			return k
		}
	}
	return ""
}

func urlRank(u string) int {
	switch {
	case strings.Contains(u, "-image_raw."):
		return rankRaw
	case strings.Contains(u, "-dwm3."):
		return rankDwm
	case strings.Contains(u, "~tplv-"):
		return rankWm
	default:
		return rankNone
	}
}

// genImageURLs 从 SSE 原文里捞出所有图床直链（原文里的 JSON 转义先还原）。
func genImageURLs(raw string) []string {
	if raw == "" {
		return nil
	}
	s := strings.NewReplacer(
		`\\u0026`, "&", `\u0026`, "&",
		`\\/`, "/", `\/`, "/",
		`\\`, "", `\`, "",
	).Replace(raw)
	return genImageURLRe.FindAllString(s, -1)
}

// mergeWatermarkFree 把原文里的无水印直链按「每一张图」合并进结果。
//
// 以前是全文只取第一个 image_raw / dwm3，再怼给 Images[0]：多图时其余几张
// 拿不到无水印地址，结构化解析漏掉的那张也永远补不回来（于是只出一张）。
//
// 现在按图的稳定标识归并，并补回结构化解析漏掉的图。
func mergeWatermarkFree(images []Image, raw string) []Image {
	best := make(map[string]string)
	var order []string
	for _, u := range genImageURLs(raw) {
		k := imageKeyOf(u)
		if k == "" {
			continue
		}
		if _, seen := best[k]; !seen {
			order = append(order, k)
		}
		if cur, ok := best[k]; !ok || urlRank(u) > urlRank(cur) {
			best[k] = u
		}
	}
	if len(best) == 0 {
		return images
	}

	matched := make(map[string]bool, len(best))
	for i := range images {
		k := images[i].imageKey()
		if k == "" {
			continue
		}
		u, ok := best[k]
		if !ok {
			continue
		}
		matched[k] = true
		if images[i].Key == "" {
			images[i].Key = k
		}
		if images[i].RawURL == "" || urlRank(u) > urlRank(images[i].RawURL) {
			images[i].RawURL = u
		}
	}
	// 原文里有、结构化解析没抓到的图，按出现顺序补回来。
	for _, k := range order {
		if matched[k] {
			continue
		}
		images = append(images, Image{Key: k, RawURL: best[k]})
	}
	return images
}

// mergeImage 把 b 里的有效字段补进 a（a 先出现，可能还没填全）。
func mergeImage(a, b Image) Image {
	if a.Key == "" {
		a.Key = b.Key
	}
	if a.ThumbURL == "" {
		a.ThumbURL = b.ThumbURL
	}
	if a.OriURL == "" {
		a.OriURL = b.OriURL
	}
	if b.RawURL != "" && urlRank(b.RawURL) > urlRank(a.RawURL) {
		a.RawURL = b.RawURL
	}
	if a.Width == 0 {
		a.Width = b.Width
	}
	if a.Height == 0 {
		a.Height = b.Height
	}
	if a.Format == "" {
		a.Format = b.Format
	}
	return a
}
