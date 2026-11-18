# 发帖文案（v1.4.0）

配图都在 `build/promo/`：`cover.png`（横幅）、`gui.png`（界面）、`multi.png`（一次 4 张）、`features.png`（能力清单）。

---

## 1. V2EX · 分享创造

**标题**：`[分享] 把豆包网页版生图接进 MCP，不需要 API Key`

**正文**：

```
一直用豆包画图，但每次都要开网页、贴提示词、再手动存图，来回切很烦。
所以写了个 MCP 服务，让 Claude Desktop / Cursor 这类客户端直接调用豆包的网页版生图。

几个自己踩过的点：

1. 不需要 API Key。走的是你自己的网页登录态（扫码一次），底层是网页版的
   /chat/completion 接口。
2. 无水印。服务端在 SSE 里会下发 image_raw / dwm3 这类去水印模板地址，
   签名覆盖整个 path，只能用它给的，手改后缀会 400。
3. 一次 1~4 张全返回。服务端一个请求可能出多张，结构化解析漏掉的会从原文按图补回。
4. 复用同一个对话。以前每调一次就多一个会话，现在首次记住 conversation_id，
   后续都追加进去；失败或对话失效会自动新建。还配了「上下文隔离」三档，避免新图被上文干扰。
5. 被风控（710022004）时可以一键弹窗拖滑块。豆包这块用的是字节的 verifycenter，
   直接把窗口开出来让你手动过，比干等半小时强。

Windows 是带 GUI 的（扫码登录、多账号排队、测试生图），mac/Linux 是命令行版。
Go 写的，单文件，不需要额外运行时。

项目地址：https://github.com/haikuotianko/doubao-image-mcp
官网（有下载）：https://haikuotianko.github.io/doubao-image-mcp/

说明：非官方实现，逆向的是网页版接口，随时可能因服务端调整失效；免费账号有额度限制。
仅供个人学习自用，请遵守豆包用户协议，不要拿来批量刷额度或对外提供服务。

配图：cover.png + gui.png + multi.png
```

---

## 2. 掘金 · 技术文章

**标题**：`逆向豆包网页版生图接口，封装成一个 MCP 服务`

**正文**：

```markdown
## 背景

豆包的网页版生图（Seedream 4.0/4.5/5.0）能力不错，但没有公开 API。
想要"在编辑器里直接让 AI 画图"，只能自己把网页版接口接出来。

这个项目把豆包网页版封装成了 MCP 服务，客户端（Claude Desktop / Cursor / 任何支持 MCP 的）
直接调用，不用 API Key，用自己浏览器的登录态。

## 协议层面几个关键发现

### 1. 两条通道

| 通道 | 端点 | 用途 |
| --- | --- | --- |
| 新（ability） | `/chat/completion` | 文生图，能精确选模型/比例/风格 |
| 旧（samantha） | `/samantha/chat/completion` | 图生图，以及新端点失败时的兜底 |

新通道把模型选择放在 `chat_ability.ability_param` 里，和旧通道的 skill content 是两套协议。

### 2. 无水印直链藏在 SSE 里

服务端会在 SSE 帧里下发 `image_raw` / `dwm3` / `lwm3` 三种模板，只有前两种是无水印的。
签名覆盖整个 URL path，手工替换后缀会直接 400——只能用服务端给的那一条。

### 3. 一次请求可能出多张

`STREAM_CHUNK` 的 patch_op 和后续的 `FULL_MSG_NOTIFY` 会各带一份图。
按图片 key 归并、保留信息最全的那份，再从原文兜底补齐，才能保证"要几张给几张"。

### 4. conversation_id 只在 SSE_ACK 里

```json
"ack_client_meta": {
  "conversation_id": "38452605462188546",
  "conversation_type": 3,
  "section_id": "38452605462188802"
}
```

继续对话时把它塞回 `client_meta.conversation_id`，同时带 `last_section_id` 和
`last_message_index`，`need_create_conversation` 设为 false。网页版就是这么做的。

### 5. 风控与签名是反的

实测加签名（X-Bogus / a_bogus）反而更容易触发 `710022004`，不加签名更稳；
`msToken` 也别塞假值，会触发 `710022002`。

## 工程实现

- Go + Wails：Windows 端有 GUI（扫码登录用 WebView2 取 HttpOnly 的 sessionid）
- 账号池：每个账号并发 1 个任务，全忙排队；MCP 与 GUI 共用同一套调度
- 传输：stdio 与 streamable HTTP 两种

## 最后

项目：https://github.com/haikuotianko/doubao-image-mcp
非官方实现，仅供学习自用，请遵守平台协议。

配图：cover.png（头图）+ multi.png（多图演示）+ features.png
```

