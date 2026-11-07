package update

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v1.1.0", "v1.2.0", true},
		{"v1.2.0", "v1.1.0", false},
		{"v1.1.0", "v1.1.0", false},
		{"v1.1.0", "v1.1.1", true},
		{"1.1.0", "v1.1.1", true},     // 前缀可有可无
		{"v1.1", "v1.1.1", true},      // 两段版本号
		{"v1.2.0", "v1.2.0-beta", false}, // 预发布不算更新
		{"v1.2.0-beta", "v1.2.0", true},  // 预发布转正算更新
		{"dev", "v9.9.9", false},      // 本地开发版不提示
		{"", "v9.9.9", false},
		{"v1.1.0", "not-a-version", false},
		{"v1.10.0", "v1.9.0", false}, // 数字比较，不是字符串比较
		{"v1.9.0", "v1.10.0", true},
	}
	for _, c := range cases {
		if got := IsNewer(c.current, c.latest); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestParse(t *testing.T) {
	if sv, ok := parse("v1.2.3"); !ok || sv.num != [3]int{1, 2, 3} || sv.pre != "" {
		t.Fatalf("parse(v1.2.3) = %+v %v", sv, ok)
	}
	if sv, ok := parse("2.0.0-rc.1+build5"); !ok || sv.num != [3]int{2, 0, 0} || sv.pre != "rc.1" {
		t.Fatalf("parse(2.0.0-rc.1+build5) = %+v %v", sv, ok)
	}
	if _, ok := parse("dev"); ok {
		t.Fatal("dev 不应该被解析成版本号")
	}
}
