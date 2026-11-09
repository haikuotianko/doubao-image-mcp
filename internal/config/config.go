// Package config 保存豆包生图 MCP 的本地配置与豆包账号池。
//
// 文件位置：%APPDATA%\doubao-image-mcp\config.json（Windows）。
// GUI 和 MCP 子进程读同一份文件，所以账号只需要登录一次。
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Account 是一个豆包网页登录态。
type Account struct {
	Name string `json:"name"`
	// Cookies 保存原始 cookie 键值（sessionid/ttwid/passport_csrf_token 等）。
	Cookies   map[string]string `json:"cookies"`
	AddedAt   time.Time         `json:"addedAt"`
	Remark    string            `json:"remark,omitempty"`
	LastCheck time.Time         `json:"lastCheck,omitempty"`
	LastError string            `json:"lastError,omitempty"`

	// Nickname / Avatar 是从豆包账号资料里取到的展示信息。
	// Avatar 存成 data URL：头像 CDN 带时效，存链接过几天就裂，存图本体才稳。
	Nickname string `json:"nickname,omitempty"`
	Avatar   string `json:"avatar,omitempty"`

	// Disabled 为 true 时该账号不参与自动排队。
	// 用「反向」字段是为了兼容老配置：老配置没有这个字段，零值 false 正好表示启用。
	Disabled bool `json:"disabled,omitempty"`

	// ChatID / ChatSection 是「复用同一个对话」用的会话标识（服务端生成，下次继续往里发）。
	// 换了账号自然是另一个对话，所以按账号存。
	ChatID      string `json:"chatId,omitempty"`
	ChatSection string `json:"chatSection,omitempty"`
	// ChatIndex 是对话里最后一条消息的序号，继续对话时带回（网页版也这么做）。
	ChatIndex int `json:"chatIndex,omitempty"`
}

// Display 返回展示用名字：优先昵称，其次内部标识。
func (a Account) Display() string {
	if strings.TrimSpace(a.Nickname) != "" {
		return a.Nickname
	}
	return a.Name
}

// Enabled 表示这个账号是否可被调度。
func (a Account) Enabled() bool { return !a.Disabled }

// SessionID 返回 sessionid（唯一判定“已登录”的凭证）。
func (a Account) SessionID() string { return a.Cookies["sessionid"] }

// CookieHeader 拼出 Cookie 请求头。
func (a Account) CookieHeader() string {
	keys := make([]string, 0, len(a.Cookies))
	for k, v := range a.Cookies {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+a.Cookies[k])
	}
	return strings.Join(parts, "; ")
}

// Config 是持久化配置。
type Config struct {
	Accounts []Account `json:"accounts"`
	Active   string    `json:"active"`

	OutDir        string `json:"outDir"`
	FitRatio      bool   `json:"fitRatio"`
	DefaultRatio  string `json:"defaultRatio"`
	DefaultModel  string `json:"defaultModel"`
	DefaultStyle  string `json:"defaultStyle"`
	HTTPAddr      string `json:"httpAddr"`
	AutoStartHTTP bool   `json:"autoStartHttp"`
	Debug         bool   `json:"debug"`

	// NoReuseChat 为 true 时每次生图都新建对话。
	// 默认是「复用同一个对话」，用反向字段才能让老配置（没有该字段）也默认开启。
	NoReuseChat bool `json:"noReuseChat,omitempty"`

	// ContextMode 是复用对话时的上下文隔离策略：none / always / auto。
	ContextMode string `json:"contextMode,omitempty"`
}

// ReuseChat 表示是否复用同一个对话生图（默认开启）。
func (c Config) ReuseChat() bool { return !c.NoReuseChat }

// Default 返回带默认值的配置。
func Default() Config {
	return Config{
		OutDir:        filepath.Join(userHome(), "Pictures", "doubao-image-mcp"),
		FitRatio:      true,
		DefaultRatio:  "1:1",
		DefaultModel:  "Seedream 5.0 Flash",
		HTTPAddr:      "127.0.0.1:8770",
		AutoStartHTTP: false,
		ContextMode:   "auto",
	}
}

func userHome() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}

// Dir 返回配置目录。
func Dir() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		base = userHome()
	}
	return filepath.Join(base, "doubao-image-mcp")
}

// Path 返回配置文件路径。
func Path() string { return filepath.Join(Dir(), "config.json") }

// Store 是配置的并发安全容器，写操作立即落盘。
type Store struct {
	mu   sync.RWMutex
	cfg  Config
	path string
}

// Load 读取配置；文件不存在时用默认配置并落盘一次，方便用户直接编辑。
func Load() (*Store, error) {
	s := &Store{cfg: Default(), path: Path()}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_ = s.saveLocked()
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &s.cfg); err != nil {
		return nil, err
	}
	s.cfg.applyDefaults()
	return s, nil
}

func (c *Config) applyDefaults() {
	d := Default()
	if c.OutDir == "" {
		c.OutDir = d.OutDir
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = d.HTTPAddr
	}
	if c.DefaultRatio == "" {
		c.DefaultRatio = d.DefaultRatio
	}
	if c.DefaultModel == "" {
		c.DefaultModel = d.DefaultModel
	}
	if c.ContextMode == "" {
		c.ContextMode = d.ContextMode
	}
}

