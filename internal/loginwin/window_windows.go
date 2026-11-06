//go:build windows

// Package loginwin 弹一个自带 WebView2 的原生窗口，让用户扫码/账号登录豆包，
// 登录成功后通过 CookieManager 把 cookie 取出来。
//
// 为什么要自建窗口：sessionid 是 HttpOnly，页面 JS 读不到；只有 WebView2 的
// CookieManager 能拿到。窗口跑在独占的锁定线程上，cookie 轮询也放在同一个线程，
// 因为 WebView2 的 COM 对象不保证跨线程调用安全。
package loginwin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

// RuntimeVersion 返回本机 WebView2 运行时版本，空字符串表示没装或读不到。
func RuntimeVersion() string {
	v, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	if err != nil {
		return ""
	}
	return v
}

// MinRuntimeVersion 是豆包网页版能正常渲染的底线（老运行时里页面白屏/报错）。
const MinRuntimeVersion = "100.0.0.0"

// RuntimeTooOld 判断运行时是否低于可用的下限。
func RuntimeTooOld(version string) bool {
	if version == "" {
		return true
	}
	cmp, err := webviewloader.CompareBrowserVersions(version, MinRuntimeVersion)
	if err != nil {
		return false
	}
	return cmp < 0
}

// DefaultURL 是登录入口（未登录会自动跳登录页，登录后回到对话页）。
const DefaultURL = "https://www.doubao.com/chat/"

// keepCookies 是登录后需要保留的 cookie（sessionid 是核心，其余提高兼容性）。
var keepCookies = []string{
	"sessionid",
	"ttwid",
	"s_v_web_id",
	"i18next",
	"passport_csrf_token",
	"passport_csrf_token_default",
	"flow_user_country",
	"flow_cur_user_sec_id",
	"dbx-web-theme",
	"conversation_list_v2_group_mode",
	"msToken",
}

// Options 控制登录窗口外观与数据目录。
type Options struct {
	Title    string
	Width    int
	Height   int
	DataPath string
	URL      string
	// Logf 非空时接收进度日志。
	Logf func(format string, args ...any)
	// PollInterval 是 cookie 轮询间隔，默认 1.5s。
	PollInterval time.Duration
	// ViewOnly 为真时只显示页面，不轮询 cookie（用来看当前会话，不会误判登录成功）。
	ViewOnly bool
	// Cookies 非空时在导航前写进 WebView2，保证窗口里看到的就是这份会话。
	Cookies map[string]string
	// ClearCookies 为真时先清空浏览器登录态。
	// 加第二个账号必须开：否则窗口一打开就是上一个账号的已登录页面，会被立刻判成“登录成功”。
	ClearCookies bool
	// SkipSessions 是「已有账号的 sessionid → 账号名」，命中不算登录成功。
	SkipSessions map[string]string
}

// ErrClosed 表示窗口在登录完成前被关掉了。
var ErrClosed = errors.New("登录窗口已关闭")

type outcome struct {
	cookies map[string]string
	err     error
}

// Window 是登录窗口句柄。
type Window struct {
	opts     Options
	hwnd     uintptr
	chromium *edge.Chromium

	// cookieHandler 非空表示有一次异步取 cookie 还在飞行中（保持对象存活，等回调）。
	cookieHandler *cookiesHandler
	cookieAt      time.Time
	polls         int
	destroyed     atomic.Bool

	// navOK / navErrAt 记录页面是否加载成功，给「一直白屏」兜底用。
	navOK     atomic.Bool
	navErr    atomic.Value // string
	startedAt time.Time
	timeoutAt atomic.Bool
	probed    atomic.Bool

	// skipSIDs：已有账号的 sessionid → 账号名，命中不算本次登录成功。
	skipSIDs map[string]string
	// skippedLog 保证「命中了旧会话」只记一次日志。
	skippedLog sync.Once

	ready   chan error
	result  chan outcome
	closeMu sync.Once
}

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procPeekMessage      = user32.NewProc("PeekMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procPostMessage      = user32.NewProc("PostMessageW")
)

