// Package doubao 是豆包网页版生图能力的 Go 客户端。
//
// 协议来源：对照 doubao-web-mcp（Python）的抓包结论实现，两条通道：
//
//   - 文生图（默认）：POST /chat/completion + chat_ability(ability_type=3) 的嵌套
//     ability_param，可精确选择模型/比例/风格，免签名。
//   - 图生图（带参考图）：POST /samantha/chat/completion，content_type=2009 +
//     skill.skill_type=3，参考图先 upload_image 拿 uri 再作为 attachment。
//
// 注意：X-Bogus 签名已失效，加签名反而会触发 710022004 人机验证，本包不签名。
package doubao

// BaseURL 是豆包网页版根地址。
const BaseURL = "https://www.doubao.com"

const (
	chromeVersion  = "148.0.0.0"
	chromiumBuild  = "148.0.7816.0"
	defaultDevice  = "7698374278500353570"
	defaultBotID   = "7338286299411103781"
	replyImageText = "生成图片：%s"
)

// Model 描述一个官方创作模型。
type Model struct {
	Name        string `json:"name"`
	ID          int    `json:"id"`
	Sub         string `json:"sub"`
	NeedUpgrade bool   `json:"needUpgrade"`
	Default     bool   `json:"default"`
}

// Models 是「模型」选择器（图片项 item_id=45033999706177282，selector id=30）清单。
// option_key 即显示文本，也就是 chat_ability 里 ability_param.model 的取值。
var Models = []Model{
	{Name: "Seedream 5.0 Flash", ID: 6, Sub: "专业快速（免费，推荐）", NeedUpgrade: false},
	{Name: "Seedream 5.0 Pro", ID: 5, Sub: "专业出图・4 倍消耗（需订阅）", NeedUpgrade: true},
	{Name: "Seedream 4.5", ID: 3, Sub: "日常生成（免费）", NeedUpgrade: false},
	{Name: "Seedream 4.0", ID: 1, Sub: "基础生图（免费，默认）", NeedUpgrade: false, Default: true},
}

// DefaultModelName 返回默认模型名。
func DefaultModelName() string {
	for _, m := range Models {
		if m.Default {
			return m.Name
		}
	}
	return Models[0].Name
}

// ModelByName 查找模型（大小写不敏感，允许只写数字 id）。
func ModelByName(name string) (Model, bool) {
	for _, m := range Models {
		if m.Name == name {
			return m, true
		}
	}
	for _, m := range Models {
		if equalFold(m.Name, name) {
			return m, true
		}
	}
	return Model{}, false
}

// Style 是官方「风格」选择器（selector id=4）的一个 option_key。
type Style struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Styles 是 32 个官方风格（option_key → 中文显示名）。
// 前端把 message_text 写成“图片风格为{显示名}”，服务端据此生效。
var Styles = []Style{
	{"skill_image_styles_portrait", "人像摄影"},
	{"skill_image_styles_film", "电影写真"},
	{"skill_image_styles_chinese", "中国风"},
	{"skill_image_styles_japanese_anime", "动漫"},
	{"skill_image_styles_3d", "3D渲染"},
	{"image_gen_style_cyberpunk", "赛博朋克"},
	{"skill_image_styles_cg", "CG 动画"},
	{"skill_image_styles_ink_wash_painting", "水墨画"},
	{"skill_image_styles_oil_painting", "油画"},
	{"skill_image_styles_classic", "古典"},
	{"skill_image_styles_watercolor", "水彩画"},
	{"skill_image_styles_cartoon", "卡通"},
	{"skill_image_styles_flat_illustration", "平面插画"},
	{"skill_image_styles_landscape", "风景"},
	{"skill_image_styles_hongkong_anime", "港风动漫"},
	{"skill_image_styles_pixel_style", "像素风格"},
	{"skill_image_styles_fluorescence", "荧光绘画"},
	{"skill_image_styles_colored_pencil", "彩铅画"},
	{"skill_image_styles_figure", "手办"},
	{"skill_image_styles_children_illustration", "儿童绘画"},
	{"skill_image_styles_abstract", "抽象"},
	{"skill_image_styles_sharp_illustration", "锐笔插画"},
	{"skill_image_styles_acg", "二次元"},
	{"skill_image_styles_ink_print", "油墨印刷"},
	{"skill_image_styles_bnw_printing", "版画"},
	{"skill_image_styles_monet", "莫奈"},
	{"skill_image_styles_picasso", "毕加索"},
	{"skill_image_styles_rembrandt", "伦勃朗"},
	{"skill_image_styles_matisse", "马蒂斯"},
	{"skill_image_styles_baroque", "巴洛克"},
	{"skill_image_styles_oldschool", "复古动漫"},
	{"skill_image_styles_picturebook", "绘本"},
}

// StyleName 返回风格显示名；未知 key 原样返回。
func StyleName(key string) string {
	for _, s := range Styles {
		if s.Key == key {
			return s.Name
		}
		if s.Name == key {
			return s.Name
		}
	}
	return key
}

// Ratios 是常用比例（服务端支持 auto）。
var Ratios = []string{"1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3", "auto"}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
