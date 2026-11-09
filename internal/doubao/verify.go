package doubao

import "strings"

// verifyChallenge 从 STREAM_ERROR 的 payload 里判断是否要求人机验证。
//
// 服务端在风控时会给一个 extra.decision（JSON 字符串），其中 type == "verify"
// 表示需要先过验证（豆包网页版里就是那个滑块弹层，由 verifycenter SDK 渲染）。
// 返回：是否需要验证、decision 原文、以及能从里面找到的验证页地址（可能为空）。
func verifyChallenge(d map[string]any) (need bool, decision, verifyURL string) {
	extra := asMap(d["extra"])
	if extra == nil {
		return false, "", ""
	}
	decision = asString(extra["decision"])
	if decision == "" {
		return false, "", ""
	}
	dec := jsonObject(decision)
	if dec == nil {
		return false, decision, ""
	}
	if asString(dec["type"]) != "verify" {
		return false, decision, ""
	}
	return true, decision, firstURL(dec, 0)
}

// firstURL 在嵌套 JSON 里找第一个 http(s) 链接（验证页地址字段名不固定，只能这么捞）。
func firstURL(v any, depth int) string {
	if depth > 6 {
		return ""
	}
	switch val := v.(type) {
	case string:
		if strings.HasPrefix(val, "http://") || strings.HasPrefix(val, "https://") {
			return val
		}
	case map[string]any:
		for _, child := range val {
			if u := firstURL(child, depth+1); u != "" {
				return u
			}
		}
	case []any:
		for _, child := range val {
			if u := firstURL(child, depth+1); u != "" {
				return u
			}
		}
	}
	return ""
}

// attachVerify 把验证信息挂到错误上（只对风控类错误有意义）。
func attachVerify(err *Error, d map[string]any) *Error {
	if err == nil || err.Kind != KindRisk {
		return err
	}
	need, decision, url := verifyChallenge(d)
	if need {
		err.NeedVerify = true
		err.VerifyURL = url
		err.VerifyData = decision
	}
	return err
}
