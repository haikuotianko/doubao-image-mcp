"""生成发帖用的推广图（cover / multi / features）。

素材：build/promo/gui.png（v1.4.0 界面）、C:\\Users\\lucky\\Pictures\\webToMcp 下的样例图。
输出：build/promo/*.png
"""
import pathlib

from PIL import Image, ImageDraw, ImageFont

ROOT = pathlib.Path(__file__).resolve().parents[1]
PROMO = ROOT / "build" / "promo"
SAMPLES = pathlib.Path(r"C:\Users\lucky\Pictures\webToMcp")
FONT_R = r"C:\Windows\Fonts\msyh.ttc"
FONT_B = r"C:\Windows\Fonts\msyhbd.ttc"

BLUE = (0, 87, 255)
DEEP = (0, 36, 140)


def font(size: int, bold: bool = False) -> ImageFont.FreeTypeFont:
    return ImageFont.truetype(FONT_B if bold else FONT_R, size)


def gradient(size, top, bottom) -> Image.Image:
    w, h = size
    base = Image.new("RGB", (w, h))
    d = ImageDraw.Draw(base)
    for y in range(h):
        t = y / max(1, h - 1)
        c = tuple(int(top[i] + (bottom[i] - top[i]) * t) for i in range(3))
        d.line([(0, y), (w, y)], fill=c)
    return base


def rounded(im: Image.Image, radius: int) -> Image.Image:
    mask = Image.new("L", im.size, 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, im.size[0] - 1, im.size[1] - 1], radius, fill=255)
    out = im.convert("RGBA")
    out.putalpha(mask)
    return out


def cover() -> pathlib.Path:
    W, H = 1200, 630
    img = gradient((W, H), BLUE, DEEP).convert("RGBA")

    # 装饰：半透明形状必须画在独立图层再合成，直接画在 RGBA 上会变成实心白块
    deco = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    dd = ImageDraw.Draw(deco)
    dd.ellipse([-160, -220, 260, 200], fill=(255, 255, 255, 20))
    dd.ellipse([900, 470, 1340, 870], fill=(255, 255, 255, 16))
    img = Image.alpha_composite(img, deco)

    x, y = 64, 92
    d = ImageDraw.Draw(img)
    d.text((x, y), "豆包生图 MCP", font=font(66, True), fill=(255, 255, 255))
    d.text((x, y + 92), "用你自己的豆包登录态生图，不需要 API Key", font=font(28), fill=(214, 228, 255))

    chips = ["6 个 MCP 工具", "多账号自动排队", "同一对话连续生图", "风控滑块验证"]
    pill = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    pd = ImageDraw.Draw(pill)
    rects = []
    cx, cy = x, y + 160
    limit = 620  # 截图从右侧 644 开始，标签必须在它左边收住
    for text in chips:
        f = font(21)
        tw = d.textlength(text, font=f)
        width = tw + 32
        if cx + width > limit:          # 放不下就换行
            cx, cy = x, cy + 56
        box = [cx, cy, cx + width, cy + 42]
        pd.rounded_rectangle(box, 21, fill=(255, 255, 255, 46), outline=(255, 255, 255, 120))
        rects.append((box, text, f))
        cx += width + 12
    img = Image.alpha_composite(img, pill)
    d = ImageDraw.Draw(img)
    for box, text, f in rects:
        d.text((box[0] + 17, box[1] + 9), text, font=f, fill=(255, 255, 255))

    d.text((x, H - 96), "Windows x64 / x86 / ARM64  ·  macOS / Linux CLI", font=font(21), fill=(190, 210, 255))
    d.text((x, H - 62), "github.com/haikuotianko/doubao-image-mcp", font=font(24, True), fill=(255, 255, 255))

    shot = Image.open(PROMO / "gui.png").convert("RGB")
    sw = 500
    shot = shot.resize((sw, int(shot.height * sw / shot.width)), Image.LANCZOS)
    shot = rounded(shot, 16)
    px, py = W - sw - 56, (H - shot.height) // 2
    shadow = Image.new("RGBA", (shot.width + 40, shot.height + 40), (0, 0, 0, 0))
    ImageDraw.Draw(shadow).rounded_rectangle([20, 20, shot.width + 20, shot.height + 20], 18, fill=(0, 0, 0, 80))
    img.alpha_composite(shadow, (px - 20, py - 14))
    img.alpha_composite(shot, (px, py))

    out = PROMO / "cover.png"
    img.convert("RGB").save(out, quality=92)
    return out


def multi() -> pathlib.Path:
    names = [f"doubao_20261106_112558_{i}.png" for i in range(1, 5)]
    tile, gap, pad, header = 420, 16, 28, 92
    W = pad * 2 + tile * 2 + gap
    H = header + pad + tile * 2 + gap + 56
    img = Image.new("RGB", (W, H), (250, 251, 254))
    d = ImageDraw.Draw(img)
    d.text((pad, 24), "一次调用，返回 4 张（无水印直链）", font=font(34, True), fill=(13, 21, 38))
    d.text((pad, 68), "doubao_image(prompt=\"...\", ratio=\"1:1\")", font=font(19), fill=(91, 102, 122))

    for i, n in enumerate(names):
        p = SAMPLES / n
        if not p.exists():
            continue
        im = Image.open(p).convert("RGB").resize((tile, tile), Image.LANCZOS)
        im = rounded(im, 14)
        r, c = divmod(i, 2)
        x = pad + c * (tile + gap)
        y = header + pad + r * (tile + gap)
        img.paste(im, (x, y), im)

    d.text((pad, H - 44), "本地路径 + CDN 链接 + 尺寸，全部返回", font=font(20), fill=(91, 102, 122))
    out = PROMO / "multi.png"
    img.save(out, quality=92)
    return out


def features() -> pathlib.Path:
    W, H = 1200, 620
    img = gradient((W, H), (245, 248, 255), (224, 234, 255)).convert("RGBA")
    d = ImageDraw.Draw(img)
    d.text((56, 44), "为什么不直接用官方 API？", font=font(38, True), fill=(13, 21, 38))

    items = [
        ("不用 API Key", "用你自己的豆包登录态，扫码一次即可"),
        ("Seedream 全系", "5.0 Flash / 4.5 / 4.0 / 5.0 Pro 随选"),
        ("多图全量", "一次 1~4 张，全部返回，带无水印直链"),
        ("多账号排队", "每号并发 1 个任务，忙了自动排队"),
        ("对话复用", "连续生图都在同一个对话里，不刷屏"),
        ("风控可自救", "被要求人机验证时，一键弹窗拖滑块"),
    ]
    col_w = (W - 56 * 2 - 24) // 2
    for i, (title, desc) in enumerate(items):
        r, c = divmod(i, 2)
        x = 56 + c * (col_w + 24)
        y = 120 + r * 152
        d.rounded_rectangle([x, y, x + col_w, y + 128], 16, fill=(255, 255, 255, 235))
        d.text((x + 24, y + 22), title, font=font(26, True), fill=(0, 87, 255))
        d.text((x + 24, y + 64), desc, font=font(19), fill=(70, 82, 104))

    out = PROMO / "features.png"
    img.convert("RGB").save(out, quality=92)
    return out


if __name__ == "__main__":
    for fn in (cover, multi, features):
        p = fn()
        print("wrote", p, p.stat().st_size // 1024, "KB")
