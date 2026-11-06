//go:build windows

package loginwin

import (
	"fmt"
	"html"
	"strings"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
)

// COM 虚表槽位：IUnknown 的 3 个方法之后才是接口自己的方法。
const (
	navArgsGetIsSuccess      = 3
	navArgsGetWebErrorStatus = 4
)

// onNavigationCompleted 在导航结束时回调（就在本窗口的消息循环线程里）。
func (w *Window) onNavigationCompleted(_ *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
	if args == nil {
		return
	}
	ok, status := navResult(args)
	if ok {
		if w.navOK.CompareAndSwap(false, true) {
			w.logf("登录页加载完成: %s", w.opts.URL)
		} else {
			w.logf("页面导航完成")
		}
		return
	}
	// 被后一次导航顶掉是正常现象，不当作失败。
	if status == webErrOperationCanceled && w.navOK.Load() {
		return
	}
	reason := webErrorText(status)
	w.navErr.Store(reason)
	w.logf("登录页加载失败: %s（status=%d, %s）", reason, status, w.opts.URL)
	w.showErrorPage(reason)
}

// onProcessFailed 处理渲染进程崩溃——不接的话也是白屏。
func (w *Window) onProcessFailed(_ *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
	kind := "未知原因"
	if args != nil {
		if k, err := args.GetProcessFailedKind(); err == nil {
			kind = processFailedText(k)
		}
	}
	w.logf("登录页渲染进程异常: %s", kind)
	w.showErrorPage("页面渲染进程崩溃：" + kind)
}

// showErrorPage 把出错原因写进窗口里，避免用户只看到一片白。
func (w *Window) showErrorPage(reason string) {
	if w.chromium == nil || w.destroyed.Load() {
		return
	}
	w.chromium.NavigateToString(errorPageHTML(reason, w.opts.URL))
}

// navResult 读 ICoreWebView2NavigationCompletedEventArgs 的 IsSuccess / WebErrorStatus。
// 这版 go-webview2 没导出这两个 getter，只能自己按槽位调。
func navResult(args *edge.ICoreWebView2NavigationCompletedEventArgs) (bool, int) {
	obj := unsafe.Pointer(args)
	var success int32
	hr, _, _ := comProcAt(obj, navArgsGetIsSuccess).Call(uintptr(obj), uintptr(unsafe.Pointer(&success)))
	if int32(hr) < 0 {
		return false, -1
	}
	var status int32
	hr, _, _ = comProcAt(obj, navArgsGetWebErrorStatus).Call(uintptr(obj), uintptr(unsafe.Pointer(&status)))
	if int32(hr) < 0 {
		return success != 0, -1
	}
	return success != 0, int(status)
}

const webErrOperationCanceled = 10

// webErrorText 是 COREWEBVIEW2_WEB_ERROR_STATUS 的中文说明。
func webErrorText(status int) string {
	switch status {
	case 0:
		return "未知错误"
	case 1:
		return "证书域名不匹配"
	case 2:
		return "证书已过期"
	case 3:
		return "客户端证书错误"
	case 4:
		return "连不上服务器"
	case 5:
		return "连接超时"
	case 6:
		return "服务器返回了无效响应"
	case 7:
		return "网络已断开"
	case 8:
		return "无法建立连接"
	case 9:
		return "域名解析失败（DNS）"
	case 10:
		return "导航被取消"
	case 11:
		return "重定向失败"
	case 13:
		return "需要身份验证"
	case 14:
		return "代理需要身份验证"
	case 15:
		return "不安全的跳转"
	case 16:
		return "URL 格式错误"
	case 17:
		return "不支持的协议"
	default:
		return fmt.Sprintf("加载失败（错误码 %d）", status)
	}
}