// Get 返回配置副本。
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.cfg
	out.Accounts = append([]Account(nil), s.cfg.Accounts...)
	return out
}

// Update 修改配置并落盘。
func (s *Store) Update(fn func(*Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.cfg)
	s.cfg.applyDefaults()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ActiveAccount 返回当前账号；未指定 active 时返回第一个。
func (s *Store) ActiveAccount() (Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.cfg.Accounts) == 0 {
		return Account{}, false
	}
	for _, a := range s.cfg.Accounts {
		if a.Name == s.cfg.Active {
			return a, true
		}
	}
	return s.cfg.Accounts[0], true
}

// PutAccount 新增或同名覆盖账号，并设为当前账号。
func (s *Store) PutAccount(a Account) error {
	return s.Update(func(c *Config) {
		if a.AddedAt.IsZero() {
			a.AddedAt = time.Now()
		}
		for i := range c.Accounts {
			if c.Accounts[i].Name == a.Name {
				c.Accounts[i] = a
				c.Active = a.Name
				return
			}
		}
		c.Accounts = append(c.Accounts, a)
		c.Active = a.Name
	})
}

// RemoveAccount 删除账号。
func (s *Store) RemoveAccount(name string) error {
	return s.Update(func(c *Config) {
		kept := c.Accounts[:0]
		for _, a := range c.Accounts {
			if a.Name != name {
				kept = append(kept, a)
			}
		}
		c.Accounts = kept
		if c.Active == name {
			c.Active = ""
		}
	})
}

// SetActive 设置默认账号（空字符串表示「按列表顺序取第一个」）。
func (s *Store) SetActive(name string) error {
	return s.Update(func(c *Config) { c.Active = name })
}

// ReorderAccounts 按给定顺序重排账号；顺序就是生图调度的优先级。
// 传进来的名字必须和现有账号一一对应，否则忽略未知条目。
func (s *Store) ReorderAccounts(names []string) error {
	return s.Update(func(c *Config) {
		byName := make(map[string]Account, len(c.Accounts))
		for _, a := range c.Accounts {
			byName[a.Name] = a
		}
		ordered := make([]Account, 0, len(c.Accounts))
		seen := make(map[string]bool, len(names))
		for _, n := range names {
			if a, ok := byName[n]; ok && !seen[n] {
				ordered = append(ordered, a)
				seen[n] = true
			}
		}
		// 不在列表里的（理论上不会有）按原顺序补回，避免丢账号。
		for _, a := range c.Accounts {
			if !seen[a.Name] {
				ordered = append(ordered, a)
			}
		}
		c.Accounts = ordered
	})
}

// Account 按名字取一个账号。
func (s *Store) Account(name string) (Account, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.cfg.Accounts {
		if a.Name == name {
			return a, true
		}
	}
	return Account{}, false
}

// EnabledAccounts 返回参与调度的账号，当前账号排在最前，其余按添加顺序。
func (s *Store) EnabledAccounts() []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Account, 0, len(s.cfg.Accounts))
	for _, a := range s.cfg.Accounts {
		if a.Name == s.cfg.Active && a.Enabled() {
			out = append(out, a)
		}
	}
	for _, a := range s.cfg.Accounts {
		if a.Name != s.cfg.Active && a.Enabled() {
			out = append(out, a)
		}
	}
	return out
}

// SetAccountEnabled 启用 / 停用一个账号。
func (s *Store) SetAccountEnabled(name string, enabled bool) error {
	return s.Update(func(c *Config) {
		for i := range c.Accounts {
			if c.Accounts[i].Name == name {
				c.Accounts[i].Disabled = !enabled
				// 停用的是当前账号时，把当前账号让给别人，免得后续操作落到停用账号上。
				if !enabled && c.Active == name {
					c.Active = ""
					for _, a := range c.Accounts {
						if a.Enabled() {
							c.Active = a.Name
							break
						}
					}
				}
				return
			}
		}
	})
}

// UpdateProfile 更新账号的昵称与头像。
func (s *Store) UpdateProfile(name, nickname, avatar string) error {
	return s.Update(func(c *Config) {
		for i := range c.Accounts {
			if c.Accounts[i].Name == name {
				if nickname != "" {
					c.Accounts[i].Nickname = nickname
				}
				if avatar != "" {
					c.Accounts[i].Avatar = avatar
				}
				return
			}
		}
	})
}

// SetChat 记住这个账号当前使用的对话，下次生图继续往里发。
func (s *Store) SetChat(name, conversationID, sectionID string, lastMessageIndex int) error {
	if strings.TrimSpace(conversationID) == "" {
		return nil
	}
	return s.Update(func(c *Config) {
		for i := range c.Accounts {
			if c.Accounts[i].Name == name {
				c.Accounts[i].ChatID = conversationID
				c.Accounts[i].ChatSection = sectionID
				c.Accounts[i].ChatIndex = lastMessageIndex
				return
			}
		}
	})
}

// ClearChat 忘掉账号的对话记录（下次生图会新建对话）。
func (s *Store) ClearChat(name string) error {
	return s.Update(func(c *Config) {
		for i := range c.Accounts {
			if c.Accounts[i].Name == name {
				c.Accounts[i].ChatID = ""
				c.Accounts[i].ChatSection = ""
				c.Accounts[i].ChatIndex = 0
				return
			}
		}
	})
}
