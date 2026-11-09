package doubao

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// samanthaMessage 构造旧通道的一条 SamantaImageInput 消息。
func samanthaMessage(text, ratio string, keys []string) map[string]any {
	content := map[string]any{"text": text}
	if ratio != "" && ratio != "auto" {
		content["ratio"] = ratio
	}
	contentJSON, _ := json.Marshal(content)

	attachments := []any{}
	for _, k := range keys {
		attachments = append(attachments, map[string]any{
			"type":  "image",
			"key":   k,
			"extra": map[string]any{"refer_types": "overall"},
		})
	}
	return map[string]any{
		"content":      string(contentJSON),
		"content_type": 2009, // SamanthaImageInput
		"attachments":  attachments,
		"references":   []any{},
		"skill": map[string]any{
			"skill_type":            3,
			"skill_type_no_default": 3,
			"skill_id":              "3",
			"skill_id_no_default":   "3",
		},
	}
}

// buildSamanthaPayload 构造 /samantha/chat/completion 的 body。
//
// memory_type 固定 0（保存到云端历史）：服务端只在存历史时才下发去水印模板
// （dwm3 / image_raw），阅后即焚只会给带水印的 lwm3。
func buildSamanthaPayload(msg map[string]any) map[string]any {
	return map[string]any{
		"messages": []any{msg},
		"completion_option": map[string]any{
			"is_regen":                 false,
			"with_suggest":             true,
			"need_create_conversation": true,
			"launch_stage":             1,
			"is_replace":               false,
			"is_delete":                false,
			"is_ai_playground":         false,
			"memory_type":              0,
			"message_from":             0,
			"use_deep_think":           false,
			"use_auto_cot":             false,
			"resend_for_regen":         false,
			"enable_commerce_credit":   false,
			"action_bar_skill_id":      3,
		},
		"evaluate_option":       map[string]any{"web_ab_params": ""},
		"local_conversation_id": uuid4(),
		"local_message_id":      uuid4(),
	}
}

// generateSamantha 走旧 agent 通道（无参考图时的降级路径）。
func (c *Client) generateSamantha(ctx context.Context, req Request, timeout time.Duration) (*Result, error) {
	return c.samanthaGenerate(ctx, req, nil, timeout)
}

// generateImageToImage 图生图：先上传参考图拿 uri，再作为附件走旧通道。
func (c *Client) generateImageToImage(ctx context.Context, req Request, timeout time.Duration) (*Result, error) {
	keys := make([]string, 0, len(req.RefImages))
	for _, path := range req.RefImages {
		key, err := c.UploadImage(ctx, path)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	res, err := c.samanthaGenerate(ctx, req, keys, timeout)
	if err != nil {
		return nil, err
	}
	res.Channel = "samantha(img2img)"
	return res, nil
}

// samanthaGenerate 发一次旧通道生图请求并解析结果。
func (c *Client) samanthaGenerate(ctx context.Context, req Request, keys []string, timeout time.Duration) (*Result, error) {
	text := withContextRule(req.Prompt, req.ContextMode)
	if req.Style != "" {
		text = text + "。图片风格为" + StyleName(req.Style)
	}
	msg := samanthaMessage(text, req.Ratio, keys)
	payload := buildSamanthaPayload(msg)

	raw, err := c.postBody(ctx, buildURL("/samantha/chat/completion", samanthaParams()), payload, timeout)
	if err != nil {
		return nil, err
	}

	res := &Result{Prompt: req.Prompt, Model: req.Model, Ratio: req.Ratio, Channel: "samantha"}
	parseSamanthaSSE(string(raw), res)
	if len(res.Images) == 0 {
		if res.Notice != "" {
			return nil, noticeErr("生成图片", res.Notice)
		}
		return nil, newErr(KindNoImage, "生成图片", "", "服务端没有返回图片", truncate(string(raw), 300))
	}
	return res, nil
}

// parseSamanthaSSE 解析旧通道的 SSE：event_type 2001 的 event_data 里是完整消息。
func parseSamanthaSSE(raw string, res *Result) {
	var noticeHits []string
	for _, ev := range parseSSE(raw) {
		if ev.Name == "gateway-error" {
			d := jsonObject(ev.Data)
			if d != nil {
				res.Notice = fmt.Sprint(d["message"])
			}
			continue
		}
		d := jsonObject(ev.Data)
		if d == nil {
			continue
		}
		switch asInt(d["event_type"]) {
		case 2005:
			res.Notice = truncate(fmt.Sprint(d["event_data"]), 300)
			continue
		case 2001:
		default:
			continue
		}

		eventData := d["event_data"]
		ed := asMap(eventData)
		if ed == nil {
			// event_data 通常是被二次编码的 JSON 字符串。
			ed = jsonObject(asString(eventData))
		}
		if ed == nil {
			continue
		}
		msg := asMap(ed["message"])
		if asInt(msg["content_type"]) != 2010 {
			continue
		}
		content := asMap(msg["content"])
		if content == nil {
			content = jsonObject(asString(msg["content"]))
		}
		for _, rawItem := range asSlice(content["data"]) {
			item := asMap(rawItem)
			if item == nil {
				continue
			}
			res.Images = append(res.Images, imageFromItem(item))
		}
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
		res.Notice = "当前账号无生图额度（服务端返回订阅提示：" +
			strings.Join(dedupeStrings(noticeHits), "、") + "），未出图。"
	}
}
