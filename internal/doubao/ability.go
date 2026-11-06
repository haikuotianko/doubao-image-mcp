package doubao

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// buildAbilityPayload 构造 /chat/completion 的 body（抓包真值，逐字段对齐前端）。
func buildAbilityPayload(prompt string, abilityType int, inner map[string]any, replyFormat string) map[string]any {
	nowMS := time.Now().UnixMilli()
	innerJSON, _ := json.Marshal(inner)
	return map[string]any{
		"client_meta": map[string]any{
			"local_conversation_id": fmt.Sprintf("local_%d", nowMS),
			"conversation_id":       "",
			"bot_id":                defaultBotID,
			"last_section_id":       "",
			"last_message_index":    nil,
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
			"need_create_conversation":  true,
			"conversation_init_option":  map[string]any{"need_ack_conversation": true},
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
			"conversation_init_option":      `{"need_ack_conversation":true}`,
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

	abilityText := req.Prompt
	if req.Style != "" {
		abilityText = req.Prompt + "。图片风格为" + StyleName(req.Style)
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

	payload := buildAbilityPayload(req.Prompt, 3, inner, replyImageText)
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
