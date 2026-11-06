package config

import "testing"

func TestParseCookies(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantSID  string
		wantTTW  string
		wantMore bool
	}{
		{
			name:    "裸 sessionid",
			in:      "7f3c0d5e9a2b4c6d8e0f1a2b3c4d5e6f",
			wantSID: "7f3c0d5e9a2b4c6d8e0f1a2b3c4d5e6f",
		},
		{
			name:    "Cookie 请求头",
			in:      "sessionid=abc123; ttwid=1%7CtW; s_v_web_id=verify_x",
			wantSID: "abc123",
			wantTTW: "1%7CtW",
		},
		{
			name:    "带 Cookie 前缀和换行",
			in:      "Cookie: sessionid=abc123;\r\n ttwid=1%7CtW",
			wantSID: "abc123",
			wantTTW: "1%7CtW",
		},
		{
			name:    "cURL 的 -H cookie",
			in:      `curl 'https://www.doubao.com/chat/' -H 'accept: */*' -H 'cookie: sessionid=abc123; ttwid=1%7CtW'`,
			wantSID: "abc123",
			wantTTW: "1%7CtW",
		},
		{
			name:    "cURL 的 -b",
			in:      `curl 'https://www.doubao.com/chat/' -b 'sessionid=abc123; ttwid=1%7CtW'`,
			wantSID: "abc123",
			wantTTW: "1%7CtW",
		},
		{
			name:    "值里带等号",
			in:      "sessionid=abc=123; ttwid=xyz=",
			wantSID: "abc=123",
			wantTTW: "xyz=",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseCookies(c.in)
			if got["sessionid"] != c.wantSID {
				t.Fatalf("sessionid = %q，期望 %q（全部：%v）", got["sessionid"], c.wantSID, got)
			}
			if c.wantTTW != "" && got["ttwid"] != c.wantTTW {
				t.Fatalf("ttwid = %q，期望 %q", got["ttwid"], c.wantTTW)
			}
		})
	}
}

func TestParseCookiesEmpty(t *testing.T) {
	if got := ParseCookies("   "); len(got) != 0 {
		t.Fatalf("空输入应返回空表，实际 %v", got)
	}
}