const (
	wsOverlappedWindow = 0x00CF0000
	// WS_CLIPCHILDREN：不裁剪子窗口的话，父窗口擦背景会把 WebView2 的内容整片盖成白色。
	wsClipChildren = 0x02000000
	cwUseDefault   = 0x80000000
	swShow         = 5
	idcArrow       = 32512
	pmRemove       = 0x0001
	wmDestroy      = 0x0002
	wmSize         = 0x0005
	wmClose        = 0x0010
	wmDpiChanged   = 0x02E0
	csHRedraw      = 0x0002
	csVRedraw      = 0x0001
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type point struct {
	x, y int32
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

// 窗口过程是包级函数，用 hwnd 映射回窗口对象。
var (
	registryMu sync.Mutex
	registry   = map[uintptr]*Window{}
	classOnce  sync.Once
	classErr   error
	className  = "DoubaoImageMcpLoginWindow"
)

func moduleHandle() windows.Handle {
	ret, _, _ := procGetModuleHandle.Call(0)
	return windows.Handle(ret)
}

func (w *Window) logf(format string, args ...any) {
	if w.opts.Logf != nil {
		w.opts.Logf(format, args...)
	}
}

// Open 打开登录窗口并等待 WebView2 初始化完成。
func Open(opts Options) (*Window, error) {
	if opts.Title == "" {
		opts.Title = "登录豆包"
	}
	if opts.Width <= 0 {
		opts.Width = 520
	}
	if opts.Height <= 0 {
		opts.Height = 780
	}
	if opts.URL == "" {
		opts.URL = DefaultURL
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 1500 * time.Millisecond
	}
	if opts.DataPath == "" {
		base := os.Getenv("APPDATA")
		if base == "" {
			base = os.TempDir()
		}
		opts.DataPath = filepath.Join(base, "doubao-image-mcp", "webview")
	}

	w := &Window{
		opts:   opts,
		ready:  make(chan error, 1),
		result: make(chan outcome, 1),
	}

	// 运行时缺失时 go-webview2 会直接 os.Exit(1) 把整个程序带走，先拦下来。
	if v := RuntimeVersion(); v == "" {
		return nil, errors.New("未检测到 WebView2 运行时，无法打开登录窗口。请安装 Microsoft Edge WebView2 Runtime（常青版）后重试")
	} else {
		w.logf("WebView2 运行时版本: %s", v)
		if RuntimeTooOld(v) {
			w.logf("警告: WebView2 运行时版本过旧（低于 %s），豆包登录页可能白屏，建议更新 Edge/WebView2", MinRuntimeVersion)
		}
	}

	// 已有账号的 sessionid：清 cookie 有延迟，命中这些也不能算本次登录成功。
	if len(opts.SkipSessions) > 0 {
		w.skipSIDs = make(map[string]string, len(opts.SkipSessions))
		for sid, name := range opts.SkipSessions {
			if sid != "" {
				w.skipSIDs[sid] = name
			}
		}
	}

	go w.run()

	if err := <-w.ready; err != nil {
		return nil, err
	}
	return w, nil
}

// WaitLogin 阻塞到用户登录成功（返回 cookie）或窗口被关闭。
func (w *Window) WaitLogin(ctx context.Context) (map[string]string, error) {
	select {
	case out := <-w.result:
		return out.cookies, out.err
	case <-ctx.Done():
		w.Close()
		return nil, ctx.Err()
	}
}

// Close 关掉窗口（幂等）。
func (w *Window) Close() {
	w.closeMu.Do(func() {
		registryMu.Lock()
		hwnd := w.hwnd
		registryMu.Unlock()
		if hwnd != 0 {
			procPostMessage.Call(hwnd, wmClose, 0, 0)
		}
	})
}

func (w *Window) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// WebView2 的控制器是 COM 对象，这条协程独占的线程必须先初始化成 STA。
	// （go-webview2 的 init() 只在进程主线程做过一次，这里不能省。）
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		w.logf("CoInitializeEx 警告: %v", err)
	} else {
		defer windows.CoUninitialize()
	}

	hwnd, err := w.createWindow()
	if err != nil {
		w.ready <- err
		w.result <- outcome{err: err}
		return
	}
	registryMu.Lock()
	w.hwnd = hwnd
	registry[hwnd] = w
	registryMu.Unlock()
	defer func() {
		registryMu.Lock()
		delete(registry, hwnd)
		registryMu.Unlock()
	}()

	// WebView2 出错时 go-webview2 会直接 os.Exit(1)，必须先接住错误
	// （真正致命的那几种错误在 Open 里已经提前拦掉了）。
	w.chromium = edge.NewChromium()
	w.chromium.DataPath = w.opts.DataPath
	w.chromium.AdditionalBrowserArgs = []string{"--disable-features=msWebOOUI,msPdfOOUI"}
	w.chromium.SetErrorCallback(func(err error) {
		w.logf("WebView2 错误: %v", err)
	})
	// 回调要在 Embed 之前挂上：这些注册发生在创建控制器的时候。
	w.chromium.NavigationCompletedCallback = w.onNavigationCompleted
	w.chromium.ProcessFailedCallback = w.onProcessFailed
	w.chromium.MessageCallback = w.onWebMessage

	if !w.chromium.Embed(hwnd) {
		embedErr := errors.New("WebView2 初始化失败（可能未安装 WebView2 运行时）")
		w.ready <- embedErr
		w.result <- outcome{err: embedErr}
		return
	}
	w.chromium.Show()
	// Embed 创建控制器时没有设置 bounds，而窗口在 Embed 之前就已经显示完，
	// 之后不会再有 WM_SIZE；不主动 Resize 一次，内容区就是 0×0 的白窗口。
	w.chromium.Resize()
	// 探针：页面「加载成功但渲染成空白」时（老 WebView2 运行时常见）也能报出来。
	w.chromium.Init(blankProbeScript)
	// 查看会话时先把配置里的 cookie 灌进浏览器，看到的就是这个账号的状态，
	// 而不是 WebView2 profile 里残留的旧登录。
	if len(w.opts.Cookies) > 0 {
		w.seedCookies(w.opts.Cookies)
	}
	// 登录新账号：先清空浏览器登录态，否则页面直接是上一个账号的已登录状态。
	if w.opts.ClearCookies {
		w.clearCookies()
	}
	w.startedAt = time.Now()
	w.chromium.Navigate(w.opts.URL)
	w.logf("登录窗口已打开: %s", w.opts.URL)

	// 通知 Open 可以返回，然后进入自己的消息循环。
	w.ready <- nil

	cookies, ok := w.messageLoop()
	procDestroyWindow.Call(hwnd)
	w.chromium.ShuttingDown()

	if ok {
		w.result <- outcome{cookies: cookies}
	} else {
		w.result <- outcome{err: ErrClosed}
	}
}