---

## 3. 知乎 · 文章

**标题**：`如何让 AI 客户端直接调用豆包画图？（不用 API Key）`

**正文**：

```markdown
豆包网页版的生图一直免费好用，但它没有开放 API，只能在网页里用。

我把它接进了 MCP，效果是：在 Claude Desktop 或 Cursor 里说一句"画一只戴红围巾的橘猫"，
图就直接生成好存在本地文件夹里，顺带回一个网页链接，点开就是那张图所在的豆包对话。

几个实现上的取舍：

- **不用 API Key**：走的是你自己账号的网页登录态，扫码一次即可。代价是属于逆向实现，
  服务端改接口就可能失效。
- **一次多张全返回**：豆包一次可能出 1~4 张，全部落盘并返回链接，不是只给第一张。
- **对话复用**：以前每个请求新建一个会话，侧边栏很快就炸了；现在默认在同一个对话里继续，
  另外还有"上下文隔离"三档，想完全独立出图时可以强制忽略上文。
- **风控自救**：遇到人机验证（710022004）会弹一个豆包窗口让你拖滑块，不需要自己开浏览器。

Windows 有图形界面，mac/Linux 是命令行版。

地址：https://github.com/haikuotianko/doubao-image-mcp

（非官方项目，仅供个人学习自用，请遵守豆包用户协议。）

配图：cover.png + gui.png + multi.png
```

---

## 4. 少数派 · 工具推荐

**标题**：`把豆包的生图能力接进你的 AI 客户端：豆包生图 MCP`

**正文**：

```markdown
豆包的生图质量这两年提升很明显，但它的使用场景一直被困在网页里：想画张图，
得先打开浏览器、找到输入框、贴提示词、等出图、再右键保存。

「豆包生图 MCP」解决的是这个来回切换的问题——它把豆包的网页版生图封装成了一个
MCP（Model Context Protocol）服务，你在 Claude Desktop、Cursor 或者任何支持 MCP
的客户端里直接说"画一张…"，图就会出现在你设置的输出目录里。

**不需要 API Key**，用的是你自己账号的网页登录态，扫码一次就行。

用下来几个比较实用的点：

- 一次生成多张时全部返回，不会只给你第一张
- 连续生图默认都在同一个对话里，不会把豆包侧边栏刷满
- 可以挂多个账号，请求自动排队到空闲账号，适合额度有限的情况
- 被要求人机验证时，程序会弹一个窗口让你拖滑块，不用自己开浏览器折腾

支持 Windows（带图形界面）、macOS 和 Linux（命令行版），单文件，不需要额外环境。

项目地址：https://github.com/haikuotianko/doubao-image-mcp
官网与下载：https://haikuotianko.github.io/doubao-image-mcp/

需要说明的是：这是非官方实现，走的是网页版接口，平台调整后可能失效；
免费账号有额度限制。适合个人自用，不建议对外提供服务。

配图：cover.png（头图）+ gui.png（界面）+ multi.png（多图）
```

---

## 5. 即刻 · 短动态

```
用豆包画图，最烦的是来回切网页。

写了个小工具，把豆包网页版的生图接进了 MCP：
在 Claude Desktop / Cursor 里说一句"画一只戴红围巾的橘猫"，图直接落到本地文件夹，
还能点链接跳回豆包看那张图所在的对话。

不用 API Key（用你自己的登录态），一次出几张全给你，
多账号还能自动排队。Windows 有界面，mac/Linux 命令行。

https://github.com/haikuotianko/doubao-image-mcp

配图：cover.png + multi.png（一次调用出的 4 种风格）
```

---

## 发布前检查

- [ ] 配图顺序：封面 → 界面 → 效果图
- [ ] 首图用 `cover.png`（1200×630，各平台都会当封面）
- [ ] 链接用 GitHub 主页，官网放第二行
- [ ] 结尾保留"非官方 / 仅供自用 / 遵守协议"的说明
