package doubao

import (
	"errors"
	"fmt"
	"strings"
)

// Kind 是错误分类，调用方据此给出可操作的提示。
type Kind int

const (
	// KindOther 是未分类错误。
	KindOther Kind = iota
	// KindLogin 表示登录态失效（710012001 / 未配置账号）。
	KindLogin
	// KindRisk 表示 IP 级风控（710022002 / 710022004），需要冷却或换网络。
	KindRisk
	// KindQuota 表示订阅/额度不足（服务端返回订阅提示）。
	KindQuota
	// KindNoImage 表示服务端没返回任何图片。
	KindNoImage
)

func (k Kind) String() string {
	switch k {
	case KindLogin:
		return "登录态失效"
	case KindRisk:
		return "风控限流"
	case KindQuota:
		return "额度不足"
	case KindNoImage:
		return "未出图"
	default:
		return "调用失败"
	}
}

// Error 是带分类的豆包接口错误。
type Error struct {
	Kind   Kind
	Op     string
	Code   string
	Msg    string
	Detail string
}

func (e *Error) Error() string {
	var b strings.Builder
	if e.Op != "" {
		b.WriteString(e.Op)
		b.WriteString(": ")
	}
	b.WriteString(e.Msg)
	if e.Code != "" {
		b.WriteString("（code=")
		b.WriteString(e.Code)
		b.WriteString("）")
	}
	if e.Detail != "" {
		b.WriteString("：")
		b.WriteString(e.Detail)
	}
	return b.String()
}

// Hint 返回给人看的处置建议。
func (e *Error) Hint() string {
	switch e.Kind {
	case KindLogin:
		return "登录态已失效，请在「豆包生图 MCP」里重新扫码登录。"
	case KindRisk:
		return "豆包 IP 级风控限流：等 30 分钟冷却、换网络（手机热点最快），或在浏览器里手动完成一次滑块验证。"
	case KindQuota:
		return "当前账号没有该模型的创作额度：换免费的 Seedream 5.0 Flash / 4.5 / 4.0，或升级订阅。"
	case KindNoImage:
		return "服务端没有返回图片，稍后重试；若反复出现请换比例或换模型。"
	default:
		if e.Code == "710020702" {
			return "服务端回的是通用「系统错误」，不是额度问题：图生图通道（旧 samantha）经常报这个，先用文生图；反复出现可稍后重试或换个模型。"
		}
		return ""
	}
}

// Is 判断错误是否属于某个分类。
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return t.Kind == e.Kind && t.Op == e.Op
}

// IsKind 判断 err 是否属于某个错误分类。
func IsKind(err error, k Kind) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind == k
	}
	return false
}

// classifyCode 把服务端错误码映射成分类。
func classifyCode(code string) Kind {
	switch strings.TrimSpace(code) {
	case "710012001":
		return KindLogin
	case "710022002", "710022004":
		return KindRisk
	}
	return KindOther
}

var paywallKeywords = []string{
	"订阅标准套餐专属能力",
	"不可用于创作",
	"升级正式订阅",
	"免费体验套餐",
}

// paywallHit 判断文本是否包含付费墙提示。
func paywallHit(text string) bool {
	for _, kw := range paywallKeywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// noticeErr 把服务端错误文本变成分类错误：先按 code 归类，命中付费墙关键词
// 才算额度不足，剩下的一律当普通接口错误——否则「system error」会被误报成没额度。
func noticeErr(op, notice string) *Error {
	code := extractCode([]byte(notice))
	kind := classifyCode(code)
	if kind == KindOther && paywallHit(notice) {
		kind = KindQuota
	}
	msg := "服务端返回错误"
	if kind == KindQuota {
		msg = "当前模型需要订阅"
	}
	return newErr(kind, op, code, msg, notice)
}

func newErr(kind Kind, op, code, msg, detail string) *Error {
	return &Error{Kind: kind, Op: op, Code: code, Msg: msg, Detail: truncate(detail, 500)}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// LoginErr 构造登录态错误。
func LoginErr(op, msg string) error {
	return &Error{Kind: KindLogin, Op: op, Msg: msg}
}

// FormatError 把错误渲染成「失败原因 + 处置建议」。
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) {
		out := fmt.Sprintf("❌ %s（%s）", e.Kind, e.Error())
		if h := e.Hint(); h != "" {
			out += "\n   → " + h
		}
		return out
	}
	return "❌ " + err.Error()
}
