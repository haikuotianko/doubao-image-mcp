package config

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T, accounts ...Account) *Store {
	t.Helper()
	return &Store{
		cfg:  Config{Accounts: accounts, FitRatio: true},
		path: filepath.Join(t.TempDir(), "config.json"),
	}
}

func names(list []Account) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = a.Name
	}
	return out
}

// 拖动排序后，调度顺序应该就是列表顺序。
func TestReorderDrivesScheduleOrder(t *testing.T) {
	s := newTestStore(t, Account{Name: "a"}, Account{Name: "b"}, Account{Name: "c"})
	if err := s.ReorderAccounts([]string{"c", "a", "b"}); err != nil {
		t.Fatal(err)
	}
	got := names(s.EnabledAccounts())
	want := []string{"c", "a", "b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("调度顺序 = %v，期望 %v", got, want)
		}
	}
}

// 默认账号为空时按列表第一个；指定默认账号时它排最前。
func TestDefaultAccountOrdering(t *testing.T) {
	s := newTestStore(t, Account{Name: "a"}, Account{Name: "b"}, Account{Name: "c"})

	if got := names(s.EnabledAccounts())[0]; got != "a" {
		t.Fatalf("默认账号为空时应取第一个，实际 %s", got)
	}

	if err := s.SetActive("b"); err != nil {
		t.Fatal(err)
	}
	if got := names(s.EnabledAccounts()); got[0] != "b" {
		t.Fatalf("指定默认账号后应排最前，实际 %v", got)
	}
}

// 停用账号不参与调度；停用的是默认账号时，默认账号让给下一个启用的。
func TestDisableAccount(t *testing.T) {
	s := newTestStore(t, Account{Name: "a"}, Account{Name: "b"}, Account{Name: "c"})
	_ = s.SetActive("a")

	if err := s.SetAccountEnabled("a", false); err != nil {
		t.Fatal(err)
	}
	if got := s.Get().Active; got != "b" {
		t.Fatalf("停用默认账号后应退回下一个启用账号，实际 %q", got)
	}
	for _, a := range s.EnabledAccounts() {
		if a.Name == "a" {
			t.Fatal("停用的账号不该出现在调度列表里")
		}
	}
}

// 排序里混入不存在的名字时不能丢账号。
func TestReorderIgnoresUnknown(t *testing.T) {
	s := newTestStore(t, Account{Name: "a"}, Account{Name: "b"})
	if err := s.ReorderAccounts([]string{"ghost", "b"}); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Get().Accounts); got != 2 {
		t.Fatalf("账号数量应保持 2，实际 %d", got)
	}
	if got := names(s.Get().Accounts); got[0] != "b" {
		t.Fatalf("已知账号应排在前，实际 %v", got)
	}
}