func processFailedText(kind edge.COREWEBVIEW2_PROCESS_FAILED_KIND) string {
	switch kind {
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_BROWSER_PROCESS_EXITED:
		return "浏览器进程退出"
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_RENDER_PROCESS_EXITED:
		return "渲染进程退出（常见于内存不足或显卡驱动异常）"
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_RENDER_PROCESS_UNRESPONSIVE:
		return "渲染进程无响应"
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_FRAME_RENDER_PROCESS_EXITED:
		return "子框架渲染进程退出"
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_UTILITY_PROCESS_EXITED:
		return "工具进程退出"
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_SANDBOX_HELPER_PROCESS_EXITED:
		return "沙箱进程退出"
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_GPU_PROCESS_EXITED:
		return "显卡进程退出"
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_PPAPI_PLUGIN_PROCESS_EXITED:
		return "插件进程退出"
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_PPAPI_BROKER_PROCESS_EXITED:
		return "插件代理进程退出"
	default:
		return "进程异常"
	}
}

// blankProbeScript 在页面文档创建时注入：8 秒后回报 body 是不是真的空的。
// 加它是因为「导航成功但整页空白」（老运行时跑不动豆包的前端）从外部完全看不出来。
const blankProbeScript = `
(function () {
  var probe = 'doubaoMcpProbe:';
  function report() {
    try {
      var b = document.body;
      var empty = !b || (b.childElementCount === 0 && (b.innerText || '').trim() === '');
      window.chrome.webview.postMessage(probe + (empty ? 'empty' : 'ok') + '|' + (document.title || ''));
    } catch (e) {
      window.chrome.webview.postMessage(probe + 'ok|');
    }
  }
  function delay() { setTimeout(report, 8000); }
  if (document.readyState === 'complete') { delay(); }
  else { window.addEventListener('load', delay); }
})();
`

// onWebMessage 接收注入脚本的回报（是否空白 + 页面标题）。
func (w *Window) onWebMessage(message string, _ *edge.ICoreWebView2, _ *edge.ICoreWebView2WebMessageReceivedEventArgs) {
	const prefix = "doubaoMcpProbe:"
	if !strings.HasPrefix(message, prefix) || w.probed.Swap(true) {
		return
	}
	body := strings.TrimPrefix(message, prefix)
	state, title, _ := strings.Cut(body, "|")
	if title != "" {
		w.logf("登录页标题: %s", title)
	}
	if state == "empty" {
		w.logf("登录页加载完成但内容为空（多半是 WebView2 运行时过旧，跑不动豆包前端）")
		w.showErrorPage("页面加载出来了，但整页是空白的")
	}
}

// errorPageHTML 是加载失败时显示在窗口里的说明页。
func errorPageHTML(reason, url string) string {
	runtime := RuntimeVersion()
	if runtime == "" {
		runtime = "检测不到"
	}
	old := ""
	if RuntimeTooOld(runtime) {
		old = `<p class="warn">当前 WebView2 运行时版本偏低（低于 100），豆包登录页很可能渲染不出来，请先更新「Microsoft Edge」或安装最新的 WebView2 运行时。</p>`
	}
	return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">
<style>
  body { margin:0; padding:36px 28px; background:#101E3C; color:#E8EEF9;
         font:14px/1.7 "Microsoft YaHei", "Segoe UI", sans-serif; }
  h1 { font-size:20px; margin:0 0 8px; }
  .reason { color:#FFB4A2; font-size:16px; margin:0 0 20px; }
  .meta { color:#8FA3C7; font-size:12px; word-break:break-all; }
  ul { padding-left:20px; margin:16px 0; }
  li { margin:6px 0; }
  .warn { color:#FFD48A; }
</style></head><body>
<h1>登录页没能加载出来</h1>
<p class="reason">` + html.EscapeString(reason) + `</p>
<p class="meta">目标地址：` + html.EscapeString(url) + `<br>WebView2 运行时：` + html.EscapeString(runtime) + `</p>
` + old + `
<ul>
  <li>在浏览器里打开 <b>https://www.doubao.com</b>，确认这台机器能正常访问（稍后可以用同一个网络回这个窗口重试）。</li>
  <li>如果公司网络/代理有限制，先关掉代理或 VPN 再试。</li>
  <li>到「设置 → 应用」里搜 Microsoft Edge WebView2 Runtime，更新到最新版。</li>
  <li>关掉本窗口，回到主界面重新点「扫码登录」；仍失败就把 <b>%APPDATA%\doubao-image-mcp\app.log</b> 发给我。</li>
</ul>
</body></html>`
}
