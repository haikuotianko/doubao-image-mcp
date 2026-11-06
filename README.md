# 豆包生图 MCP（doubao-image-mcp）

把**豆包网页版**（doubao.com）的生图能力封装成本地 MCP 服务：不需要官方 API Key，用你自己的网页登录态直接调用，Seedream 4.0 / 4.5 / 5.0 全系可用。附带一个桌面端（Wails + WebView2），负责扫码登录、账号池、测试生图与排障。

> Local MCP server that turns Doubao's web image generation into tools for Claude / Cursor / any MCP client — no API key, just your own web session. Windows 10/11 only.

- **扫码登录**：内置 WebView2 窗口，豆包 App 扫码即可；`sessionid` 是 HttpOnly，只能从浏览器 Cookie 里取，所以必须用内嵌浏览器。
- **会话可见**：账号行点「会话」会把**该账号的 cookie** 灌进浏览器再打开豆包——登录态还在不在，一眼看到，而不是靠猜。
- **多图全量**：一次生成 1~4 张就返回几张；GUI 里点任意一张看大图，可另存到任意目录。
- **两种传输**：stdio（客户端拉起进程）与 streamable HTTP（常驻，多客户端共用），共用同一套 4 个工具。
- **排障内置**：登录失败会在窗口里写明原因（网络 / DNS / 代理 / 渲染进程崩溃 / 运行时过旧），`-diagnose` 一键出环境报告。

> ⚠️ **非官方项目**：这里调用的是豆包**网页版**接口，属于逆向实现，随时可能因服务端调整而失效。仅供个人学习与自用，请遵守豆包用户协议；请不要用它批量刷额度、对外提供服务或分发他人账号。作者不提供任何可用性或稳定性保证。

## 环境要求

- **Windows 10 / 11（x64）**。目前仅支持 Windows：扫码登录窗口是 Win32 + WebView2 实现，macOS / Linux 下编译不过（`internal/loginwin` 是 Windows 专用）。
- [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)：Win11 自带；Win10 若缺失，登录窗口打不开（程序会明确提示）。
- 豆包账号一个（免费账号即可用 Seedream 5.0 Flash）。

## 快速开始

1. 运行 `doubao-image-mcp.exe`。
2. 点右上角「扫码登录」，用豆包 App 扫码，窗口自动关闭。
3. 在「设置」里选好输出目录（默认 `图片\doubao-image-mcp`）。
4. 在想用的 MCP 客户端里按下面二选一配置，重启客户端即可。

### 方式一：stdio（客户端自己拉起进程，推荐）

**别照抄下面的路径**——GUI 的「MCP 服务」卡片会按当前这台机器的真实位置生成可直接粘贴的 JSON，`doubao-image-mcp.exe -diagnose` 也会打印同一份配置。

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

`-mcp` 时日志写在 `%APPDATA%\doubao-image-mcp\mcp.log`，stdout 只走 JSON-RPC，不会污染协议。

### 方式二：本地 HTTP（常驻服务，可被多个客户端共用）

在 GUI 里「MCP 服务 → 启动」，或命令行：

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

> 这个 HTTP 接口**没有鉴权**，等于「谁连上谁就能花你的额度」。默认只绑 `127.0.0.1`，不要改成 `0.0.0.0` 或局域网地址。

## 工具

| 工具 | 作用 |
| --- | --- |
| `doubao_image` | 文生图 / 图生图。**一次生成几张就返回几张**：本地路径 + 图片 CDN 链接 |
| `doubao_models` | 列出模型、风格、比例 |
| `doubao_ping` | 检查当前账号登录态是否有效 |
| `doubao_accounts` | 列出 / 切换 / **启用·停用** / 删除账号，输出里带昵称、启用状态、是否正在生成 |

`doubao_image` 参数：

- `prompt`（必填）
- `ratio`：`1:1` `4:3` `3:4` `16:9` `9:16` `3:2` `2:3`，或 `auto`（默认）
- `model`：`Seedream 5.0 Flash`（免费，推荐）、`Seedream 4.5`、`Seedream 4.0`、`Seedream 5.0 Pro`（需订阅）；留空用配置里的默认值
- `style`：32 种预设风格，key 列表见 `doubao_models`
- `ref_images`：参考图本地绝对路径数组，传了就走图生图
- `out_dir` / `fit_ratio` / `timeout_seconds` / `inline`
  - `fit_ratio` 默认开启：服务端忽略请求比例时，用模糊背景补齐到目标比例（偏差 3% 内不动）
  - `inline` 为 true 时把图片本体以 base64 塞进 MCP 响应，供能直接显示图片的客户端使用

多图说明：服务端一次可能给 1~4 张（提示词里写「生成 4 张不同风格」就容易触发）。返回文本是「共 N 张：」加每张一行的路径，`images` 数组也是 N 项。解析按图的稳定标识归并，结构化解析漏了也会从 SSE 原文里按 `image_raw` 直链补回，不会只出第一张。

## 多账号与自动排队

可以登录任意多个豆包账号。**加账号就再点一次账号卡片里的「扫码登录」**：程序会先清空浏览器里的旧登录态、重新出二维码，并且把已有账号的 `sessionid` 排除在外，所以不会出现「一打开就显示已登录、第二个号扫不进去」的情况。每个账号：

