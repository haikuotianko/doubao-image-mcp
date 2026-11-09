package doubao

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// buildAbilityPayload 构造 /chat/completion 的 body（抓包真值，逐字段对齐前端）。
//
// conversationID 非空表示「在这个已有对话里继续」：此时不再新建对话，
// 并把上一段的 section_id 带回去，服务端就会把新消息追加到同一个对话里。
func buildAbilityPayload(prompt string, abilityType int, inner map[string]any, replyFormat, conversationID, sectionID string, lastMessageIndex int) map[string]any {
	nowMS := time.Now().UnixMilli()
	innerJSON, _ := json.Marshal(inner)
	reuse := strings.TrimSpace(conversationID) != ""
	return map[string]any{
		"client_meta": map[string]any{
			"local_conversation_id": fmt.Sprintf("local_%d", nowMS),
			"conversation_id":       conversationID,
			"bot_id":                defaultBotID,
			"last_section_id":       sectionID,
			"last_message_index":    lastIndexValue(lastMessageIndex),
			"local_permissions": []any{
				map[string]any{"permission_name": "ACCESS_COARSE_LOCATION", "status": 3},
				map[string]any{"permission_name": "ACCESS_FINE_LOCATION", "status": 3},
				map[string]any{"permission_name": "ACCESS_BACKGROUND_LOCATION", "status": 3},
			},
		},
		"messages": []any{
			map[string]any{
				"local_message_id": uuid4(),
				"content_block": []any{
					map[string]any{
						"block_type": 10000,
						"content": map[string]any{
							"text_block": map[string]any{
								"text":          fmt.Sprintf(replyFormat, prompt),
								"icon_url":      "",
								"icon_url_dark": "",
								"summary":       "",
							},
							"pc_event_block": "",
						},
						"block_id":      uuid4(),
						"parent_id":     "",
						"meta_info":     []any{},
						"extra_info":    map[string]any{},
						"append_fields": []any{},
					},
				},
				"message_status": 0,
			},
		},
		"option": map[string]any{
			"send_message_scene":        "",
			"create_time_ms":            nowMS,
			"collect_id":                "",
			"is_audio":                  false,
			"answer_with_suggest":       false,
			"tts_switch":                false,
			"need_deep_think":           0,
			"click_clear_context":       false,
			"from_suggest":              false,
			"is_regen":                  false,
			"is_replace":                false,
			"is_from_click_option":      false,
			"is_from_click_softlink":    false,
			"disable_sse_cache":         false,
			"select_text_action":        "",
			"is_select_text":            false,
			"resend_for_regen":          false,
			"scene_type":                0,
			"unique_key":                uuid4(),
			"start_seq":                 0,
			"need_create_conversation":  !reuse,
			"conversation_init_option":  map[string]any{"need_ack_conversation": !reuse},
			"regen_query_id":            []any{},
			"edit_query_id":             []any{},
			"regen_instruction":         "",
			"no_replace_for_regen":      false,
			"message_from":              0,
			"shared_app_name":           "",
			"shared_app_id":             "",
			"sse_recv_event_options":    map[string]any{"support_chunk_delta": true},
			"support_lazy_fetch_stream": true,
			"is_ai_playground":          false,
			"is_old_user":               true,
			"general_task_param":        map[string]any{"server_do_compare": true},
			"recovery_option": map[string]any{
				"is_recovery":            false,
				"req_create_time_sec":    time.Now().Unix(),
				"append_sse_event_scene": 0,
			},
			"message_storage_type":        0,
			"related_deleted_message_ids": map[string]any{},
			"connector_info_list":         []any{},
			"model_config":                map[string]any{"model_item_key": "", "model_extra_params": map[string]any{}},
			"aggregate_params": map[string]any{
				"mention_skill_list":  "[]",
				"mention_plugin_list": "[]",
				"mention_ext":         "[{}]",
				"conversation_mode":   "1",
				"mode_id":             "",
				"model_item_key":      "",
				"agent_mode":          "",
				"reasoning_effort":    "",
				"provider_id":         "",
			},
			"conversation_mode": 1,
		},
		"chat_ability": map[string]any{
			"ability_type":  abilityType,
			"ability_param": string(innerJSON),
		},
		"user_context": []any{},
		"ext": map[string]any{
			"answer_with_suggest":           "0",
			"sub_conv_firstmet_type":        "1",
			"general_task_param":            `{"server_do_compare":true}`,
			"collection_id":                 "",
			"is_finish":                     "1",
			"conversation_init_option":      fmt.Sprintf(`{"need_ack_conversation":%t}`, !reuse),
			"commerce_credit_config_enable": "0",
		},
	}
}

