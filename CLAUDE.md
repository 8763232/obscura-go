# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

本项目创建一个调用 [Obscura](https://github.com/h4ckf0r0day/obscura) 无头浏览器的 Go 客户端示例，实现通过 CDP (Chrome DevTools Protocol) 操控浏览器、拦截/替换网络请求、发送网络请求等功能。

- `launcher/` — Obscura 可执行文件，按平台存放（darwin_arm64、linux_amd64、windows_amd64）。每个平台目录包含 `obscura`（主进程）和 `obscura-worker`（并行抓取 worker）两个二进制文件。
- `rod/` — [go-rod](https://github.com/go-rod/rod) 源码，作为 CDP 客户端实现的参考。rod 是一个基于 CDP WebSocket 的高级浏览器驱动库。
- `chromedp/` — [chromedp](https://github.com/chromedp/chromedp) 源码，另一个 CDP 客户端参考。
- `obscura/` — Obscura 引擎的 Rust 源码（参考用）。

## 核心架构概念

### CDP 连接模型（参考 rod）

```
Obscura serve (--port 9222)  →  WebSocket (CDP)  →  Go 客户端
```

1. **启动 Obscura**：`./launcher/darwin_arm64/obscura serve --port 9222` 启动 CDP WebSocket 服务端
2. **WebSocket 连接**：Go 客户端通过 WebSocket 连接到 `ws://127.0.0.1:9222/devtools/browser`
3. **CDP 协议通信**：JSON-RPC 格式的请求/响应/事件，通过 WebSocket 传输

### go-rod 的分层架构

- `lib/cdp/` — WebSocket 连接层，处理 WebSocket 握手、消息收发
- `lib/proto/` — CDP 协议类型定义（请求、响应、事件的 Go 结构体），代码自动生成
- `Browser` — 浏览器级别操作：连接、创建页面、事件监听
- `Page` — 页面级别操作：导航、执行 JS、DOM 查询、截图
- `Element` — 元素级别操作：点击、输入、属性读取
- `HijackRouter` — 网络拦截：通过 CDP Fetch 域拦截请求/响应

### 网络拦截模式（关键参考：`rod/hijack.go:44-111`）

```
启用 Fetch.enable（带 URL 匹配模式）
    → 监听 Fetch.requestPaused 事件
    → 对每个匹配的请求，执行自定义 handler
    → handler 可选择：继续请求、返回自定义响应、或失败请求
```

## 当前项目状态

项目处于初始阶段：
- `go.mod` 定义了模块名 `obscura-go`，Go 版本 1.25
- `main.go` 仅为 GoLand 生成的 Hello World 模板
- 无实际依赖，无业务代码

## 开发要点

- Obscura 通过 `launcher/` 目录下的二进制文件直接运行，无需安装 Chrome 或 Node.js
- 客户端需要通过 WebSocket 连接 Obscura 的 CDP 端点，而非启动 Chrome
- 与 rod 的关键区别：rod 内置了自动下载/查找浏览器的逻辑（`lib/launcher`），而本项目直接使用 `launcher/` 目录下的 Obscura 二进制文件
- rod 的 `HijackRouter` 模式是本项目网络拦截功能的核心参考
