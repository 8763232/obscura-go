# obscura-go 设计文档

日期：2026-05-22

## 1. 项目定位

`github.com/8763232/obscura-go` 是一个可复用的 Go 库，提供对 [Obscura](https://github.com/h4ckf0r0day/obscura) 无头浏览器的 CDP (Chrome DevTools Protocol) 客户端封装。配合示例代码展示用法。

## 2. 整体架构

```
github.com/8763232/obscura-go/
├── cdp/                # WebSocket 连接 + JSON-RPC 编解码（零依赖）
│   ├── conn.go         # WebSocket 连接与握手
│   ├── client.go       # 基于 conn 的 JSON-RPC 调用 (Call)
│   └── event.go        # 事件流读取与分发
├── proto/              # CDP 协议类型（9 个 Obscura 支持的域）
│   ├── common.go       # Request / Event 接口
│   ├── target.go       # createTarget, closeTarget, attachToTarget, createBrowserContext 等
│   ├── page.go         # navigate, getFrameTree, setDeviceMetricsOverride, loadEventFired
│   ├── runtime.go      # evaluate, callFunctionOn, getProperties
│   ├── dom.go          # getDocument, querySelector, querySelectorAll, getOuterHTML, resolveNode
│   ├── network.go      # enable, setCookies, getCookies, setExtraHTTPHeaders, setUserAgentOverride
│   ├── fetch.go        # enable, continueRequest, fulfillRequest, failRequest + requestPaused 事件
│   ├── storage.go      # getCookies, setCookies, deleteCookies
│   ├── input.go        # dispatchMouseEvent, dispatchKeyEvent
│   └── lp.go           # getMarkdown（Obscura 私有域）
├── launcher/           # Obscura 二进制下载与进程管理
│   ├── browser.go      # 从 GitHub Releases 下载各平台归档
│   └── launcher.go     # 进程启动（exec obscura serve --port N）
├── obscura.go          # Browser 根类型：连接、创建页面、事件流、Incognito
├── page.go             # Page 类型：Navigate、Evaluate、WaitFor、Emulate、Cookies、Screenshot
├── hijack.go           # HijackRouter：Fetch 域封装，拦截/替换/失败请求，重定向控制
├── element.go          # Element 类型：Click、Input、Text、HTML、Attribute
├── context.go          # Context/Timeout 链式上下文（Browser/Page/Element）
├── error.go            # 错误类型定义
├── go.mod
└── examples/
    ├── basic/          # 基础操作：启动浏览器 → 导航 → 截图 → JS 执行
    ├── hijack/         # 网络拦截：mock API、修改请求头、控制重定向
    └── concurrent/     # 并发多页面：Incognito 隔离 + goroutine 并发
```

### 依赖关系

```
examples/  ──→  obscura-go (Browser/Page/Hijack/Element)
                        │
           ┌────────────┼────────────┐
           ▼            ▼            ▼
         cdp/        proto/      launcher/
    (WebSocket)   (CDP 类型)   (下载+启动)
```

- `cdp/` 零外部依赖，仅使用 Go 标准库
- `proto/` 零外部依赖，纯类型定义
- `launcher/` 零外部依赖，使用 `os/exec` + `net/http`
- 核心包 `obscura-go` 只依赖上面三个内部包
- 整个库无第三方依赖

## 3. API 风格

混合模式——chromedp 的 context 驱动 + rod 的链式操作（非 Must 版本，所有方法返回 error）：

```go
// chromedp 风格：context 驱动
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
page = page.Context(ctx)

// rod 风格：链式操作
title, err := page.Navigate("https://example.com").
    Element("h1").
    Text()
```

## 4. Launcher — 下载与进程管理

### 平台映射

根据 `GOOS` 和 `GOARCH` 映射到 GitHub Releases 中的下载文件：

| 平台 | 下载文件 |
|------|----------|
| darwin_arm64 | obscura-aarch64-macos.tar.gz |
| darwin_amd64 | obscura-x86_64-macos.tar.gz |
| linux_amd64 | obscura-x86_64-linux.tar.gz |
| linux_arm64 | obscura-aarch64-linux.tar.gz |
| windows_amd64 | obscura-x86_64-windows.zip |

### 下载流程

1. 检查 `~/.cache/obscura-go/<version>/obscura` (或 `obscura.exe`) 是否存在
2. 不存在则从 `https://github.com/h4ckf0r0day/obscura/releases/latest/download/<archive>` 下载
3. 解压到 `~/.cache/obscura-go/<version>/`，包含 `obscura` 和 `obscura-worker`
4. 验证：运行 `obscura --version` 检查可执行性

### 缓存目录结构

```
~/.cache/obscura-go/
├── latest/
│   ├── obscura
│   └── obscura-worker
└── v1.2.0/
    ├── obscura
    └── obscura-worker
```

### Launcher 类型

```go
type Launcher struct {
    Version    string       // "latest" 或具体版本号，默认 "latest"
    Port       int          // CDP WebSocket 端口，0 表示随机
    Proxy      string       // 可选 HTTP/SOCKS5 代理
    Stealth    bool         // 是否启用反检测模式
    Workers    int          // worker 进程数，默认 1
    CacheDir   string       // 下载缓存目录
    HTTPClient *http.Client
}
```

### 启动流程

```
Launch(ctx)
  → 确保二进制存在（下载/缓存）
  → exec.Command(bin, "serve", "--port", port, ...)
  → 轮询 ws://127.0.0.1:PORT/devtools/browser 直到可用
  → 返回 WebSocket URL + cleanup 函数
  → cancel() → kill 进程 + 清理临时文件
```

## 5. cdp 层 — WebSocket 与 JSON-RPC

### WebSocket 连接 (`cdp/conn.go`)

精简实现：只支持文本帧 (opcode 0x1)，不支持分片/Ping/Pong/二进制帧。

```go
func Connect(ctx context.Context, wsURL string) (*Conn, error)

type Conn struct { /* 封装 net.Conn + bufio.Reader */ }
func (c *Conn) Send(msg []byte) error
func (c *Conn) Read() ([]byte, error)
func (c *Conn) Close() error
```

### JSON-RPC 客户端 (`cdp/client.go`)

```go
type Client struct { /* conn, reqID, pending map, events chan */ }

func NewClient(conn *Conn) *Client
func (c *Client) Call(ctx context.Context, method string, params, result any) error
func (c *Client) Events() <-chan *Event
func (c *Client) Close() error
```

Call 流程：
1. 构造 `{"id":N, "method":"...", "params":{...}}`
2. 发送后在 pending map 注册 channel
3. 读取 goroutine 根据 `id` 路由：有 id → pending channel，无 id → events channel
4. context 取消返回超时错误

### 事件分发 (`cdp/event.go`)

```go
type Event struct {
    Method    string          // "Page.loadEventFired", "Fetch.requestPaused" 等
    SessionID string          // target session ID
    Params    json.RawMessage // 原始 JSON
}
```

events channel 缓冲大小默认 256。

## 6. proto 层 — CDP 协议类型

Obscura 实现的 9 个 CDP 域，每个域定义请求（Request）、响应（Result）、事件（Event）：

| 域 | 文件 | 关键类型 |
|----|------|---------|
| Target | target.go | createTarget, closeTarget, attachToTarget, createBrowserContext, disposeBrowserContext |
| Page | page.go | navigate, getFrameTree, setDeviceMetricsOverride, loadEventFired |
| Runtime | runtime.go | evaluate, callFunctionOn, getProperties |
| DOM | dom.go | getDocument, querySelector, querySelectorAll, getOuterHTML, resolveNode |
| Network | network.go | enable, setCookies, getCookies, setExtraHTTPHeaders, setUserAgentOverride |
| Fetch | fetch.go | enable, continueRequest, fulfillRequest, failRequest + requestPaused 事件 |
| Storage | storage.go | getCookies, setCookies, deleteCookies |
| Input | input.go | dispatchMouseEvent, dispatchKeyEvent |
| LP | lp.go | getMarkdown (Obscura 私有域) |

### 公共接口

```go
type Request interface {
    Method() string    // 返回 CDP 方法名，如 "Page.navigate"
}

type Event interface {
    EventName() string // 返回事件名，如 "Page.loadEventFired"
}
```

类型命名规范：`<Domain><Method>`（请求）、`<Domain><Method>Result`（响应）、`<Domain><Event>`（事件）。

## 7. 核心类型

### Browser (`obscura.go`)

```go
type Browser struct {
    client           *cdp.Client
    ctx              context.Context
    cancel           context.CancelFunc
    launcher         *launcher.Launcher
    pagesMu          sync.Mutex
    pages            map[string]*Page
    BrowserContextID string    // "" = 默认，非空 = incognito
    timeout          time.Duration
}
```

主要方法：
- `New() *Browser`
- `Connect(ctx, wsURL string) error` — 连接已运行的 Obscura
- `Launch(ctx, opts ...LauncherOption) error` — 自动下载 + 启动 + 连接
- `NewPage(ctx) (*Page, error)` — 创建新页面
- `NewIncognito(ctx) (*Browser, error)` — 创建隔离浏览上下文
- `Pages() ([]*Page, error)` — 获取所有活跃页面
- `Close() error` — 关闭浏览器，若由 Launcher 启动则 kill 进程
- `Context(ctx) / Timeout(d)` — 链式上下文

**Incognito 并发隔离**：每个 `NewIncognito` 创建独立的 BrowserContext，Cookie/Storage 完全隔离。每个 incognito 实例可独立 NewPage、添加 HijackRouter，适合并发场景。

### Page (`page.go`)

```go
type Page struct {
    browser   *Browser
    sessionID string
    targetID  string
    frameID   string
    ctx       context.Context
    cancel    context.CancelFunc
    timeout   time.Duration
}
```

方法分组：
- **导航**：`Navigate(url)`、`WaitUntil(condition)`
- **JS 执行**：`Evaluate(expression, result)`
- **DOM**：`Element(selector)`、`Elements(selector)`、`HTML()`、`Markdown()`
- **网络拦截**：`HijackRequests() *HijackRouter`
- **设备模拟**：`SetUserAgent(ua)`、`SetViewport(w, h)`
- **Cookie**：`GetCookies()`、`SetCookies(cookies)`
- **截图**：`Screenshot() ([]byte, error)`
- **链式上下文**：`Context(ctx)`、`Timeout(d)`

### Element (`element.go`)

```go
type Element struct {
    page     *Page
    nodeID   int
    selector string
}
```

方法：`Click()`、`Input(text)`、`Text()`、`HTML()`、`Attribute(name)`、`WaitFor(selector)`

**限制**：不支持 iframe 和 shadow DOM 操作。此限制在文档中明确说明。

### HijackRouter (`hijack.go`)

```go
type HijackRouter struct {
    browser  *Browser
    client   proto.Client     // Browser 或 Page（决定作用域）
    patterns []*proto.FetchRequestPattern
    handlers []*hijackHandler
    ctx      context.Context
    cancel   context.CancelFunc
    running  bool
}
```

工作流程：
```
router.Add(pattern, resourceType, handler)
  → 转为 FetchRequestPattern + 正则
  → 调用 Fetch.enable 注册拦截模式

router.Run()
  → 监听 Fetch.requestPaused 事件
  → 匹配的请求在 goroutine 中调用 handler(ctx, req, res)
  → handler 决策：
      ├── 不操作 → Continue（继续原请求）
      ├── res.Fulfill(code, headers, body) → 返回自定义响应
      ├── res.Fail(reason)                → 返回失败
      └── req.SetURL/SetMethod/... + req.Continue() → 修改后继续

router.Stop()
  → Fetch.disable + 取消事件监听
```

### HijackRequest / HijackResponse

```go
type HijackRequest struct {
    URL             string
    Method          string
    Headers         map[string]string
    Body            string
    Type            string              // "Document" | "XHR" | "Script" | ...

    // 响应阶段字段（StatusCode != 0 表示已收到响应）
    StatusCode      int                 // 0 = 请求阶段
    ResponseHeaders map[string]string

    RedirectChain   []string            // 重定向链路中的 URL 列表
}
func (r *HijackRequest) SetURL(url string)
func (r *HijackRequest) SetMethod(method string)
func (r *HijackRequest) SetHeader(key, value string)
func (r *HijackRequest) SetBody(body string)
func (r *HijackRequest) Continue()

type HijackResponse struct {
    StatusCode int
    Headers    map[string]string
    Body       string
}
func (r *HijackResponse) Fulfill(code int, headers map[string]string, body string)
func (r *HijackResponse) Fail(reason string)
func (r *HijackResponse) Follow()       // 响应阶段：跟随重定向
func (r *HijackResponse) FollowTo(url string) // 修改重定向目标
```

### 重定向控制

不依赖浏览器自动跟随重定向。遇到 301/302 时：

```
Fetch.requestPaused (请求阶段, StatusCode=0)
  → handler 处理 → Continue → 浏览器发请求 → 收到 302

Fetch.requestPaused (响应阶段, StatusCode=302)
  → handler 看到 StatusCode=302, ResponseHeaders 含 "Location"
  → 三种决策：
      ├── res.Follow()            → 浏览器跟随 Location
      ├── res.FollowTo(newURL)    → 浏览器重定向到新地址
      │   新请求 → 再次触发 Fetch.requestPaused
      ├── res.Fulfill(200, ...)   → 返回自定义内容，停止重定向
      └── res.Fail(reason)        → 显示错误页
```

**作用域**：通过 Page.HijackRequests() 创建的 HijackRouter 使用该 Page 的 CDP session，只拦截该页面的网络请求。通过 Browser.HijackRequests() 创建的则拦截整个浏览器的请求。

**并发安全**：HijackRouter 内部用 sync.Mutex 保护 handlers；每个 requestPaused 在独立 goroutine 处理。

### 错误处理 (`error.go`)

```go
type Error struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    string `json:"data,omitempty"`
}
func (e *Error) Error() string
```

策略：所有公开方法返回 `error`，不 panic。核心库不提供 `Must*` 方法。

## 8. 示例代码

### examples/basic/main.go — 基础操作

展示启动 + 导航 + 元素操作 + JS 执行 + Markdown 转换 + 截图。

### examples/hijack/main.go — 网络拦截

展示 mock API、修改请求头、控制 302 重定向（跟随/阻止/修改目标）、按资源类型过滤。

### examples/concurrent/main.go — 并发多页面

展示多个 Incognito 上下文并发，每个上下文独立页面和拦截，sync.WaitGroup 收集结果。

## 9. 非功能需求

- **零第三方依赖**：仅使用 Go 标准库
- **不支持 iframe/shadow DOM**：文档中明确说明此限制
- **错误不 panic**：核心库所有方法返回 error
- **并发安全**：Browser/Page/HijackRouter 使用 sync.Mutex 保护共享状态
- **CDP 子集**：只实现 Obscura 支持的 9 个域，不引入不必要类型