// generateTextToImage 走新端点：模型/比例/风格精确选择。
func (c *Client) generateTextToImage(ctx context.Context, req Request, timeout time.Duration) (*Result, error) {
	model := req.Model
	if strings.TrimSpace(model) == "" {
		model = DefaultModelName()
	}
	if _, ok := ModelByName(model); !ok {
		names := make([]string, 0, len(Models))
		for _, m := range Models {
			names = append(names, m.Name)
		}
		return nil, fmt.Errorf("未知模型 %q，可用：%s", model, strings.Join(names, " / "))
	}
	ratio := strings.TrimSpace(req.Ratio)
	if ratio == "" {
		ratio = "auto"
	}

	abilityText := withContextRule(req.Prompt, req.ContextMode)
	if req.Style != "" {
		abilityText = abilityText + "。图片风格为" + StyleName(req.Style)
	}

	inner := map[string]any{
		"ability_param": map[string]any{
			"model": model,
			"ratio": ratio,
			"input_box_content": map[string]any{
				"user_input_content":   abilityText,
				"reply_message_format": replyImageText,
			},
		},
		"ability_type": 1,
	}

	payload := buildAbilityPayload(req.Prompt, 3, inner, replyImageText, req.ConversationID, req.SectionID, req.LastMessageIndex)
	raw, err := c.postBody(ctx, buildURL("/chat/completion", newEndpointParams()), payload, timeout)
	if err != nil {
		return nil, err
	}
	res, err := parseImageAbilitySSE(string(raw), req)
	if err != nil {
		return nil, err
	}
	res.Channel = "ability"
	if len(res.Images) == 0 {
		if res.Notice != "" {
			return nil, noticeErr("生成图片", res.Notice)
		}
		return nil, newErr(KindNoImage, "生成图片", "", "服务端没有返回图片", truncate(string(raw), 300))
	}
	return res, nil
}

// parseImageAbilitySSE 解析新端点的生图 SSE。
//
// 成品在 STREAM_CHUNK 的 patch_op → content_block(block_type=2074)
// → creation_block.creations[].image；FULL_MSG_NOTIFY 里也有一份完整的。
func parseImageAbilitySSE(raw string, req Request) (*Result, error) {
	// 抓原始 SSE 用：设 DOUBAO_MCP_DUMP_SSE=<文件路径> 就会把这次响应原文写下来，
	// 服务端改结构时靠它对齐字段。
	if path := strings.TrimSpace(os.Getenv("DOUBAO_MCP_DUMP_SSE")); path != "" {
		_ = os.WriteFile(path, []byte(raw), 0o644)
	}
	res := &Result{Prompt: req.Prompt, Model: req.Model, Ratio: req.Ratio}
	if res.Model == "" {
		res.Model = DefaultModelName()
	}
	var noticeHits []string
	var ids convIDs

	for _, ev := range parseSSE(raw) {
		if ev.Name == "STREAM_ERROR" || ev.Name == "gateway-error" {
			d := jsonObject(ev.Data)
			if d == nil {
				return nil, newErr(KindOther, "生成图片", "", "服务端流错误", ev.Data)
			}
			code := ""
			if v, ok := d["error_code"]; ok {
				code = fmt.Sprint(v)
			} else if v, ok := d["code"]; ok {
				code = fmt.Sprint(v)
			}
			msg := asString(d["message"])
			if msg == "" {
				msg = asString(d["msg"])
			}
			return nil, newErr(classifyCode(code), "生成图片", code, "服务端返回错误", msg+" "+ev.Data)
		}
		// 会话线索可能在任意事件里（ack_client_meta 那帧的事件名不在生图白名单里），
		// 所以先提取，再按事件类型筛图片内容。
		if d := jsonObject(ev.Data); d != nil {
			ids.pick(d)
		}
		if ev.Name != "STREAM_CHUNK" && ev.Name != "FULL_MSG_NOTIFY" && ev.Name != "STREAM_MSG_NOTIFY" {
			continue
		}
		d := jsonObject(ev.Data)
		if d == nil {
			continue
		}
		for _, rawPatch := range asSlice(d["patch_op"]) {
			pv := asMap(asMap(rawPatch)["patch_value"])
			creationImages(asSlice(pv["content_block"]), &res.Images)
		}
		msg := asMap(d["message"])
		creationImages(asSlice(msg["content_block"]), &res.Images)

		if paywallHit(ev.Data) {
			for _, kw := range paywallKeywords {
				if strings.Contains(ev.Data, kw) {
					noticeHits = append(noticeHits, kw)
				}
			}
		}
	}

	// 全文兜底：按图合并无水印直链，并补回结构化解析漏掉的图（多图时一张都不能少）。
	res.Images = dedupeImages(mergeWatermarkFree(res.Images, raw))
	ids.apply(res)
	extractConversation(raw, res)
	if len(noticeHits) > 0 && len(res.Images) == 0 {
		res.Notice = "当前账号无该模型的创作额度（服务端返回订阅提示：" +
			strings.Join(dedupeStrings(noticeHits), "、") + "），未出图。"
	}
	return res, nil
}

