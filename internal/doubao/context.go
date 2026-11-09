package doubao

import "strings"

// 复用同一个对话时，新提示词会和前面的图片共享上下文。
// 用一段提示词把二者隔开，避免「我要一张雪山」被理解成「给上一张图加雪山」。
const (
	// ContextNone 不隔离：提示词原样发送，模型可以自由引用上文。
	ContextNone = "none"
	// ContextAlways 每次隔离：明确告诉模型忘掉上文，只按新提示词生成。
	ContextAlways = "always"
	// ContextAuto 自动：把判断权交给模型（有关联就沿用，没关联就忽略）。
	ContextAuto = "auto"
)

const (
	contextRuleAlways = "这是一次全新的生图请求，与之前生成的内容没有任何关联。" +
		"请忽略上面的全部上下文，只根据下面的提示词生成图片：\n"
	contextRuleAuto = "如果下面的生图请求与之前的内容无关，请忽略上文、只按新提示词生成；" +
		"如果与之前有关联（例如同一主体换风格、换角度、继续修改），可以沿用上文信息：\n"
)

// NormalizeContextMode 把配置值收敛成三档之一，未知值按自动处理。
func NormalizeContextMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ContextNone:
		return ContextNone
	case ContextAlways:
		return ContextAlways
	default:
		return ContextAuto
	}
}

// withContextRule 按隔离策略给提示词加前缀。
func withContextRule(prompt, mode string) string {
	switch NormalizeContextMode(mode) {
	case ContextAlways:
		return contextRuleAlways + prompt
	case ContextAuto:
		return contextRuleAuto + prompt
	default:
		return prompt
	}
}

// ParseChatID 从用户输入里取出会话 id。
// 既可以是纯 id（38451559467912706），也可以是网页链接（https://www.doubao.com/chat/<id>）。
func ParseChatID(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}
	// 取最后一段路径，去掉查询串和锚点
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimRight(s, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	// 只接受纯数字，避免把任意文字当成 id
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return s
}

// ChatURL 由会话 id 拼出网页地址；没有 id 时返回空串。
func ChatURL(conversationID string) string {
	id := strings.TrimSpace(conversationID)
	if id == "" {
		return ""
	}
	return "https://www.doubao.com/chat/" + id
}
