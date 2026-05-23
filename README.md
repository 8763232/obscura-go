# obscura-go

Go 客户端库，用于操控 [Obscura](https://github.com/8763232/obscura-ex) 无头浏览器。通过 CDP (Chrome DevTools Protocol) 实现页面导航、JS 执行、DOM 操作、网络请求拦截等功能，并提供内置 MITM 代理实现 HTTPS 流量完整可见。

## 功能

- **CDP 客户端**：WebSocket 连接 + JSON-RPC，Go 标准库实现，零外部依赖
- **MITM 代理**：`--proxy` 模式下拦截全部 HTTP/HTTPS 请求，可见每步 301/302 重定向及 Set-Cookie
- **网络拦截**：`HijackRouter` + `LoadResponse`，Go 端自定义 HTTP 客户端（TLS/代理/Cookie）
- **Stealth 模式**：反指纹检测 + Tracker 拦截
- **并发多页面**：`NewIncognito()` 隔离上下文

## 安装

```bash
go get github.com/8763232/obscura-go
```

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    "time"
    obscura "github.com/8763232/obscura-go"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    browser := obscura.New()
    browser.Serve(ctx)  // 自动查找并启动本地 obscura 二进制
    defer browser.Close()

    page, _ := browser.NewPage(ctx)
    page.Navigate(ctx, "https://example.com")

    var title string
    page.Evaluate(ctx, "document.title", &title)
    fmt.Println(title)
}
```

## 示例

```bash
# 基础操作：导航 → 截图 → Markdown 转换
go run ./examples/basic/

# 网络代理模式：MITM 代理拦截全部 HTTPS 请求
go run ./examples/hijack/

# 并发多页面
go run ./examples/concurrent/
```

## MITM 代理

obscura 配置 `--proxy` 后全部 HTTP/HTTPS 流量经过 Go 代理：

```go
proxy := &obscura.ProxyServer{
    OnRequest: func(req *http.Request) {
        fmt.Printf("[Request] %s %s\n", req.Method, req.URL)
    },
    OnResponse: func(req *http.Request, resp *http.Response) {
        fmt.Printf("[Response] %d %s\n", resp.StatusCode, req.URL)
        for _, sc := range resp.Header["Set-Cookie"] {
            fmt.Printf("  Set-Cookie: %s\n", sc)
        }
    },
}
proxyAddr, _ := proxy.Start()
defer proxy.Stop()

browser.Serve(ctx, obscura.WithProxy(proxyAddr))
```

每个 301/302 重定向、Set-Cookie、子资源请求均在回调中可见。

## 架构

```
examples/ ──→ obscura-go (Browser / Page / HijackRouter / ProxyServer)
                   │
        ┌──────────┼──────────┐
        ▼          ▼          ▼
      cdp/      proto/    launcher/
  (WebSocket) (CDP类型)  (下载+启动)
```

## Obscura 编译

obscura 引擎源码在 https://github.com/8763232/obscura-ex，本项目对其做了以下修改以支持 MITM 代理：

1. `obscura-net/src/client.rs`：`danger_accept_invalid_certs(true)` 硬编码
2. `obscura-browser/src/page.rs`：代理模式下跳过 wreq stealth 客户端
3. `obscura-cdp/src/server.rs`：`Fetch.requestPaused` 响应阶段字段
4. `obscura-cdp/src/dispatch.rs`：`Security.setIgnoreCertificateErrors` 处理

编译：

```bash
cd obscura
. "$HOME/.cargo/env"
cargo build --release --features stealth
cp target/release/obscura ../launcher/darwin_arm64/latest/
```

## License

Apache 2.0