func (w *Window) createWindow() (uintptr, error) {
	classOnce.Do(func() {
		cursor, _, _ := procLoadCursor.Call(0, idcArrow)
		name, _ := syscall.UTF16PtrFromString(className)
		wc := wndClassEx{
			style:         csHRedraw | csVRedraw,
			lpfnWndProc:   windows.NewCallback(wndProc),
			hInstance:     moduleHandle(),
			hCursor:       windows.Handle(cursor),
			hbrBackground: 0, // 不擦背景，避免盖住 WebView2
			lpszClassName: name,
		}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		if ret, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); ret == 0 {
			classErr = fmt.Errorf("RegisterClassExW 失败: %v", callErr)
		}
	})
	if classErr != nil {
		return 0, classErr
	}

	title, _ := syscall.UTF16PtrFromString(w.opts.Title)
	class, _ := syscall.UTF16PtrFromString(className)
	hwnd, _, callErr := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(class)),
		uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow|wsClipChildren,
		cwUseDefault, cwUseDefault,
		uintptr(w.opts.Width), uintptr(w.opts.Height),
		0, 0,
		uintptr(moduleHandle()),
		0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW 失败: %v", callErr)
	}
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	return hwnd, nil
}

// messageLoop 抽窗口消息并定期检查登录 cookie（同一线程内完成，避免 COM 跨线程）。
func (w *Window) messageLoop() (map[string]string, bool) {
	ticker := time.NewTicker(w.opts.PollInterval)
	defer ticker.Stop()

	var m msg
	for {
		if w.destroyed.Load() {
			return nil, false
		}
		for {
			ret, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
			if ret == 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
			if m.message == wmDestroy {
				return nil, false
			}
		}

		if w.cookieHandler != nil {
			select {
			case res := <-w.cookieHandler.result:
				w.cookieHandler = nil
				if res.ok {
					w.logf("已取到登录 cookie（%d 项）", len(res.cookies))
					return res.cookies, true
				}
				w.polls++
				if res.skipped != "" {
					w.skippedLog.Do(func() {
						w.logf("当前还是账号 %s 的会话，等待扫码新账号（旧登录态已清空）", res.skipped)
					})
					continue
				}
				if w.polls <= 3 {
					w.logf("第 %d 次取 cookie：共 %d 项，匹配 %d 项，尚未登录", w.polls, res.total, len(res.cookies))
				}
			default:
			}
		}

		select {
		case <-ticker.C:
			// 一直没加载成功就把原因显示到窗口里，别让用户对着白屏干等。
			if !w.navOK.Load() && !w.timeoutAt.Swap(true) && !w.startedAt.IsZero() &&
				time.Since(w.startedAt) > 25*time.Second {
				reason, _ := w.navErr.Load().(string)
				if reason == "" {
					reason = "页面 25 秒内没有加载完成"
				}
				w.logf("登录页长时间未就绪: %s", reason)
				w.showErrorPage(reason)
			}
			// 回调迟迟不来就丢弃这次请求，免得永远轮询不下去。
			// 查看会话模式下不轮询：看到的是配置里的 cookie，不该被判成「登录成功」。
			if w.opts.ViewOnly {
				continue
			}
			if w.cookieHandler != nil && time.Since(w.cookieAt) > 5*time.Second {
				w.cookieHandler = nil
			}
			if w.cookieHandler == nil {
				w.requestCookies()
			}
		default:
			time.Sleep(15 * time.Millisecond)
		}
	}
}

