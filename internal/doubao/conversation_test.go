package doubao

import "testing"

// 响应里先出现一个「别的」conversation_id（干扰项），真正的会话 id 在 ack_client_meta 里。
// 结构化解析必须拿到 ack 里的那个，而不是全文第一个。
const decoySSE = `data: {"some_other":{"conversation_id":"11111111111111111"}}

data: {"message":{"conversation_id":"11111111111111111","message_id":"1"}}

data: {"downlink_body":{"ack_client_meta":{"conversation_id":"22222222222222222","local_conversation_id":"local_1","conversation_type":3,"section_id":"33333333333333333","conversation_info":{"conversation_id":"22222222222222222"}}}}

`

func TestParseImageAbilitySSEPicksAckConversation(t *testing.T) {
	res, err := parseImageAbilitySSE(decoySSE, Request{Prompt: "x"})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if res.ConversationID != "22222222222222222" {
		t.Errorf("会话 id 应为 ack_client_meta 里的 22222222222222222，得到 %q", res.ConversationID)
	}
	if res.SectionID != "33333333333333333" {
		t.Errorf("section id 应为 33333333333333333，得到 %q", res.SectionID)
	}
}

func TestParseImageAbilitySSEFallbackToMessage(t *testing.T) {
	// 没有 ack_client_meta 时退回 message.conversation_id
	raw := `data: {"message":{"conversation_id":"44444444444444444"}}
`
	res, err := parseImageAbilitySSE(raw, Request{Prompt: "x"})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if res.ConversationID != "44444444444444444" {
		t.Errorf("应退回 message.conversation_id，得到 %q", res.ConversationID)
	}
}

func TestParseImageAbilitySSENoConversation(t *testing.T) {
	// 旧通道那种没有会话信息的响应：应保持空，交由上层清掉记录
	raw := `data: {"patch_op":[{"patch_value":{"content_block":[]}}]}
`
	res, err := parseImageAbilitySSE(raw, Request{Prompt: "x"})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if res.ConversationID != "" {
		t.Errorf("不应解析出会话 id，得到 %q", res.ConversationID)
	}
}

func TestParseImageAbilitySSECapturesMessageIndex(t *testing.T) {
	// 样本取自网页版真实响应：ack 帧里带 message_index，消息帧里带 index_in_conv
	raw := `data: {"message":{"conversation_id":"38452605462188546","message_id":"1","index_in_conv":16}}

data: {"downlink_body":{"ack_client_meta":{"conversation_id":"38452605462188546","section_id":"38452605462188802","message_index":16}},"modify":[{"message":{"index_in_conv":17}}]}
`
	res, err := parseImageAbilitySSE(raw, Request{Prompt: "x"})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if res.ConversationID != "38452605462188546" {
		t.Errorf("会话 id 解析错误：%q", res.ConversationID)
	}
	if res.LastMessageIndex != 17 {
		t.Errorf("消息序号应取最大值 17，得到 %d", res.LastMessageIndex)
	}
}

func TestLastIndexValue(t *testing.T) {
	if v := lastIndexValue(0); v != nil {
		t.Errorf("0 应该输出 null，得到 %#v", v)
	}
	if v := lastIndexValue(17); v != 17 {
		t.Errorf("17 应该原样输出，得到 %#v", v)
	}
}
