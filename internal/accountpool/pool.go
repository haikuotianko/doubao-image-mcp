// Package accountpool 把账号池变成「自动排队」的调度器。
//
// 规则：一个账号同一时刻只跑一个生图任务（豆包对并发不友好，容易被风控），
// 请求来了先挑空闲的已启用账号；全忙就排队等，不会直接失败。
package accountpool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"doubao-image-mcp/internal/config"
)

// Pool 是并发安全的账号调度器。
type Pool struct {
	accounts func() []config.Account

	mu      sync.Mutex
	busy    map[string]int
	waiting int
	wake    chan struct{}
}

// New 创建调度器；accounts 每次调度时现取，这样启用/停用立刻生效。
func New(accounts func() []config.Account) *Pool {
	return &Pool{
		accounts: accounts,
		busy:     make(map[string]int),
		wake:     make(chan struct{}, 1),
	}
}

// Acquire 取一个空闲账号；全部忙时排队等待，ctx 取消则放弃。
// 返回的 release 必须调用（建议 defer），否则这个账号会一直被占着。
func (p *Pool) Acquire(ctx context.Context) (config.Account, func(), error) {
	return p.wait(ctx, "")
}

// AcquireNamed 指定账号取号（界面上「用某个账号生成」用的）。
// 该账号被停用或不存在时立刻报错，不会退化成随便挑一个。
func (p *Pool) AcquireNamed(ctx context.Context, name string) (config.Account, func(), error) {
	if name == "" {
		return p.Acquire(ctx)
	}
	return p.wait(ctx, name)
}

// wait 是两种取号的公共排队逻辑：name 为空表示任意空闲账号。
func (p *Pool) wait(ctx context.Context, name string) (config.Account, func(), error) {
	for {
		// 调用方一般已经过滤过停用账号；这里再挡一层，保证「停用的绝不被调度」。
		all := p.accounts()
		accounts := make([]config.Account, 0, len(all))
		for _, a := range all {
			if a.Enabled() {
				accounts = append(accounts, a)
			}
		}

		p.mu.Lock()
		var target config.Account
		found := false
		if name != "" {
			for _, a := range accounts {
				if a.Name == name {
					target, found = a, true
					break
				}
			}
			if !found {
				p.mu.Unlock()
				return config.Account{}, nil, fmt.Errorf("账号 %s 已停用或不存在", name)
			}
		}
		free := false
		if found {
			free = p.busy[target.Name] == 0
		} else {
			for _, a := range accounts {
				if p.busy[a.Name] == 0 {
					target, found, free = a, true, true
					break
				}
			}
		}
		if free {
			p.busy[target.Name]++
			p.mu.Unlock()
			return target, func() { p.release(target.Name) }, nil
		}
		if len(accounts) == 0 {
			p.mu.Unlock()
			return config.Account{}, nil, errors.New("没有启用中的账号：请在「豆包生图 MCP」里登录并启用至少一个账号")
		}
		p.waiting++
		p.mu.Unlock()

		select {
		case <-ctx.Done():
			p.mu.Lock()
			p.waiting--
			p.mu.Unlock()
			return config.Account{}, nil, ctx.Err()
		case <-p.wake:
		case <-time.After(500 * time.Millisecond): // 兜底轮询，避免漏唤醒
		}

		p.mu.Lock()
		p.waiting--
		p.mu.Unlock()
	}
}

func (p *Pool) release(name string) {
	p.mu.Lock()
	if p.busy[name] > 0 {
		p.busy[name]--
	}
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Busy 返回每个账号当前在跑的任务数，给界面显示。
func (p *Pool) Busy() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]int, len(p.busy))
	for k, v := range p.busy {
		if v > 0 {
			out[k] = v
		}
	}
	return out
}

// Waiting 返回正在排队等账号的请求数。
func (p *Pool) Waiting() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waiting
}
