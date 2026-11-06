package doubao

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // 允许读取 webp 结果图
)

// FitRatio 把图片按目标比例补边：原图等比缩小居中，四周用自身放大虚化压暗作背景。
//
// 服务端给的图片比例偶尔和请求的不一致（尤其是 auto / 风格模板），这里做一次
// 二次修正。偏差小于 3% 视为已经达标，直接返回原路径。
func FitRatio(path, ratio string) (string, error) {
	target, ok := parseRatio(ratio)
	if !ok {
		return path, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return path, err
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return path, nil // 解不开就当已经合规，别把图弄丢
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return path, nil
	}
	cur := float64(w) / float64(h)
	if math.Abs(cur/target-1) < 0.03 {
		return path, nil
	}

	var cw, ch int
	if cur > target {
		ch = h
		cw = int(math.Round(float64(h) * target))
	} else {
		cw = w
		ch = int(math.Round(float64(w) / target))
	}
	if cw < 1 || ch < 1 {
		return path, nil
	}

	canvas := image.NewRGBA(image.Rect(0, 0, cw, ch))

	// 背景：先按 cover 铺满画布，再缩到很小再放大（等价于大半径模糊），最后压暗。
	bw, bh := coverSize(w, h, cw, ch)
	bg := image.NewRGBA(image.Rect(0, 0, bw, bh))
	xdraw.ApproxBiLinear.Scale(bg, bg.Bounds(), img, b, xdraw.Src, nil)

	blur := image.NewRGBA(image.Rect(0, 0, atLeast(1, bw/64), atLeast(1, bh/64)))
	xdraw.ApproxBiLinear.Scale(blur, blur.Bounds(), bg, bg.Bounds(), xdraw.Src, nil)
	// 两轮缩放让残留结构彻底糊掉，否则背景里还能认出主体。
	mid := image.NewRGBA(image.Rect(0, 0, blur.Bounds().Dx()*2, blur.Bounds().Dy()*2))
	xdraw.ApproxBiLinear.Scale(mid, mid.Bounds(), blur, blur.Bounds(), xdraw.Src, nil)
	xdraw.ApproxBiLinear.Scale(blur, blur.Bounds(), mid, mid.Bounds(), xdraw.Src, nil)
	xdraw.ApproxBiLinear.Scale(bg, bg.Bounds(), blur, blur.Bounds(), xdraw.Src, nil)
	darken(bg, 0.45)
	xdraw.Draw(canvas, canvas.Bounds(), bg, image.Pt((bw-cw)/2, (bh-ch)/2), xdraw.Src)

	// 前景：原图按 contain 等比缩放居中盖上去。
	fw, fh := containSize(w, h, cw, ch)
	fx, fy := (cw-fw)/2, (ch-fh)/2
	xdraw.ApproxBiLinear.Scale(canvas, image.Rect(fx, fy, fx+fw, fy+fh), img, b, xdraw.Over, nil)

	var out bytes.Buffer
	if strings.EqualFold(format, "jpeg") {
		if err := jpeg.Encode(&out, canvas, &jpeg.Options{Quality: 95}); err != nil {
			return path, err
		}
	} else if err := png.Encode(&out, canvas); err != nil {
		return path, err
	}
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		return path, err
	}
	return path, nil
}

// parseRatio 解析 "16:9" 这类比例，auto 或非法值返回 false。
func parseRatio(ratio string) (float64, bool) {
	parts := strings.SplitN(strings.TrimSpace(ratio), ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	a, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	b, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil || a <= 0 || b <= 0 {
		return 0, false
	}
	return a / b, true
}

// coverSize 返回铺满画布所需的尺寸（等比放大，超出部分裁掉）。
func coverSize(w, h, cw, ch int) (int, int) {
	scale := math.Max(float64(cw)/float64(w), float64(ch)/float64(h))
	return atLeast(cw, int(math.Ceil(float64(w)*scale))), atLeast(ch, int(math.Ceil(float64(h)*scale)))
}

// containSize 返回放进画布内的尺寸（等比缩小，不裁剪）。
func containSize(w, h, cw, ch int) (int, int) {
	scale := math.Min(float64(cw)/float64(w), float64(ch)/float64(h))
	return atLeast(1, int(math.Round(float64(w)*scale))), atLeast(1, int(math.Round(float64(h)*scale)))
}

// darken 按系数压暗整张图（原地修改）。
func darken(img *image.RGBA, factor float64) {
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = uint8(float64(img.Pix[i]) * factor)
		img.Pix[i+1] = uint8(float64(img.Pix[i+1]) * factor)
		img.Pix[i+2] = uint8(float64(img.Pix[i+2]) * factor)
	}
}

func atLeast(floor, v int) int {
	if v < floor {
		return floor
	}
	return v
}
