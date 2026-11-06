# 豆包生图 MCP（doubao-image-mcp）

把**豆包网页版**（doubao.com）的生图能力封装成本地 MCP 服务：**不需要官方 API Key**，用你自己的网页登录态直接调用，Seedream 4.0 / 4.5 / 5.0 全系可用。附带一个桌面端（Wails + WebView2），负责扫码登录、多账号管理、测试生图与排障。

> Local MCP server that exposes Doubao's web image generation as tools for Claude Desktop / Cursor / any MCP client — no API key, just your own web session.

![界面](docs/screenshot.png)

## 目录

- [它能做什么](#它能做什么)
- [环境要求](#环境要求)
- [安装](#安装)
- [首次使用](#首次使用)
- [接入 MCP 客户端](#接入-mcp-客户端)
- [工具参考](#工具参考)
- [多账号与自动排队](#多账号与自动排队)
- [数据位置](#数据位置)
- [排障](#排障)
- [已知限制](#已知限制)
- [跨平台说明](#跨平台说明)
- [开发](#开发)

## 它能做什么

MCP 对外提供 4 个工具：

| 工具 | 作用 |
| --- | --- |
| `doubao_image` | 文生图 / 图生图。**一次生成几张就返回几张**：本地文件路径 + CDN 链接 + 尺寸 |
| `doubao_models` | 列出可用模型、32 种风格 key、8 种比例 |
| `doubao_ping` | 检查当前账号登录态是否有效（也能发现 IP 风控） |
| `doubao_accounts` | 账号池：列出 / 切换默认 / 启用 / 停用 / 删除，输出带昵称与忙碌状态 |

几个实现上的点，用起来能感觉到：

- **无水印直链**：从 SSE 里挑 `image_raw` / `dwm3` 模板地址（手工改后缀会 400，签名覆盖整个 path，只能用服务端给的）。
- **多图全量**：服务端一次可能给 1~4 张，全部返回；结构化解析漏掉的图会从原文按图补回。
- **比例修正**：服务端经常忽略请求比例，默认用虚化背景补齐到目标比例（偏差 3% 内不动）。
- **双通道降级**：新端点 `/chat/completion` 失败自动降级到旧的 samantha 通道。

## 环境要求

- **Windows 10 / 11（x64）** —— 当前唯一完整支持的平台，原因见[跨平台说明](#跨平台说明)。
- [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)：Win11 自带；Win10 若缺失，登录窗口打不开（程序会明确提示并给出日志）。
- 一个豆包账号（免费账号就能用 Seedream 5.0 Flash）。

## 安装

### 方式一：下载 Release（推荐）

1. 到 [Releases](../../releases) 下载 `doubao-image-mcp-vX.Y.Z-windows-amd64.zip`
2. 解压到任意目录（路径可以含中文和空格）
3. 双击 `doubao-image-mcp.exe`

程序是单文件，不需要安装。配置和账号写在 `%APPDATA%\doubao-image-mcp`，所以移动/改名 exe 不会丢账号。

### 方式二：从源码构建

需要 Go 1.22+、Node.js 18+、[Wails CLI](https://wails.io/docs/gettingstarted/installation)：

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
git clone <本仓库>
cd doubao-image-mcp
wails build          # 产出 build\bin\doubao-image-mcp.exe
```

只改后端时可以 `go build ./...` 快速验证编译。

## 首次使用

1. 打开程序，点账号卡片右上角的 **「扫码登录」**。
2. 弹出内置浏览器窗口，用豆包 App 扫码，窗口自动关闭。
3. 账号出现在列表里，带**头像 + 昵称**（登录成功后自动抓取）。
4. 在「设置」里选好输出目录（默认 `图片\doubao-image-mcp`）、默认模型/比例/风格。

**加第二个账号**：再点一次「扫码登录」。程序会先清空浏览器里的旧登录态、重新出二维码，并且把已有账号的 `sessionid` 排除在外，所以不会出现「一打开就显示已登录、第二个号扫不进去」的情况。

**排序与默认账号**：账号行可以拖动排序（顺序就是生图优先级）；「设置 → 默认账号」可以指定优先用哪个号，留空则用列表里第一个启用的账号。停用的账号不参与调度，也不会出现在两个账号下拉里。

## 接入 MCP 客户端

客户端配置里的路径**不要手抄**：GUI 的「MCP 服务」卡片会按当前机器的真实位置生成好可直接粘贴的 JSON，`doubao-image-mcp.exe -diagnose` 也会打印同一份。

### 方式一：stdio（推荐，客户端自己拉起进程）

```json
{
  "mcpServers": {
    "doubao": {
      "command": "C:\\Users\\<你>\\Downloads\\doubao-image-mcp.exe",
      "args": ["-mcp"]
    }
  }
}
```

- **Claude Desktop**：改 `%APPDATA%\Claude\claude_desktop_config.json`，重启客户端。
- **Cursor**：`Settings → MCP → Add new MCP server`，选 `command` 类型，命令填 `<路径>\doubao-image-mcp.exe -mcp`。
- **Cherry Studio / 其他**：选「stdio」传输，命令和参数同上。

`-mcp` 模式下日志写到 `%APPDATA%\doubao-image-mcp\mcp.log`，stdout 只走 JSON-RPC，不会污染协议。

### 方式二：本地 HTTP（常驻，多客户端共用）

在 GUI 里点「启动 HTTP」，或命令行：

```powershell
.\doubao-image-mcp.exe -http 127.0.0.1:8770   # 不带地址则用配置里的地址
```

客户端配置：

```json
{
  "mcpServers": {
    "doubao": {
      "type": "http",
      "url": "http://127.0.0.1:8770/mcp"
    }
  }
}
```

> ⚠️ 这个 HTTP 接口**没有鉴权**，等于「谁连上谁就能花你的额度」。默认只绑 `127.0.0.1`，不要改成 `0.0.0.0` 或局域网地址。

### 怎么确认接上了

在客户端里问一句「用 doubao_ping 看看豆包登录态」，返回 `账号 xxx 登录态正常` 就通了。返回「还没有登录任何豆包账号」说明账号不在配置里，回 GUI 扫码即可。

## 工具参考

### `doubao_image` —— 生成图片

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `prompt` | string | ✅ | 画面描述，越具体越好 |
| `ratio` | string | | `1:1` `4:3` `3:4` `16:9` `9:16` `3:2` `2:3` `auto`；留空用配置默认 |
| `model` | string | | `Seedream 5.0 Flash`（免费，推荐）/ `Seedream 4.5` / `Seedream 4.0` / `Seedream 5.0 Pro`（需订阅） |
| `style` | string | | 风格 `option_key`，32 种，用 `doubao_models` 查 |
| `ref_images` | string[] | | 本地绝对路径数组，传了就走**图生图** |
| `out_dir` | string | | 保存目录；留空用配置里的输出目录 |
| `fit_ratio` | bool | | 是否裁剪/补边到请求比例；留空跟随配置（默认开） |
| `timeout_seconds` | int | | 超时秒数，默认 180 |
| `inline` | bool | | 为 true 时把图片本体以 base64 放进 MCP 响应（体积大），默认只给路径/链接 |

返回的文本是「共 N 张：」加每张一行路径；结构化输出：

```json
{
  "prompt": "海边日落，电影感",
  "model": "Seedream 5.0 Flash",
  "ratio": "16:9",
  "channel": "ability",
  "notice": "",
  "images": [
    {
      "path": "C:\\Users\\me\\Pictures\\doubao-image-mcp\\doubao_20261106_153012_1.png",
      "url": "https://p3-flow-imagex-sign.byteimg.com/...",
      "width": 2048,
      "height": 2048,
      "format": "png"
    }
  ]
}
```

调用示例（任何 MCP 客户端替我发这个请求）：

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "doubao_image",
    "arguments": {
      "prompt": "赛博朋克风格的重庆夜景，霓虹灯、雨夜、电影感",
      "ratio": "16:9",
      "model": "Seedream 5.0 Flash",
      "style": "image_gen_style_cyberpunk"
    }
  }
}
```

图生图就是多传参考图：

```json
{
  "name": "doubao_image",
  "arguments": {
    "prompt": "把这张照片改成水彩画风格",
    "ref_images": ["C:\\Users\\me\\Pictures\\cat.jpg"],
    "ratio": "1:1"
  }
}
```

对 AI 说话就行，不用手写 JSON：

> 用 doubao_image 画一张「雪山日落，16:9，电影感」
> 用 doubao_image 把这四张图分别改成水彩风格：`C:\pics\1.jpg` …

### `doubao_models` —— 查模型 / 风格 / 比例

无参数。返回：

```json
{
  "models": [{"name": "Seedream 5.0 Flash", "sub": "专业快速（免费，推荐）", "needUpgrade": false}],
  "defaultModel": "Seedream 5.0 Flash",
  "styles": [{"key": "skill_image_styles_film", "name": "电影写真"}],
  "ratios": ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3", "auto"]
}
```

### `doubao_ping` —— 检查登录态

无参数。返回 `{ok, account, message}`。风控（`710022002` / `710022004`）和登录失效都在 `message` 里说明。

### `doubao_accounts` —— 账号池

| 参数 | 说明 |
| --- | --- |
| `action` | `list`（默认）/ `switch` / `enable` / `disable` / `remove` |
| `name` | `switch`、`enable`、`disable`、`remove` 要操作的账号名 |

返回包含 `active`（默认账号）、`waiting`（排队请求数）和账号列表：

```json
{
  "active": "doubao-1106-152143",
  "waiting": 0,
  "accounts": [
    {"name": "doubao-1106-152143", "nickname": "hai_lucky", "enabled": true, "busy": false, "active": true}
  ]
}
```

## 多账号与自动排队

生图请求（MCP 与 GUI 测试共用同一个池）按下面的规则走：

1. **默认账号优先**，其余按列表顺序，挑一个**空闲**的已启用账号；
2. 每个账号同一时刻只跑一个任务（豆包对并发不友好，容易踩风控）；
3. 全忙就**排队等待**，不会直接失败；排队数显示在顶部徽标和 `doubao_accounts` 的 `waiting` 里；
4. 一个可调度账号都没有时，明确报「没有启用中的账号」。

> 排队只在**同一个进程内**生效。stdio 模式下每个 MCP 客户端会拉起自己的进程，多个客户端之间不共享队列；要严格串行就用 HTTP 模式（一个进程服务所有客户端）。

## 数据位置

| 内容 | 路径 |
| --- | --- |
| 配置与账号 | `%APPDATA%\doubao-image-mcp\config.json`（**里面有明文 cookie，别外传**） |
| WebView2 登录态 | `%APPDATA%\doubao-image-mcp\webview` |
| 运行日志 | `%APPDATA%\doubao-image-mcp\app.log`（超过 2MB 滚成 `app.log.1`） |
| stdio 日志 | `%APPDATA%\doubao-image-mcp\mcp.log` |
| 默认输出 | `%USERPROFILE%\Pictures\doubao-image-mcp` |

## 排障

### 扫码窗口白屏 / 打不开

登录窗口是原生 WebView2，出问题基本是运行时、代理或网络三类。程序会把这些情况写进窗口和日志：

1. 加载失败时窗口里直接显示原因（连不上服务器 / DNS 失败 / 代理要认证 / 渲染进程崩溃），不再是白屏。
2. 页面「加载成功但整页空白」（老 WebView2 运行时跑不动豆包前端）会被探针识别并提示。
3. 跑一次自检，把报告发回来：

   ```powershell
   .\doubao-image-mcp.exe -diagnose
   ```

   报告落在 `%APPDATA%\doubao-image-mcp\diag-<时间>.txt`，含 Windows 版本、WebView2 运行时版本、配置目录可写性、代理设置、DNS/TCP/HTTPS 连通性，以及按当前机器路径生成好的 stdio 配置。

### 其它命令行开关

```powershell
.\doubao-image-mcp.exe -login                  # 只开登录窗口（日志写 app.log）
.\doubao-image-mcp.exe -login -login-url about:blank   # 换个地址，验证窗口本身是否正常
.\doubao-image-mcp.exe -view                   # 用默认账号的会话开浏览器窗口
.\doubao-image-mcp.exe -diagnose               # 环境自检
```

### 生成失败

| 现象 | 原因与处理 |
| --- | --- |
| 「当前账号无该模型的创作额度」 | 免费额度用完了，换模型或换账号 |
| 错误码 `710022002` / `710022004` | IP 风控，冷却约 30 分钟 / 换网络 / 去网页版过一次滑块 |
| 图生图返回 `710020702`「系统错误」 | 旧通道脆弱，先用文生图或换账号稍后重试 |
| 返回「没有启用中的账号」 | 账号被停用了，去 GUI 打开开关 |

## 已知限制

- 走的是**网页版接口**，属于逆向实现，没有官方保证；豆包改接口就可能失效。
- 免费账号有额度限制。
- 图生图（`ref_images`）走旧的 samantha 通道，比文生图脆弱。
- 服务端有时忽略请求的比例；默认的 `fit_ratio` 会用虚化背景补齐，关掉就保留原图。
- **仅 Windows**（见下）。

## 跨平台说明

Go 本身当然跨平台，这个项目里 **MCP 主体也是纯 Go**（`internal/doubao` 协议客户端、`internal/mcpserver` 工具定义、`internal/accountpool` 调度、`internal/config` 持久化），没有任何平台绑定。

卡住的是这三处，都在 Windows 专有 API 上：

| 位置 | 依赖 | 换到 macOS / Linux 要做什么 |
| --- | --- | --- |
| `internal/loginwin` | Win32 窗口 + WebView2 的 CookieManager（`//go:build windows`） | 换成 WKWebView（mac）/ WebKitGTK（Linux），或改为「手动粘贴 sessionid」 |
| `diagnose.go` | `golang.org/x/sys/windows`、注册表 | 拆成 `_windows.go` / `_unix.go` 两份实现 |
| `app.go` 的「打开所在文件夹」 | `explorer.exe` | 换 `open` / `xdg-open` |

所以在 macOS / Linux 上目前 `go build` 会在 `loginwin` 和 `x/sys/windows` 上断掉。要让它在 mac 上跑起来，最小改动是给登录窗口加一个「手动导入 cookie」通道（把 `sessionid` 填进 `config.json` 即可被 MCP 使用）。欢迎 PR —— 但请附上能跑的验证，我这里只有 Windows 环境。

## 开发

```powershell
wails build        # 产出 build\bin\doubao-image-mcp.exe
wails dev          # 热重载
go build ./...     # 只编译后端
go test ./internal/...   # 账号池 / 配置的单元测试
```

代码结构：

```
main.go                  启动模式（GUI / -mcp / -http / -diagnose / -login / -view）
diagnose.go              环境自检报告
app.go                   Wails 绑定：设置、账号、扫码登录、测试生图、图片另存、落盘日志
internal/config          配置与账号持久化（账号池顺序、启用状态、昵称头像）
internal/accountpool     账号调度：按优先级取空闲账号，全忙排队
internal/doubao          豆包协议客户端（SSE 解析、多图归并、图生图、上传、比例修正、资料抓取）
internal/loginwin        原生 Win32 + WebView2 登录窗口（取 HttpOnly cookie、错误页、空白探针）
internal/mcpserver       MCP 工具定义（stdio / streamable HTTP）
frontend/                Vanilla JS + vite
```

调试小技巧：设 `DOUBAO_MCP_DUMP_SSE=<文件路径>` 会把这次生成的原始 SSE 写到该文件，方便服务端改结构时对齐字段。

## 许可证与免责

[MIT](LICENSE)。软件按「现状」提供，不附带任何明示或暗示的担保。

> ⚠️ **非官方项目**：这里调用的是豆包**网页版**接口，属于逆向实现，随时可能因服务端调整而失效。仅供个人学习与自用，请遵守豆包用户协议；请不要用它批量刷额度、对外提供服务或分发他人账号。