// dedupeImages 合并同一张图的多份记录（patch 帧与 full notify 各带一份），
// 保留信息最全的那份：先出现的可能还没填上尺寸/key/无水印地址。
func dedupeImages(in []Image) []Image {
	out := make([]Image, 0, len(in))
	index := make(map[string]int, len(in))
	for _, img := range in {
		if img.imageURL() == "" {
			continue // 生成过程中的占位帧，没有任何地址
		}
		key := img.imageKey()
		if key == "" {
			key = img.imageURL()
		}
		if pos, ok := index[key]; ok {
			out[pos] = mergeImage(out[pos], img)
			continue
		}
		index[key] = len(out)
		out = append(out, img)
	}
	return out
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// 从 SSE 原文里提取会话 id / section id（兜底手段）。
// 结构化解析不到时才用：正则取的是全文第一处，理论上可能命中别的对话 id。
var (
	conversationIDRe = regexp.MustCompile(`"conversation_id"\s*:\s*"(\d+)"`)
	sectionIDRe      = regexp.MustCompile(`"section_id"\s*:\s*"(\d+)"`)
)

// convIDs 收集一帧一帧里的会话线索，最后按优先级决定用哪个：
// ack_client_meta（服务端确认的那一帧）> message/meta 里的会话信息 > 全文正则兜底。
type convIDs struct {
	ackConv, ackSection string
	msgConv, msgSection string
	lastIndex           int
}

// pickConversation 从单帧 JSON 里取会话 id（只记录，不直接写进结果）。
func (c *convIDs) pick(d map[string]any) {
	if d == nil {
		return
	}
	if ack := findMapByKey(d, "ack_client_meta", 6); ack != nil {
		if c.ackConv == "" {
			c.ackConv = asString(ack["conversation_id"])
		}
		if c.ackSection == "" {
			c.ackSection = asString(ack["section_id"])
		}
	}
	if c.msgConv == "" {
		if msg := asMap(d["message"]); msg != nil {
			c.msgConv = asString(msg["conversation_id"])
		}
	}
	if c.msgSection == "" {
		if meta := asMap(d["meta"]); meta != nil {
			c.msgSection = asString(meta["section_id"])
		}
	}
	for _, key := range []string{"index_in_conv", "message_index"} {
		if n := findMaxIntByKey(d, key, 6); n > c.lastIndex {
			c.lastIndex = n
		}
	}
}

// apply 把收集到的 id 按优先级写进结果。
func (c *convIDs) apply(res *Result) {
	switch {
	case c.ackConv != "":
		res.ConversationID = c.ackConv
	case c.msgConv != "":
		res.ConversationID = c.msgConv
	}
	switch {
	case c.ackSection != "":
		res.SectionID = c.ackSection
	case c.msgSection != "":
		res.SectionID = c.msgSection
	}
	if c.lastIndex > 0 {
		res.LastMessageIndex = c.lastIndex
	}
}

// findMapByKey 在嵌套 JSON 里按 key 找第一个对象（深度有限，避免遍历过大）。
func findMapByKey(v any, key string, depth int) map[string]any {
	if depth <= 0 {
		return nil
	}
	if list := asSlice(v); list != nil {
		for _, child := range list {
			if sub := findMapByKey(child, key, depth-1); sub != nil {
				return sub
			}
		}
		return nil
	}
	m := asMap(v)
	if m == nil {
		return nil
	}
	if inner := asMap(m[key]); inner != nil {
		return inner
	}
	for _, child := range m {
		if sub := findMapByKey(child, key, depth-1); sub != nil {
			return sub
		}
	}
	return nil
}

func extractConversation(raw string, res *Result) {
	if res.ConversationID == "" {
		if m := conversationIDRe.FindStringSubmatch(raw); m != nil {
			res.ConversationID = m[1]
		}
	}
	if res.SectionID == "" {
		if m := sectionIDRe.FindStringSubmatch(raw); m != nil {
			res.SectionID = m[1]
		}
	}
}

// lastIndexValue 让 last_message_index 在没有历史时是 null（和网页版一致）。
func lastIndexValue(n int) any {
	if n <= 0 {
		return nil
	}
	return n
}

// findMaxIntByKey 在嵌套 JSON 里找 key 的最大整数值（消息序号可能出现在多帧里）。
func findMaxIntByKey(v any, key string, depth int) int {
	if depth <= 0 {
		return 0
	}
	if list := asSlice(v); list != nil {
		max := 0
		for _, child := range list {
			if n := findMaxIntByKey(child, key, depth-1); n > max {
				max = n
			}
		}
		return max
	}
	m := asMap(v)
	if m == nil {
		return 0
	}
	max := 0
	if raw, ok := m[key]; ok {
		if n := toInt(raw); n > max {
			max = n
		}
	}
	for _, child := range m {
		if n := findMaxIntByKey(child, key, depth-1); n > max {
			max = n
		}
	}
	return max
}

// toInt 尽力把 JSON 数字/字符串转成 int。
func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}
