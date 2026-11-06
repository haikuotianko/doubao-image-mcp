package accountpool

import (
	"context"
	"testing"
	"time"

	"doubao-image-mcp/internal/config"
)

// 两个账号：前两个请求立刻拿到，第三个必须排队等到有人释放。
func TestAcquireQueuesUntilRelease(t *testing.T) {
	accounts := []config.Account{{Name: "a"}, {Name: "b"}}
	p := New(func() []config.Account { return accounts })
	ctx := context.Background()

	_, rel1, err := p.Acquire(ctx)
	if err != nil {
		t.Fatalf("第一个: %v", err)
	}
	_, rel2, err := p.Acquire(ctx)
	if err != nil {
		t.Fatalf("第二个: %v", err)
	}
	if got := len(p.Busy()); got != 2 {
		t.Fatalf("应有两个账号在跑，实际 %d", got)
	}

	done := make(chan error, 1)
	go func() {
		_, rel3, err := p.Acquire(ctx)
		if err == nil {
			rel3()
		}
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("第三个不该立刻拿到账号（err=%v）", err)
	case <-time.After(200 * time.Millisecond):
		// 正确：在排队
	}

	rel1()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("释放后应拿到账号: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("释放后第三个请求没有被唤醒")
	}
	rel2()
}

// 指定账号取号：被停用/不存在时立刻报错，不会退化成随便挑一个。
func TestAcquireNamed(t *testing.T) {
	accounts := []config.Account{{Name: "a"}, {Name: "b", Disabled: true}}
	p := New(func() []config.Account { return accounts })

	acc, rel, err := p.AcquireNamed(context.Background(), "a")
	if err != nil || acc.Name != "a" {
		t.Fatalf("指定 a 应拿到 a，实际 %v %v", acc.Name, err)
	}
	rel()

	if _, _, err := p.AcquireNamed(context.Background(), "b"); err == nil {
		t.Fatal("停用的账号不该被取到")
	}
	if _, _, err := p.AcquireNamed(context.Background(), "ghost"); err == nil {
		t.Fatal("不存在的账号应报错")
	}
}

// 指定账号正忙时要排队等它，而不是换别的账号。
func TestAcquireNamedWaitsForSameAccount(t *testing.T) {
	accounts := []config.Account{{Name: "a"}, {Name: "b"}}
	p := New(func() []config.Account { return accounts })

	_, rel, err := p.AcquireNamed(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan string, 1)
	go func() {
		acc, r, err := p.AcquireNamed(context.Background(), "a")
		if err == nil {
			r()
		}
		done <- acc.Name
	}()

	select {
	case name := <-done:
		t.Fatalf("a 忙着时不该拿到别的账号（拿到 %q）", name)
	case <-time.After(200 * time.Millisecond):
	}

	rel()
	select {
	case name := <-done:
		if name != "a" {
			t.Fatalf("应拿到 a，实际 %q", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("释放后没有被唤醒")
	}
}

// 停用所有账号时要明确报错，不能死等。
func TestAcquireNoEnabledAccount(t *testing.T) {
	p := New(func() []config.Account { return nil })
	if _, _, err := p.Acquire(context.Background()); err == nil {
		t.Fatal("没有可用账号时应返回错误")
	}
}

// ctx 取消时排队中的请求要能退出。
func TestAcquireContextCancel(t *testing.T) {
	accounts := []config.Account{{Name: "a"}}
	p := New(func() []config.Account { return accounts })
	_, rel, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, _, err := p.Acquire(ctx); err == nil {
		t.Fatal("ctx 超时后应返回错误")
	}
}