- 列表里显示**头像 + 昵称**（登录成功后自动抓取，抓不到就用昵称首字兜底）；
- 有独立的**启用开关**，停用的账号不参与调度，但仍可手动查看会话；
- 有独立的**「会话」按钮**，用该账号的 cookie 打开豆包，单独确认它的登录态；
- **拖动可以调整顺序**，顺序就是生图时的优先级。

「设置」里的**默认账号**决定优先用哪个号（留空则用列表里第一个启用的账号）；「测试生图」上方也能单独指定这次用哪个账号，默认「自动（任意空闲账号）」。两个下拉都只列**启用中**的账号，停用的不会出现在里面，后端也会拒绝对停用账号的指定。

生图请求（MCP 与 GUI 测试共用同一个池）按下面的规则走：

1. 默认账号优先，其余按列表顺序，挑一个**空闲**的已启用账号；
2. 每个账号同一时刻只跑一个任务（豆包对并发不友好，容易踩风控）；
3. 全忙就**排队等待**，不会直接报错；排队数会显示在顶部徽标和 `doubao_accounts` 输出里；
4. 一个可调度账号都没有时，明确报「没有启用中的账号」。

> 注意：排队只在**同一个进程内**生效。stdio 模式下每个 MCP 客户端会拉起自己的进程，所以多个客户端之间不共享队列；要严格串行就用 HTTP 模式（一个进程服务所有客户端）。

## 数据位置

- 配置与账号：`%APPDATA%\doubao-image-mcp\config.json`（**里面有明文 cookie，别外传**）
- WebView2 登录态：`%APPDATA%\doubao-image-mcp\webview`
- 运行日志：`%APPDATA%\doubao-image-mcp\app.log`（超过 2MB 滚成 `app.log.1`）
- 默认输出：`%USERPROFILE%\Pictures\doubao-image-mcp`

## 排查：扫码窗口白屏 / 打不开

登录窗口是原生 WebView2，出问题基本是运行时、代理或网络三类。程序已经把这些情况写进窗口和日志，不用再猜：

1. 加载失败时窗口里直接显示原因（连不上服务器 / DNS 失败 / 代理要认证 / 渲染进程崩溃 等），不再是白屏。
2. 页面「加载成功但整页空白」（老 WebView2 运行时跑不动豆包前端）会被探针识别并提示。
3. 让对方跑一次自检，把报告发回来：

   ```powershell
   .\doubao-image-mcp.exe -diagnose
   ```

   报告落在 `%APPDATA%\doubao-image-mcp\diag-<时间>.txt`，含 Windows 版本、WebView2 运行时版本、配置目录可写性、代理设置、DNS/TCP/HTTPS 连通性，以及按当前机器路径生成好的 stdio 配置。WebView2 运行时低于 100 时登录页大概率渲染不出来。

只开登录窗口（不启动主界面）方便定位：

```powershell
.\doubao-image-mcp.exe -login                        # 用默认地址，日志写 app.log
.\doubao-image-mcp.exe -login -login-url about:blank  # 换个地址试，验证窗口本身是否正常
```

## 查看会话

账号行点「会话」会新开一个浏览器窗口，**先把该账号的 cookie 写进 WebView2** 再打开豆包，所以：

- 看到的就是 MCP 正在用的那份登录态（不是浏览器 profile 里遗留的旧登录）；
- 页面停在聊天页说明还活着，被踢回登录页说明 `sessionid` 失效了，直接在里面重新扫码即可；
- 这个窗口只读不写：不轮询 cookie、不覆盖账号配置。

命令行等价：`.\doubao-image-mcp.exe -view`，日志里会记「已写入 N/M 项会话 cookie」和页面标题。

## 已知限制

- 走的是网页版接口，**没有官方保证**；豆包改接口就可能失效。
- 免费账号有额度限制，超额时工具会返回「当前账号无该模型的创作额度」。
- 触发 IP 风控（错误码 `710022002` / `710022004`）时，需要冷却约 30 分钟、换网络，或去网页版手动过一次滑块。
- 图生图（`ref_images`）走旧的 samantha 通道，比文生图脆弱：2026-10 实测部分账号带附件会返回 `710020702`「系统错误」（同一通道纯文生图仍正常）。遇到就先用文生图，或换账号 / 稍后重试。
- 服务端有时忽略请求的比例。默认的 `fit_ratio` 会用虚化背景把图补齐到目标比例，关掉就保留原图。
- 仅 Windows。

## 开发

```powershell
wails build        # 产出 build\bin\doubao-image-mcp.exe
go build ./...     # 只编译后端
wails dev          # 热重载开发
```

代码结构：

```
main.go                  多种启动模式（GUI / -mcp / -http / -diagnose / -login / -view）
diagnose.go              环境自检报告（运行时版本、目录、代理、连通性）
app.go                   Wails 绑定：设置、账号、扫码登录、测试生图、图片另存、落盘日志
internal/config          配置与账号持久化
internal/doubao          豆包协议客户端（SSE 解析、多图归并、图生图、上传、比例修正）
internal/loginwin        原生 Win32 + WebView2 登录窗口（取 HttpOnly cookie、错误页、空白探针）
internal/mcpserver       MCP 工具定义（stdio / streamable HTTP）
frontend/                Vanilla JS + vite
```

调试小技巧：设 `DOUBAO_MCP_DUMP_SSE=<文件路径>` 会把这次生成的原始 SSE 写到该文件，方便服务端改结构时对齐字段。

## 许可证

[MIT](LICENSE)。软件按「现状」提供，不附带任何明示或暗示的担保。