// cookieURL 决定取哪些站点的 cookie。
const cookieURL = "https://www.doubao.com"

// ICoreWebView2CookieManager::GetCookies 是异步接口，第二个参数是完成回调对象。
// go-webview2 v1.0.22 把它当成同步接口（传了个出参指针），直接调用会 AV 崩溃，
// 所以这里自己按 vtable 槽位调用，并实现一个最小的完成回调 COM 对象。
const getCookiesVtblIndex = 5 // IUnknown(3) + CreateCookie + CopyCookie

type cookiesHandler struct {
	vtbl   *getCookiesHandlerVtbl
	refs   int32
	result chan cookieResult
	// skip 是「已有账号的 sessionid → 账号名」，命中就不算登录成功。
	skip map[string]string
}

// cookieResult 是一次异步取 cookie 的结果。
type cookieResult struct {
	cookies map[string]string
	names   []string
	total   int
	ok      bool
	// skipped 非空表示命中了已有账号的会话（不是这次要登录的新账号）。
	skipped string
}

type getCookiesHandlerVtbl struct {
	queryInterface edge.ComProc
	addRef         edge.ComProc
	release        edge.ComProc
	invoke         edge.ComProc
}

const sOK = uintptr(0)

var cookiesHandlerVtbl = &getCookiesHandlerVtbl{
	queryInterface: edge.NewComProc(func(this, refiid, out uintptr) uintptr {
		*(*uintptr)(unsafe.Pointer(out)) = this
		cookiesAddRef(this)
		return sOK
	}),
	addRef: edge.NewComProc(func(this uintptr) uintptr { return cookiesAddRef(this) }),
	release: edge.NewComProc(func(this uintptr) uintptr {
		h := (*cookiesHandler)(unsafe.Pointer(this))
		return uintptr(atomic.AddInt32(&h.refs, -1))
	}),
	// 回调里就把列表读干净：列表对象只在 Invoke 期间保证有效。
	invoke: edge.NewComProc(func(this, hr, list uintptr) uintptr {
		h := (*cookiesHandler)(unsafe.Pointer(this))
		var res cookieResult
		if hr == 0 && list != 0 {
			res.cookies, res.names, res.total, res.ok, res.skipped = readCookieList(
				(*edge.ICoreWebView2CookieList)(unsafe.Pointer(list)), h.skip)
		}
		select {
		case h.result <- res:
		default:
		}
		return sOK
	}),
}

func cookiesAddRef(this uintptr) uintptr {
	h := (*cookiesHandler)(unsafe.Pointer(this))
	return uintptr(atomic.AddInt32(&h.refs, 1))
}

// comProcAt 从 COM 对象的 vtable 里取出第 index 个函数指针。
func comProcAt(obj unsafe.Pointer, index uintptr) edge.ComProc {
	vtbl := *(*unsafe.Pointer)(obj)
	entry := (*uintptr)(unsafe.Add(vtbl, index*unsafe.Sizeof(uintptr(0))))
	return edge.ComProc(*entry)
}

// seedCookies 把配置里的 cookie 写进 WebView2，这样「查看会话」看到的就是
// 配置里正在用的那个账号，而不是浏览器 profile 里某次登录留下的残留。
func (w *Window) seedCookies(cookies map[string]string) {
	manager, err := w.chromium.GetCookieManager()
	if err != nil || manager == nil {
		w.logf("写入会话 cookie 失败: %v", err)
		return
	}
	defer manager.Release()

	ok := 0
	for name, value := range cookies {
		cookie, err := manager.CreateCookie(name, value, ".doubao.com", "/")
		if err != nil || cookie == nil {
			continue
		}
		if err := manager.AddOrUpdateCookie(cookie); err == nil {
			ok++
		}
		cookie.Release()
	}
	w.logf("已写入 %d/%d 项会话 cookie", ok, len(cookies))
}

