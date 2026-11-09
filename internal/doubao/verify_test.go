package doubao

import "testing"

func TestVerifyChallenge(t *testing.T) {
	// 服务端风控时会给 extra.decision（JSON 字符串），type=verify 表示要求过验证
	decision := `{"type":"verify","verify_scene":"slider","url":"https://www.doubao.com/verify?token=abc"}`

	need, raw, url := verifyChallenge(map[string]any{
		"extra": map[string]any{"decision": decision},
	})
	if !need {
		t.Fatal("type=verify 时应该判定为需要验证")
	}
	if raw != decision {
		t.Errorf("应保留 decision 原文，得到 %q", raw)
	}
	if url != "https://www.doubao.com/verify?token=abc" {
		t.Errorf("应提取出验证页地址，得到 %q", url)
	}
}

func TestVerifyChallengeNegative(t *testing.T) {
	cases := []map[string]any{
		nil,
		{},
		{"extra": map[string]any{}},
		{"extra": map[string]any{"decision": `{"type":"block"}`}},
		{"extra": map[string]any{"decision": "not-json"}},
	}
	for _, c := range cases {
		if need, _, _ := verifyChallenge(c); need {
			t.Errorf("不该判定为需要验证：%#v", c)
		}
	}
}

func TestAttachVerifyOnlyForRisk(t *testing.T) {
	decision := `{"type":"verify"}`
	d := map[string]any{"extra": map[string]any{"decision": decision}}

	// 非风控错误不挂验证信息
	loginErr := attachVerify(newErr(KindLogin, "生成图片", "710012001", "登录失效", ""), d)
	if loginErr.NeedVerify {
		t.Error("登录类错误不该标记需要验证")
	}

	riskErr := attachVerify(newErr(KindRisk, "生成图片", "710022004", "风控", ""), d)
	if !riskErr.NeedVerify {
		t.Error("风控错误应标记需要验证")
	}
	if riskErr.Hint() == "" {
		t.Error("风控错误应给出处置建议")
	}
}
