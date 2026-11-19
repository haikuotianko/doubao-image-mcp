# 隐私政策 / Privacy Policy

最后更新：2026-11-19

本项目（doubao-image-mcp，下称"本程序"）是一个**本地运行的** MCP 服务与桌面程序。它没有后端服务器，也不收集任何用户数据。

## 1. 我们收集什么

**什么都不收集。**

本程序不包含任何统计、埋点、崩溃上报或行为分析。开发者无法看到你生成了什么、用了多少次、装在哪台机器上。

## 2. 你的数据存在哪里

全部保存在**你自己的电脑上**：

| 数据 | 位置 | 说明 |
| --- | --- | --- |
| 豆包登录态（Cookie / sessionid） | `%APPDATA%\doubao-image-mcp\config.json`（Windows）<br>`~/.config/doubao-image-mcp/config.json`（macOS / Linux） | 明文保存在本地，用于调用豆包接口。**这是你机器上最敏感的文件**，不要提交到 git 或分享 |
| 生成的图片 | 你在设置里指定的输出目录 | 由你完全控制 |
| 调试用的 SSE 原始数据 | 仅当你设置 `DOUBAO_MCP_DUMP_SSE` 时才写出 | 关闭即不产生 |

删除程序目录下的 `config.json` 即可清空全部本地数据（图片需自行删除）。

## 3. 本程序会联网访问哪些地址

只有下面三类，全部由你的操作触发：

| 目标 | 为什么 | 传输内容 |
| --- | --- | --- |
| `*.doubao.com`（字节跳动） | 生图、登录、账号校验 | 你的提示词、图片参数、登录 Cookie |
| `api.github.com` / `github.com` | 启动时检查新版本 | 只读取版本号，不上传任何信息 |
| 你在 `ref_images` 里给出的本地文件 | 图生图时读取后上传到豆包 | 仅该图片本身 |

除此之外没有其它外连。本程序不访问任何自建服务器、不复用第三方统计 SDK。

> 注意：`doubao.com` 属于第三方服务，你通过这些请求提交的提示词与图片受**豆包用户协议与隐私政策**约束，不在本政策覆盖范围内。

## 4. 本地 HTTP 模式

如果你启用 HTTP 模式（默认 `127.0.0.1:8770`）：

- 默认只监听本机回环地址，**外部网络无法访问**
- 默认**没有鉴权**；如需在共享机器上使用，请在设置里配置「HTTP 访问令牌」，并考虑开启每分钟请求上限
- 本程序不会把你的 HTTP 端口暴露到公网，也没有内建的内网穿透

## 5. 遥测与第三方共享

- 无遥测
- 不向任何第三方出售、共享、传输你的数据
- 不使用 Cookie 做跨站追踪（本程序根本没有网页端）

> 官网（GitHub Pages）的访问统计由 GoatCounter 提供，只记录页面浏览量，不使用 Cookie、不追踪个人。这与本程序本体无关。

## 6. 儿童隐私

本程序不面向 13 岁以下儿童，也不收集任何年龄信息。

## 7. 你的权利

由于数据完全在本地，你随时可以：

- **查看**：直接打开 `config.json`
- **删除**：删除该文件
- **迁移**：复制该文件到另一台机器

不存在"向开发者申请删除"的环节，因为我们没有你的数据。

## 8. 政策变更

本政策如有修改，会更新本文件顶部的日期，并在 Release Notes 中说明。

## 9. 联系方式

- 问题反馈：<https://github.com/haikuotianko/doubao-image-mcp/issues>
- 邮箱：1135470449@qq.com

---

# Privacy Policy (English)

Last updated: 2026-11-19

doubao-image-mcp is a **local** MCP server and desktop app. There is no backend, and no user data is collected.

**We collect nothing.** No analytics, no telemetry, no crash reporting.

**Where your data lives:** all on your own machine — the Doubao session cookie in `config.json` under your OS config directory, and generated images in the output folder you choose. Delete `config.json` to erase the stored credentials.

**Network access:** only `*.doubao.com` (image generation / login), `api.github.com` (update check), and the local reference images you explicitly pass in `ref_images`. No self-hosted servers, no third-party SDKs.

**HTTP mode:** binds `127.0.0.1` by default and has no authentication unless you configure an access token. It is never exposed to the public internet by this program.

**Third parties:** nothing is sold, shared, or transmitted. Prompts and images you send to doubao.com are governed by ByteDance's own terms, not this policy.

Contact: 1135470449@qq.com · <https://github.com/haikuotianko/doubao-image-mcp/issues>