// clearCookies 清空浏览器登录态，让登录页重新出现二维码。
//
// DeleteAllCookies 是异步的（没有回调），清完还要抽一会儿消息再导航，
// 否则页面可能拿到旧 cookie 直接进聊天页——这是「登录第二个账号」失败的主因。
func (w *Window) clearCookies() {
	manager, err := w.chromium.GetCookieManager()
	if err != nil || manager == nil {
		w.logf("清理登录态失败: %v", err)
		return
	}
	defer manager.Release()
	if err := manager.DeleteAllCookies(); err != nil {
		w.logf("清理登录态失败: %v", err)
		return
	}
	w.pump(700 * time.Millisecond)
	w.logf("已清空浏览器登录态，等待扫码新账号")
}

// pump 在指定时间内抽消息，让 WebView2 的异步操作有机会完成。
func (w *Window) pump(d time.Duration) {
	var m msg
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		for {
			ret, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
			if ret == 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}
		time.Sleep(15 * time.Millisecond)
	}
}

// requestCookies 发起一次异步取 cookie；结果通过 w.cookieHandler.result 回来。
func (w *Window) requestCookies() {
	if w.chromium == nil {
		return
	}
	manager, err := w.chromium.GetCookieManager()
	if err != nil || manager == nil {
		return
	}
	defer manager.Release()

	uri, err := syscall.UTF16PtrFromString(cookieURL)
	if err != nil {
		return
	}
	handler := &cookiesHandler{vtbl: cookiesHandlerVtbl, refs: 1, result: make(chan cookieResult, 1), skip: w.skipSIDs}
	w.cookieHandler = handler
	w.cookieAt = time.Now()

	getCookies := comProcAt(unsafe.Pointer(manager), getCookiesVtblIndex)
	hr, _, _ := getCookies.Call(
		uintptr(unsafe.Pointer(manager)),
		uintptr(unsafe.Pointer(uri)),
		uintptr(unsafe.Pointer(handler)),
	)
	if hr != 0 {
		w.logf("GetCookies 调用失败: 0x%x", hr)
		w.cookieHandler = nil
	}
}

// readCookieList 把回调拿到的列表转成 map；只有拿到 sessionid 才算登录成功。
// skip 里是已有账号的 sessionid：命中了说明页面还是旧会话，不能当成本次登录成功。
// 列表对象由 WebView2 持有，只在回调期间有效，所以这里不 Release，也必须当场读完。
func readCookieList(list *edge.ICoreWebView2CookieList, skip map[string]string) (map[string]string, []string, int, bool, string) {
	if list == nil {
		return nil, nil, 0, false, ""
	}
	count, err := list.GetCount()
	if err != nil {
		return nil, nil, 0, false, ""
	}
	want := make(map[string]bool, len(keepCookies))
	for _, name := range keepCookies {
		want[name] = true
	}
	found := make(map[string]string, len(keepCookies))
	names := make([]string, 0, count)
	for i := uint32(0); i < count; i++ {
		cookie, err := list.GetItem(i)
		if err != nil || cookie == nil {
			continue
		}
		name, _ := cookie.GetName()
		if name != "" {
			names = append(names, name)
		}
		if want[name] {
			if v, err := cookie.GetValue(); err == nil && v != "" {
				found[name] = v
			}
		}
		cookie.Release()
	}
	sid := found["sessionid"]
	if sid == "" {
		return nil, names, int(count), false, ""
	}
	if owner, ok := skip[sid]; ok {
		// 已登录账号的会话：清 cookie 还没生效，或者用户就在这个账号上打转。
		return nil, names, int(count), false, owner
	}
	return found, names, int(count), true, ""
}

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	registryMu.Lock()
	w := registry[hwnd]
	registryMu.Unlock()

	switch message {
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		// WM_DESTROY 是在 WM_CLOSE 的窗口过程里同步派发的，外层消息循环看不到这条消息，
		// 只能靠这个标志位通知它退出。
		if w != nil {
			w.destroyed.Store(true)
		}
		procPostQuitMessage.Call(0)
		return 0
	case wmSize, wmDpiChanged:
		if w != nil && w.chromium != nil {
			w.chromium.Resize()
		}
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}
