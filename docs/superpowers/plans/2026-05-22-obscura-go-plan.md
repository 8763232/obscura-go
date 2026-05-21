# obscura-go 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从零构建 Go 库 `github.com/8763232/obscura-go`，提供 Obscura 无头浏览器的 CDP 客户端，支持自动下载二进制、网络拦截/重定向控制、并发多页面。

**Architecture:** 四层结构——`cdp/`（WebSocket + JSON-RPC）、`proto/`（CDP 类型定义）、`launcher/`（下载 + 进程管理）、核心包（Browser/Page/HijackRouter/Element）。零外部依赖，仅 Go 标准库。

**Tech Stack:** Go 1.25，标准库 only（net/http、encoding/json、os/exec、sync、context）

---

## 文件结构总览

```
github.com/8763232/obscura-go/
├── cdp/
│   ├── conn.go         # WebSocket 连接与帧读写
│   ├── client.go       # JSON-RPC 调用 + 事件分发
│   └── event.go        # Event 类型定义
├── proto/
│   ├── common.go       # Request/Event 接口
│   ├── target.go       # Target 域
│   ├── page.go         # Page 域
│   ├── runtime.go      # Runtime 域
│   ├── dom.go          # DOM 域
│   ├── network.go      # Network 域
│   ├── fetch.go        # Fetch 域
│   ├── storage.go      # Storage 域
│   ├── input.go        # Input 域
│   └── lp.go           # LP 域 (Obscura 私有)
├── launcher/
│   ├── browser.go      # 二进制下载
│   └── launcher.go     # 进程管理
├── obscura.go          # Browser 类型
├── page.go             # Page 类型
├── hijack.go           # HijackRouter
├── element.go          # Element 类型
├── context.go          # 链式上下文
├── error.go            # 错误类型
├── go.mod
└── examples/
    ├── basic/main.go
    ├── hijack/main.go
    └── concurrent/main.go
```

---

### Task 1: 项目初始化

**Files:**
- Modify: `go.mod`
- Create: 所有目录结构

- [ ] **Step 1: 更新 go.mod**

```go
module github.com/8763232/obscura-go

go 1.25
```

- [ ] **Step 2: 创建目录结构**

Run:
```bash
mkdir -p cdp proto launcher examples/basic examples/hijack examples/concurrent
```

- [ ] **Step 3: 验证**

Run: `go build ./...`
Expected: 无错误（空目录，无文件可编译）

- [ ] **Step 4: Commit**

```bash
git add go.mod cdp/ proto/ launcher/ examples/
git commit -m "初始化项目目录结构"
```

---

### Task 2: cdp 层 — WebSocket 连接 (`cdp/conn.go`)

**Files:**
- Create: `cdp/conn.go`

- [ ] **Step 1: 编写 cdp/conn.go**

```go
package cdp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
)

// Conn 是对 WebSocket 连接的封装，只支持文本帧。
type Conn struct {
	conn net.Conn
	r    *bufio.Reader
}

// Connect 通过 HTTP Upgrade 建立 WebSocket 连接。
func Connect(ctx context.Context, wsURL string) (*Conn, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("cdp: 解析 ws URL: %w", err)
	}

	if u.Scheme != "ws" {
		return nil, fmt.Errorf("cdp: 不支持的协议: %s", u.Scheme)
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, fmt.Errorf("cdp: 连接 %s: %w", u.Host, err)
	}

	key := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		conn.Close()
		return nil, fmt.Errorf("cdp: 生成密钥: %w", err)
	}
	secKey := base64.StdEncoding.EncodeToString(key)

	req := fmt.Sprintf("GET %s HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n\r\n",
		u.RequestURI(), u.Host, secKey)

	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("cdp: 发送握手请求: %w", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("cdp: 读取握手响应: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("cdp: 握手失败，状态码: %d", resp.StatusCode)
	}

	return &Conn{conn: conn, r: bufio.NewReader(conn)}, nil
}

// Send 发送文本帧。
func (c *Conn) Send(msg []byte) error {
	// FIN + Text opcode = 0x81
	frame := []byte{0x81}

	length := len(msg)
	switch {
	case length <= 125:
		frame = append(frame, byte(length|0x80)) // MASK bit set
	case length <= 65535:
		frame = append(frame, byte(126|0x80))
		frame = append(frame, byte(length>>8), byte(length))
	default:
		frame = append(frame, byte(127|0x80))
		for i := 7; i >= 0; i-- {
			frame = append(frame, byte(length>>(i*8)))
		}
	}

	mask := make([]byte, 4)
	if _, err := io.ReadFull(rand.Reader, mask); err != nil {
		return fmt.Errorf("cdp: 生成 mask: %w", err)
	}
	frame = append(frame, mask...)

	for i, b := range msg {
		frame = append(frame, b^mask[i%4])
	}

	_, err := c.conn.Write(frame)
	return err
}

// Read 读取下一帧的 payload。只处理文本帧和非分片帧。
func (c *Conn) Read() ([]byte, error) {
	b0, err := c.r.ReadByte()
	if err != nil {
		return nil, err
	}
	fin := b0&0x80 != 0
	opcode := b0 & 0x0F

	if !fin {
		return nil, errors.New("cdp: 不支持分片帧")
	}
	if opcode == 0x08 {
		return nil, errors.New("cdp: 收到关闭帧")
	}
	if opcode == 0x09 {
		// Ping → 回复 Pong
		b1, _ := c.r.ReadByte()
		length := int(b1 & 0x7F)
		io.CopyN(io.Discard, c.r, int64(length))
		return c.Read()
	}
	if opcode != 0x01 {
		return nil, fmt.Errorf("cdp: 不支持的 opcode: %x", opcode)
	}

	b1, err := c.r.ReadByte()
	if err != nil {
		return nil, err
	}

	length := int64(b1 & 0x7F)
	switch {
	case length == 126:
		buf := make([]byte, 2)
		if _, err := io.ReadFull(c.r, buf); err != nil {
			return nil, err
		}
		length = int64(buf[0])<<8 | int64(buf[1])
	case length == 127:
		buf := make([]byte, 8)
		if _, err := io.ReadFull(c.r, buf); err != nil {
			return nil, err
		}
		length = int64(buf[0])<<56 | int64(buf[1])<<48 | int64(buf[2])<<40 | int64(buf[3])<<32 |
			int64(buf[4])<<24 | int64(buf[5])<<16 | int64(buf[6])<<8 | int64(buf[7])
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(c.r, payload); err != nil {
		return nil, err
	}

	return payload, nil
}

// Close 关闭连接。
func (c *Conn) Close() error {
	return c.conn.Close()
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./cdp`
Expected: 编译成功，无错误

- [ ] **Step 3: Commit**

```bash
git add cdp/conn.go
git commit -m "feat: 添加 cdp WebSocket 连接层"
```

---

### Task 3: cdp 层 — JSON-RPC 客户端与事件 (`cdp/client.go`, `cdp/event.go`)

**Files:**
- Create: `cdp/event.go`
- Create: `cdp/client.go`

- [ ] **Step 1: 编写 cdp/event.go**

```go
package cdp

import "encoding/json"

// Event 是 CDP 事件。
type Event struct {
	Method    string          `json:"method"`
	SessionID string          `json:"sessionId"`
	Params    json.RawMessage `json:"params"`
}
```

- [ ] **Step 2: 编写 cdp/client.go**

```go
package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

type rpcRequest struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     int64            `json:"id,omitempty"`
	Result json.RawMessage  `json:"result,omitempty"`
	Error  *json.RawMessage `json:"error,omitempty"`
	Method string           `json:"method,omitempty"`
	Params json.RawMessage  `json:"params,omitempty"`
}

// Client 是 JSON-RPC 客户端。
type Client struct {
	conn    *Conn
	reqID   int64
	mu      sync.Mutex
	pending map[int64]chan *rpcResponse
	events  chan *Event
	done    chan struct{}

	once sync.Once
}

// NewClient 创建客户端，自动启动读取 goroutine。
func NewClient(conn *Conn) *Client {
	c := &Client{
		conn:    conn,
		pending: make(map[int64]chan *rpcResponse),
		events:  make(chan *Event, 256),
		done:    make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *Client) readLoop() {
	defer close(c.done)
	defer close(c.events)

	for {
		msg, err := c.conn.Read()
		if err != nil {
			return
		}

		var resp rpcResponse
		if err := json.Unmarshal(msg, &resp); err != nil {
			continue
		}

		if resp.Method != "" && resp.ID == 0 {
			// 事件：无 id，有 method
			c.events <- &Event{
				Method: resp.Method,
				Params: resp.Params,
			}
		} else {
			// 响应：有 id
			c.mu.Lock()
			ch, ok := c.pending[resp.ID]
			if ok {
				delete(c.pending, resp.ID)
			}
			c.mu.Unlock()
			if ok {
				ch <- &resp
			}
		}
	}
}

// Call 发起 JSON-RPC 调用并等待响应。
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	id := atomic.AddInt64(&c.reqID, 1)

	req := rpcRequest{ID: id, Method: method}
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("cdp: 编码 params: %w", err)
		}
		req.Params = data
	}

	msg, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("cdp: 编码请求: %w", err)
	}

	ch := make(chan *rpcResponse, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.conn.Send(msg); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("cdp: 发送请求: %w", err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	case resp := <-ch:
		if resp == nil {
			return fmt.Errorf("cdp: 连接已关闭")
		}
		if resp.Error != nil {
			return fmt.Errorf("cdp: %s", *resp.Error)
		}
		if resp.Result != nil && result != nil {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("cdp: 解码结果: %w", err)
			}
		}
		return nil
	}
}

// Events 返回事件通道。
func (c *Client) Events() <-chan *Event {
	return c.events
}

// Close 关闭客户端。
func (c *Client) Close() error {
	err := c.conn.Close()
	c.once.Do(func() {})
	return err
}
```

- [ ] **Step 3: 验证编译**

Run: `go build ./cdp`
Expected: 编译成功

- [ ] **Step 4: Commit**

```bash
git add cdp/event.go cdp/client.go
git commit -m "feat: 添加 cdp JSON-RPC 客户端与事件系统"
```

---

### Task 4: cdp 层 — 事件 SessionID 过滤

**Files:**
- Modify: `cdp/client.go`

> 注：CDP 事件中包含 `sessionId` 字段（attachToTarget 返回），需要在事件中正确填充。当前 `readLoop` 中未处理 `resp.SessionID`，需要补充。

- [ ] **Step 1: 更新 readLoop 中的事件解析**

修改 `cdp/client.go` 中事件分支：

```go
// 将
c.events <- &Event{
    Method: resp.Method,
    Params: resp.Params,
}

// 替换为
// rpcResponse 中增加 SessionID 字段
type rpcResponse struct {
    ID        int64            `json:"id,omitempty"`
    Result    json.RawMessage  `json:"result,omitempty"`
    Error     *json.RawMessage `json:"error,omitempty"`
    Method    string           `json:"method,omitempty"`
    Params    json.RawMessage  `json:"params,omitempty"`
    SessionID string           `json:"sessionId,omitempty"`
}

// 事件分支改为
c.events <- &Event{
    Method:    resp.Method,
    SessionID: resp.SessionID,
    Params:    resp.Params,
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./cdp`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add cdp/client.go
git commit -m "fix: cdp 事件中传递 SessionID"
```

---

### Task 5: proto 层 — 公共接口 (`proto/common.go`)

**Files:**
- Create: `proto/common.go`

- [ ] **Step 1: 编写 proto/common.go**

```go
package proto

// Request 是所有 CDP 请求类型必须实现的接口。
type Request interface {
	Method() string
}

// Event 是所有 CDP 事件类型必须实现的接口。
type Event interface {
	EventName() string
}

// Node 是 DOM 节点类型。
type Node struct {
	NodeID    int     `json:"nodeId"`
	NodeType  int     `json:"nodeType"`
	NodeName  string  `json:"nodeName"`
	NodeValue string  `json:"nodeValue"`
	Children  []*Node `json:"children,omitempty"`
}

// RemoteObject 是 Runtime 远程对象。
type RemoteObject struct {
	Type        string         `json:"type"`
	Subtype     string         `json:"subtype,omitempty"`
	ClassName   string         `json:"className,omitempty"`
	Value       any            `json:"value,omitempty"`
	ObjectID    string         `json:"objectId,omitempty"`
	Description string         `json:"description,omitempty"`
	Preview     *ObjectPreview `json:"preview,omitempty"`
}

// ObjectPreview 是远程对象的预览。
type ObjectPreview struct {
	Type        string              `json:"type"`
	Subtype     string              `json:"subtype"`
	Description string              `json:"description"`
	Properties  []*PropertyPreview  `json:"properties"`
}

// PropertyPreview 是属性预览。
type PropertyPreview struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./proto`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add proto/common.go
git commit -m "feat: 添加 proto 公共类型与接口"
```

---

### Task 6: proto 层 — Target 域 (`proto/target.go`)

**Files:**
- Create: `proto/target.go`

- [ ] **Step 1: 编写 proto/target.go**

```go
package proto

// Target.createTarget
type TargetCreateTarget struct {
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

func (r TargetCreateTarget) Method() string { return "Target.createTarget" }

type TargetCreateTargetResult struct {
	TargetID string `json:"targetId"`
}

// Target.closeTarget
type TargetCloseTarget struct {
	TargetID string `json:"targetId"`
}

func (r TargetCloseTarget) Method() string { return "Target.closeTarget" }

// Target.attachToTarget
type TargetAttachToTarget struct {
	TargetID string `json:"targetId"`
	Flatten  bool   `json:"flatten"`
}

func (r TargetAttachToTarget) Method() string { return "Target.attachToTarget" }

type TargetAttachToTargetResult struct {
	SessionID string `json:"sessionId"`
}

// Target.createBrowserContext
type TargetCreateBrowserContext struct{}

func (r TargetCreateBrowserContext) Method() string { return "Target.createBrowserContext" }

type TargetCreateBrowserContextResult struct {
	BrowserContextID string `json:"browserContextId"`
}

// Target.disposeBrowserContext
type TargetDisposeBrowserContext struct {
	BrowserContextID string `json:"browserContextId"`
}

func (r TargetDisposeBrowserContext) Method() string { return "Target.disposeBrowserContext" }

// Target.setDiscoverTargets
type TargetSetDiscoverTargets struct {
	Discover bool `json:"discover"`
}

func (r TargetSetDiscoverTargets) Method() string { return "Target.setDiscoverTargets" }

// Target.getTargets
type TargetGetTargets struct{}

func (r TargetGetTargets) Method() string { return "Target.getTargets" }

type TargetGetTargetsResult struct {
	TargetInfos []TargetInfo `json:"targetInfos"`
}

// Target.getTargetInfo
type TargetGetTargetInfo struct {
	TargetID string `json:"targetId"`
}

func (r TargetGetTargetInfo) Method() string { return "Target.getTargetInfo" }

type TargetGetTargetInfoResult struct {
	TargetInfo TargetInfo `json:"targetInfo"`
}

// TargetInfo
type TargetInfo struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./proto`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add proto/target.go
git commit -m "feat: 添加 proto Target 域类型"
```

---

### Task 7: proto 层 — Page 域 (`proto/page.go`)

**Files:**
- Create: `proto/page.go`

- [ ] **Step 1: 编写 proto/page.go**

```go
package proto

// Page.navigate
type PageNavigate struct {
	URL string `json:"url"`
}

func (r PageNavigate) Method() string { return "Page.navigate" }

type PageNavigateResult struct {
	FrameID   string `json:"frameId"`
	LoaderID  string `json:"loaderId"`
	ErrorText string `json:"errorText,omitempty"`
}

// Page.getFrameTree
type PageGetFrameTree struct{}

func (r PageGetFrameTree) Method() string { return "Page.getFrameTree" }

type PageGetFrameTreeResult struct {
	FrameTree *PageFrameTree `json:"frameTree"`
}

type PageFrameTree struct {
	Frame       *PageFrame       `json:"frame"`
	ChildFrames []*PageFrameTree `json:"childFrames,omitempty"`
}

type PageFrame struct {
	ID     string `json:"id"`
	Loader string `json:"loaderId"`
	URL    string `json:"url"`
}

// Page.enable
type PageEnable struct{}

func (r PageEnable) Method() string { return "Page.enable" }

// Page.addScriptToEvaluateOnNewDocument
type PageAddScriptToEvaluateOnNewDocument struct {
	Source string `json:"source"`
}

func (r PageAddScriptToEvaluateOnNewDocument) Method() string {
	return "Page.addScriptToEvaluateOnNewDocument"
}

type PageAddScriptToEvaluateOnNewDocumentResult struct {
	Identifier string `json:"identifier"`
}

// Page.setDeviceMetricsOverride
type PageSetDeviceMetricsOverride struct {
	Width             int   `json:"width"`
	Height            int   `json:"height"`
	DeviceScaleFactor float64 `json:"deviceScaleFactor"`
	Mobile            bool  `json:"mobile"`
}

func (r PageSetDeviceMetricsOverride) Method() string {
	return "Page.setDeviceMetricsOverride"
}

// Page.captureScreenshot
type PageCaptureScreenshot struct {
	Format string `json:"format,omitempty"`
}

func (r PageCaptureScreenshot) Method() string { return "Page.captureScreenshot" }

type PageCaptureScreenshotResult struct {
	Data string `json:"data"`
}

// 事件
type PageLoadEventFired struct {
	Timestamp float64 `json:"timestamp"`
}

func (e PageLoadEventFired) EventName() string { return "Page.loadEventFired" }

type PageDOMContentEventFired struct {
	Timestamp float64 `json:"timestamp"`
}

func (e PageDOMContentEventFired) EventName() string { return "Page.domContentEventFired" }

type PageFrameNavigated struct {
	Frame *PageFrame `json:"frame"`
}

func (e PageFrameNavigated) EventName() string { return "Page.frameNavigated" }
```

- [ ] **Step 2: 验证编译**

Run: `go build ./proto`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add proto/page.go
git commit -m "feat: 添加 proto Page 域类型"
```

---

### Task 8: proto 层 — Runtime 域 (`proto/runtime.go`)

**Files:**
- Create: `proto/runtime.go`

- [ ] **Step 1: 编写 proto/runtime.go**

```go
package proto

// Runtime.evaluate
type RuntimeEvaluate struct {
	Expression    string `json:"expression"`
	ObjectGroup   string `json:"objectGroup,omitempty"`
	ReturnByValue bool   `json:"returnByValue,omitempty"`
}

func (r RuntimeEvaluate) Method() string { return "Runtime.evaluate" }

type RuntimeEvaluateResult struct {
	Result           *RemoteObject `json:"result"`
	ExceptionDetails any           `json:"exceptionDetails,omitempty"`
}

// Runtime.callFunctionOn
type RuntimeCallFunctionOn struct {
	FunctionDeclaration string          `json:"functionDeclaration"`
	ObjectID            string          `json:"objectId,omitempty"`
	Arguments           []*CallArgument `json:"arguments,omitempty"`
	ReturnByValue       bool            `json:"returnByValue"`
}

func (r RuntimeCallFunctionOn) Method() string { return "Runtime.callFunctionOn" }

type RuntimeCallFunctionOnResult struct {
	Result *RemoteObject `json:"result"`
}

type CallArgument struct {
	Value  any    `json:"value,omitempty"`
	Handle string `json:"handle,omitempty"`
}

// Runtime.getProperties
type RuntimeGetProperties struct {
	ObjectID    string `json:"objectId"`
	OwnOnly     bool   `json:"ownProperties,omitempty"`
	Accessors   bool   `json:"accessorPropertiesOnly,omitempty"`
}

func (r RuntimeGetProperties) Method() string { return "Runtime.getProperties" }

type RuntimeGetPropertiesResult struct {
	Result []*PropertyDescriptor `json:"result"`
}

type PropertyDescriptor struct {
	Name         string        `json:"name"`
	Value        *RemoteObject `json:"value,omitempty"`
	Writable     bool          `json:"writable"`
	Configurable bool          `json:"configurable"`
	Enumerable   bool          `json:"enumerable"`
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./proto`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add proto/runtime.go
git commit -m "feat: 添加 proto Runtime 域类型"
```

---

### Task 9: proto 层 — DOM 域 (`proto/dom.go`)

**Files:**
- Create: `proto/dom.go`

- [ ] **Step 1: 编写 proto/dom.go**

```go
package proto

// DOM.getDocument
type DOMGetDocument struct {
	Depth int `json:"depth,omitempty"`
}

func (r DOMGetDocument) Method() string { return "DOM.getDocument" }

type DOMGetDocumentResult struct {
	Root *Node `json:"root"`
}

// DOM.querySelector
type DOMQuerySelector struct {
	NodeID   int    `json:"nodeId"`
	Selector string `json:"selector"`
}

func (r DOMQuerySelector) Method() string { return "DOM.querySelector" }

type DOMQuerySelectorResult struct {
	NodeID int `json:"nodeId"`
}

// DOM.querySelectorAll
type DOMQuerySelectorAll struct {
	NodeID   int    `json:"nodeId"`
	Selector string `json:"selector"`
}

func (r DOMQuerySelectorAll) Method() string { return "DOM.querySelectorAll" }

type DOMQuerySelectorAllResult struct {
	NodeIDs []int `json:"nodeIds"`
}

// DOM.getOuterHTML
type DOMGetOuterHTML struct {
	NodeID int `json:"nodeId"`
}

func (r DOMGetOuterHTML) Method() string { return "DOM.getOuterHTML" }

type DOMGetOuterHTMLResult struct {
	OuterHTML string `json:"outerHTML"`
}

// DOM.resolveNode
type DOMResolveNode struct {
	NodeID int `json:"nodeId"`
}

func (r DOMResolveNode) Method() string { return "DOM.resolveNode" }

type DOMResolveNodeResult struct {
	Object *RemoteObject `json:"object"`
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./proto`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add proto/dom.go
git commit -m "feat: 添加 proto DOM 域类型"
```

---

### Task 10: proto 层 — Network 域 (`proto/network.go`)

**Files:**
- Create: `proto/network.go`

- [ ] **Step 1: 编写 proto/network.go**

```go
package proto

// Network.enable
type NetworkEnable struct{}

func (r NetworkEnable) Method() string { return "Network.enable" }

// Network.setCookies
type NetworkSetCookies struct {
	Cookies []*CookieParam `json:"cookies"`
}

func (r NetworkSetCookies) Method() string { return "Network.setCookies" }

type CookieParam struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
	Expires  float64 `json:"expires,omitempty"`
	URL      string `json:"url,omitempty"`
}

type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"httpOnly"`
	SameSite string `json:"sameSite,omitempty"`
	Expires  float64 `json:"expires"`
}

// Network.getCookies
type NetworkGetCookies struct {
	Urls []string `json:"urls,omitempty"`
}

func (r NetworkGetCookies) Method() string { return "Network.getCookies" }

type NetworkGetCookiesResult struct {
	Cookies []*Cookie `json:"cookies"`
}

// Network.setExtraHTTPHeaders
type NetworkSetExtraHTTPHeaders struct {
	Headers map[string]string `json:"headers"`
}

func (r NetworkSetExtraHTTPHeaders) Method() string { return "Network.setExtraHTTPHeaders" }

// Network.setUserAgentOverride
type NetworkSetUserAgentOverride struct {
	UserAgent string `json:"userAgent"`
}

func (r NetworkSetUserAgentOverride) Method() string {
	return "Network.setUserAgentOverride"
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./proto`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add proto/network.go
git commit -m "feat: 添加 proto Network 域类型"
```

---

### Task 11: proto 层 — Fetch 域 (`proto/fetch.go`)

**Files:**
- Create: `proto/fetch.go`

- [ ] **Step 1: 编写 proto/fetch.go**

```go
package proto

// Fetch.enable
type FetchEnable struct {
	Patterns           []*FetchRequestPattern `json:"patterns,omitempty"`
	HandleAuthRequests bool                   `json:"handleAuthRequests,omitempty"`
}

func (r FetchEnable) Method() string { return "Fetch.enable" }

type FetchRequestPattern struct {
	URLPattern   string `json:"urlPattern,omitempty"`
	ResourceType string `json:"resourceType,omitempty"`
	RequestStage string `json:"requestStage,omitempty"` // "Request" | "Response"
}

// Fetch.disable
type FetchDisable struct{}

func (r FetchDisable) Method() string { return "Fetch.disable" }

// Fetch.continueRequest
type FetchContinueRequest struct {
	RequestID string            `json:"requestId"`
	URL       string            `json:"url,omitempty"`
	Method    string            `json:"method,omitempty"`
	Headers   []FetchHeaderEntry `json:"headers,omitempty"`
	PostData  string            `json:"postData,omitempty"`
}

func (r FetchContinueRequest) Method() string { return "Fetch.continueRequest" }

type FetchHeaderEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Fetch.fulfillRequest
type FetchFulfillRequest struct {
	RequestID       string            `json:"requestId"`
	ResponseCode    int               `json:"responseCode"`
	ResponseHeaders []FetchHeaderEntry `json:"responseHeaders,omitempty"`
	Body            string            `json:"body,omitempty"`
}

func (r FetchFulfillRequest) Method() string { return "Fetch.fulfillRequest" }

// Fetch.failRequest
type FetchFailRequest struct {
	RequestID   string `json:"requestId"`
	ErrorReason string `json:"errorReason"`
}

func (r FetchFailRequest) Method() string { return "Fetch.failRequest" }

// Fetch.requestPaused 事件
type FetchRequestPaused struct {
	RequestID          string            `json:"requestId"`
	Request            *FetchRequest     `json:"request"`
	ResourceType       string            `json:"resourceType"`
	ResponseStatusCode int               `json:"responseStatusCode,omitempty"`
	ResponseHeaders    []FetchHeaderEntry `json:"responseHeaders,omitempty"`
}

func (e FetchRequestPaused) EventName() string { return "Fetch.requestPaused" }

type FetchRequest struct {
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	Headers  map[string]string `json:"headers"`
	PostData string            `json:"postData,omitempty"`
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./proto`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add proto/fetch.go
git commit -m "feat: 添加 proto Fetch 域类型"
```

---

### Task 12: proto 层 — Storage 域 (`proto/storage.go`)

**Files:**
- Create: `proto/storage.go`

- [ ] **Step 1: 编写 proto/storage.go**

```go
package proto

// Storage.getCookies
type StorageGetCookies struct {
	BrowserContextID string `json:"browserContextId,omitempty"`
}

func (r StorageGetCookies) Method() string { return "Storage.getCookies" }

type StorageGetCookiesResult struct {
	Cookies []*Cookie `json:"cookies"`
}

// Storage.setCookies
type StorageSetCookies struct {
	Cookies          []*CookieParam `json:"cookies"`
	BrowserContextID string         `json:"browserContextId,omitempty"`
}

func (r StorageSetCookies) Method() string { return "Storage.setCookies" }

// Storage.clearCookies
type StorageClearCookies struct {
	BrowserContextID string `json:"browserContextId,omitempty"`
}

func (r StorageClearCookies) Method() string { return "Storage.clearCookies" }

// Storage.deleteCookies
type StorageDeleteCookies struct {
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	Domain  string `json:"domain,omitempty"`
	Path    string `json:"path,omitempty"`
}

func (r StorageDeleteCookies) Method() string { return "Storage.deleteCookies" }
```

- [ ] **Step 2: 验证编译**

Run: `go build ./proto`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add proto/storage.go
git commit -m "feat: 添加 proto Storage 域类型"
```

---

### Task 13: proto 层 — Input 域 (`proto/input.go`)

**Files:**
- Create: `proto/input.go`

- [ ] **Step 1: 编写 proto/input.go**

```go
package proto

// Input.dispatchMouseEvent
type InputDispatchMouseEvent struct {
	Type       string  `json:"type"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Button     string  `json:"button,omitempty"`
	ClickCount int     `json:"clickCount,omitempty"`
}

func (r InputDispatchMouseEvent) Method() string { return "Input.dispatchMouseEvent" }

// Input.dispatchKeyEvent
type InputDispatchKeyEvent struct {
	Type string `json:"type"`
	Key  string `json:"key,omitempty"`
	Text string `json:"text,omitempty"`
	Code string `json:"code,omitempty"`
}

func (r InputDispatchKeyEvent) Method() string { return "Input.dispatchKeyEvent" }
```

- [ ] **Step 2: 验证编译**

Run: `go build ./proto`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add proto/input.go
git commit -m "feat: 添加 proto Input 域类型"
```

---

### Task 14: proto 层 — LP 域 (`proto/lp.go`)

**Files:**
- Create: `proto/lp.go`

- [ ] **Step 1: 编写 proto/lp.go**

```go
package proto

// LP.getMarkdown（Obscura 私有域）
type LPGetMarkdown struct{}

func (r LPGetMarkdown) Method() string { return "LP.getMarkdown" }

type LPGetMarkdownResult struct {
	Markdown string `json:"markdown"`
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./proto`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add proto/lp.go
git commit -m "feat: 添加 proto LP 域类型（Obscura 私有）"
```

---

### Task 15: launcher — 二进制下载 (`launcher/browser.go`)

**Files:**
- Create: `launcher/browser.go`

- [ ] **Step 1: 编写 launcher/browser.go**

```go
package launcher

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// 平台 → 下载文件名映射
var platformArchive = map[string]string{
	"darwin_arm64":  "obscura-aarch64-macos.tar.gz",
	"darwin_amd64":  "obscura-x86_64-macos.tar.gz",
	"linux_amd64":   "obscura-x86_64-linux.tar.gz",
	"linux_arm64":   "obscura-aarch64-linux.tar.gz",
	"windows_amd64": "obscura-x86_64-windows.zip",
}

// binName 返回当前平台的 obscura 二进制名。
func binName() string {
	if runtime.GOOS == "windows" {
		return "obscura.exe"
	}
	return "obscura"
}

// defaultCacheDir 返回默认缓存目录。
func defaultCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "obscura-go")
}

// Browser 管理 obscura 二进制的下载。
type Browser struct {
	Version    string // "latest" 或具体版本号
	CacheDir   string
	HTTPClient *http.Client
}

// NewBrowser 创建默认 Browser 实例。
func NewBrowser() *Browser {
	return &Browser{
		Version:    "latest",
		CacheDir:   defaultCacheDir(),
		HTTPClient: http.DefaultClient,
	}
}

// BinPath 返回当前平台的 obscura 二进制路径。
func (b *Browser) BinPath() string {
	return filepath.Join(b.CacheDir, b.Version, binName())
}

// Get 确保二进制存在，不存在则下载。
func (b *Browser) Get(ctx context.Context) (string, error) {
	binPath := b.BinPath()

	if b.validate(binPath) == nil {
		return binPath, nil
	}

	// 清理可能损坏的缓存
	_ = os.RemoveAll(filepath.Join(b.CacheDir, b.Version))

	archive, ok := platformArchive[runtime.GOOS+"_"+runtime.GOARCH]
	if !ok {
		return "", fmt.Errorf("launcher: 不支持的平台: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	url := fmt.Sprintf(
		"https://github.com/h4ckf0r0day/obscura/releases/%s/download/%s",
		b.Version, archive,
	)
	if b.Version == "latest" {
		url = fmt.Sprintf(
			"https://github.com/h4ckf0r0day/obscura/releases/latest/download/%s",
			archive,
		)
	}

	if err := b.download(ctx, url, archive); err != nil {
		return "", fmt.Errorf("launcher: 下载失败: %w", err)
	}

	if err := b.validate(binPath); err != nil {
		return "", fmt.Errorf("launcher: 验证失败: %w", err)
	}

	return binPath, nil
}

func (b *Browser) validate(binPath string) error {
	_, err := os.Stat(binPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(binPath, "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("无法执行 obscura: %w", err)
	}
	return nil
}

func (b *Browser) download(ctx context.Context, url, archive string) error {
	dir := filepath.Join(b.CacheDir, b.Version)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := b.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	tmpFile, err := os.CreateTemp("", "obscura-*"+filepath.Ext(archive))
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	switch {
	case filepath.Ext(archive) == ".gz" || filepath.Ext(archive) == ".tgz":
		return extractTarGz(tmpFile.Name(), dir)
	case filepath.Ext(archive) == ".zip":
		return extractZip(tmpFile.Name(), dir)
	default:
		return fmt.Errorf("不支持的文件格式: %s", archive)
	}
}

func extractTarGz(path, dest string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, filepath.Base(hdr.Name))
		if hdr.Typeflag == tar.TypeDir {
			os.MkdirAll(target, 0755)
			continue
		}

		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()

		if err := os.Chmod(target, 0755); err != nil {
			return err
		}
	}
	return nil
}

func extractZip(path, dest string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(dest, filepath.Base(f.Name))
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./launcher`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add launcher/browser.go
git commit -m "feat: 添加 launcher 二进制下载功能"
```

---

### Task 16: launcher — 进程管理 (`launcher/launcher.go`)

**Files:**
- Create: `launcher/launcher.go`

- [ ] **Step 1: 编写 launcher/launcher.go**

```go
package launcher

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// Launcher 管理 obscura 进程的启动。
type Launcher struct {
	browser *Browser
	Port    int    // 0 = 随机端口
	Proxy   string // HTTP/SOCKS5 代理
	Stealth bool
	Workers int

	cmd *exec.Cmd
	mu  sync.Mutex
}

// New 创建默认 Launcher。
func New() *Launcher {
	return &Launcher{
		browser: NewBrowser(),
		Port:    0,
		Workers: 1,
	}
}

// Launch 下载（如需要）并启动 obscura serve 进程。
// 返回 CDP WebSocket URL 和 cleanup 函数。
func (l *Launcher) Launch(ctx context.Context) (wsURL string, cleanup func(), err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	binPath, err := l.browser.Get(ctx)
	if err != nil {
		return "", nil, err
	}

	port := l.Port
	if port == 0 {
		p, err := randomPort()
		if err != nil {
			return "", nil, fmt.Errorf("launcher: 无可用端口: %w", err)
		}
		port = p
	}

	args := []string{"serve", "--port", strconv.Itoa(port)}
	if l.Proxy != "" {
		args = append(args, "--proxy", l.Proxy)
	}
	if l.Stealth {
		args = append(args, "--stealth")
	}
	if l.Workers > 1 {
		args = append(args, "--workers", strconv.Itoa(l.Workers))
	}

	l.cmd = exec.CommandContext(ctx, binPath, args...)
	if err := l.cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("launcher: 启动 obscura 失败: %w", err)
	}

	wsURL = fmt.Sprintf("ws://127.0.0.1:%d/devtools/browser", port)

	// 等待 WebSocket 就绪
	if err := l.waitReady(ctx, wsURL); err != nil {
		l.cmd.Process.Kill()
		return "", nil, fmt.Errorf("launcher: obscura 未就绪: %w", err)
	}

	cleanup = func() {
		if l.cmd != nil && l.cmd.Process != nil {
			l.cmd.Process.Kill()
		}
	}

	return wsURL, cleanup, nil
}

func (l *Launcher) waitReady(ctx context.Context, wsURL string) error {
	host := "127.0.0.1"
	port := ""
	for i := len(wsURL) - 1; i >= 0; i-- {
		if wsURL[i] == ':' {
			port = wsURL[i+1:]
			break
		}
	}
	// 提取端口（去掉路径）
	for i, c := range port {
		if c == '/' {
			port = port[:i]
			break
		}
	}

	addr := net.JoinHostPort(host, port)

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("连接超时")
		}

		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func randomPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./launcher`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add launcher/launcher.go
git commit -m "feat: 添加 launcher 进程管理功能"
```

---

### Task 17: 错误类型 (`error.go`)

**Files:**
- Create: `error.go`

- [ ] **Step 1: 编写 error.go**

```go
package obscura

import "fmt"

// Error 是 CDP 协议错误。
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("cdp error %d: %s", e.Code, e.Message)
}

// 常用错误
var (
	ErrBrowserClosed   = fmt.Errorf("obscura: 浏览器已关闭")
	ErrPageClosed      = fmt.Errorf("obscura: 页面已关闭")
	ErrInvalidSelector = fmt.Errorf("obscura: 无效的 CSS 选择器")
	ErrTimeout         = fmt.Errorf("obscura: 操作超时")
)
```

- [ ] **Step 2: 验证编译**

Run: `go build .`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add error.go
git commit -m "feat: 添加错误类型定义"
```

---

### Task 18: 链式上下文 (`context.go`)

**Files:**
- Create: `context.go`

- [ ] **Step 1: 编写 context.go**

```go
package obscura

import (
	"context"
	"time"
)

// Context 返回指定 context 的克隆。
func (b *Browser) Context(ctx context.Context) *Browser {
	b2 := *b
	b2.ctx = ctx
	return &b2
}

// GetContext 返回当前 context。
func (b *Browser) GetContext() context.Context {
	return b.ctx
}

// Timeout 返回带超时的克隆。
func (b *Browser) Timeout(d time.Duration) *Browser {
	ctx, _ := context.WithTimeout(b.ctx, d)
	return b.Context(ctx)
}

// WithCancel 返回带 cancel 的克隆。
func (b *Browser) WithCancel() (*Browser, context.CancelFunc) {
	ctx, cancel := context.WithCancel(b.ctx)
	return b.Context(ctx), cancel
}

// Context 返回指定 context 的克隆。
func (p *Page) Context(ctx context.Context) *Page {
	p2 := *p
	p2.ctx = ctx
	return &p2
}

// GetContext 返回当前 context。
func (p *Page) GetContext() context.Context {
	return p.ctx
}

// Timeout 返回带超时的克隆。
func (p *Page) Timeout(d time.Duration) *Page {
	ctx, _ := context.WithTimeout(p.ctx, d)
	return p.Context(ctx)
}

// WithCancel 返回带 cancel 的克隆。
func (p *Page) WithCancel() (*Page, context.CancelFunc) {
	ctx, cancel := context.WithCancel(p.ctx)
	return p.Context(ctx), cancel
}

// Context 返回指定 context 的克隆。
func (el *Element) Context(ctx context.Context) *Element {
	el2 := *el
	el2.ctx = ctx
	return &el2
}

// GetContext 返回当前 context。
func (el *Element) GetContext() context.Context {
	return el.ctx
}
```

- [ ] **Step 2: 验证编译**

Run: `go build .`
Expected: 编译成功（Browser、Page、Element 类型尚未定义，会报错。等 Task 19-21 完成后再验证。）

- [ ] **Step 3: Commit**

```bash
git add context.go
git commit -m "feat: 添加链式上下文方法（Browser/Page/Element）"
```

---

### Task 19: Browser 类型 (`obscura.go`)

**Files:**
- Create: `obscura.go`

- [ ] **Step 1: 编写 obscura.go**

```go
package obscura

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/8763232/obscura-go/cdp"
	"github.com/8763232/obscura-go/launcher"
	"github.com/8763232/obscura-go/proto"
)

// Browser 是 obscura 浏览器实例的控制句柄。
type Browser struct {
	client           *cdp.Client
	ctx              context.Context
	cancel           context.CancelFunc
	launcher         *launcher.Launcher
	pagesMu          sync.Mutex
	pages            map[string]*Page
	BrowserContextID string
	timeout          time.Duration
	events           <-chan *cdp.Event
}

// New 创建 Browser 实例。
func New() *Browser {
	ctx, cancel := context.WithCancel(context.Background())
	return &Browser{
		ctx:     ctx,
		cancel:  cancel,
		pages:   make(map[string]*Page),
		timeout: 30 * time.Second,
	}
}

// Connect 连接到已运行的 obscura CDP 服务。
func (b *Browser) Connect(ctx context.Context, wsURL string) error {
	conn, err := cdp.Connect(ctx, wsURL)
	if err != nil {
		return err
	}

	b.client = cdp.NewClient(conn)
	b.events = b.client.Events()

	return proto.TargetSetDiscoverTargets{Discover: true}.Method()
	// 使用 b.client.Call 发送
}

// 修正上面的代码：实际调用 CDP
func (b *Browser) Connect(ctx context.Context, wsURL string) error {
	conn, err := cdp.Connect(ctx, wsURL)
	if err != nil {
		return err
	}

	b.client = cdp.NewClient(conn)
	b.events = b.client.Events()

	return b.Call(ctx, "", proto.TargetSetDiscoverTargets{Discover: true})
}

// Launch 使用 launcher 下载并启动 obscura。
func (b *Browser) Launch(ctx context.Context, opts ...func(*launcher.Launcher)) error {
	l := launcher.New()
	for _, o := range opts {
		o(l)
	}
	b.launcher = l

	wsURL, cleanup, err := l.Launch(ctx)
	if err != nil {
		return err
	}
	_ = cleanup // 由 Browser.Close() 调用

	return b.Connect(ctx, wsURL)
}

// NewPage 创建新页面。
func (b *Browser) NewPage(ctx context.Context) (*Page, error) {
	res := proto.TargetCreateTargetResult{}
	if err := b.Call(ctx, "", proto.TargetCreateTarget{URL: "about:blank"}); err != nil {
		return nil, err
	}
	// 修复：使用返回结果
	return b.pageFromTarget(ctx, res.TargetID)
}

// NewIncognito 创建隔离的浏览上下文。
func (b *Browser) NewIncognito(ctx context.Context) (*Browser, error) {
	res := proto.TargetCreateBrowserContextResult{}
	if err := b.Call(ctx, "", proto.TargetCreateBrowserContext{}); err != nil {
		return nil, err
	}

	incog := *b
	incog.BrowserContextID = res.BrowserContextID
	incog.pages = make(map[string]*Page)
	return &incog, nil
}

// Call 发起 CDP 调用。sessionID 为空时使用浏览器级调用。
func (b *Browser) Call(ctx context.Context, sessionID string, req proto.Request) error {
	return b.client.Call(ctx, req.Method(), nil, nil)
}

// CallResult 发起 CDP 调用并将结果解析到 result。
func (b *Browser) CallResult(ctx context.Context, sessionID string, req proto.Request, result any) error {
	return b.client.Call(ctx, req.Method(), req, result)
}

// Events 返回过滤后的 CDP 事件通道，只接收指定 method 的事件。
func (b *Browser) Events(method string, sessionID string) <-chan *cdp.Event {
	filtered := make(chan *cdp.Event, 256)
	go func() {
		defer close(filtered)
		for e := range b.events {
			if e.Method == method && (sessionID == "" || e.SessionID == sessionID) {
				select {
				case <-b.ctx.Done():
					return
				case filtered <- e:
				}
			}
		}
	}()
	return filtered
}

// Close 关闭浏览器和所有页面。
func (b *Browser) Close() error {
	if b.BrowserContextID != "" {
		proto.TargetDisposeBrowserContext{BrowserContextID: b.BrowserContextID}.Method()
		b.Call(context.Background(), "", proto.TargetDisposeBrowserContext{
			BrowserContextID: b.BrowserContextID,
		})
	}
	b.cancel()
	if b.launcher != nil {
		// cleanup 由 launcher 管理
	}
	if b.client != nil {
		return b.client.Close()
	}
	return nil
}

func (b *Browser) pageFromTarget(ctx context.Context, targetID string) (*Page, error) {
	b.pagesMu.Lock()
	defer b.pagesMu.Unlock()

	res := proto.TargetAttachToTargetResult{}
	if err := b.CallResult(ctx, "", proto.TargetAttachToTarget{TargetID: targetID, Flatten: true}, &res); err != nil {
		return nil, err
	}

	sessionCtx, sessionCancel := context.WithCancel(b.ctx)

	p := &Page{
		browser:   b,
		sessionID: res.SessionID,
		targetID:  targetID,
		ctx:       sessionCtx,
		cancel:    sessionCancel,
		timeout:   b.timeout,
	}

	// 启用 Page 域以接收事件
	b.Call(ctx, p.sessionID, proto.PageEnable{})

	b.pages[targetID] = p
	return p, nil
}

// pageFromTarget 需要接收返回 — 修复上面的代码
```

- [ ] **Step 1: 编写完整的 obscura.go**

```go
package obscura

import (
	"context"
	"sync"
	"time"

	"github.com/8763232/obscura-go/cdp"
	"github.com/8763232/obscura-go/launcher"
	"github.com/8763232/obscura-go/proto"
)

// Browser 是 obscura 浏览器实例的控制句柄。
type Browser struct {
	client           *cdp.Client
	ctx              context.Context
	cancel           context.CancelFunc
	launchCleanup    func()
	pagesMu          sync.Mutex
	pages            map[string]*Page
	BrowserContextID string
	timeout          time.Duration
	eventCh          <-chan *cdp.Event
}

// New 创建 Browser 实例。
func New() *Browser {
	ctx, cancel := context.WithCancel(context.Background())
	return &Browser{
		ctx:     ctx,
		cancel:  cancel,
		pages:   make(map[string]*Page),
		timeout: 30 * time.Second,
	}
}

// Connect 连接到已运行的 obscura CDP 服务。
func (b *Browser) Connect(ctx context.Context, wsURL string) error {
	conn, err := cdp.Connect(ctx, wsURL)
	if err != nil {
		return err
	}

	b.client = cdp.NewClient(conn)
	b.eventCh = b.client.Events()

	return b.call(ctx, "", proto.TargetSetDiscoverTargets{Discover: true})
}

// Launch 使用 launcher 下载并启动 obscura。
func (b *Browser) Launch(ctx context.Context, opts ...func(*launcher.Launcher)) error {
	l := launcher.New()
	for _, o := range opts {
		o(l)
	}

	wsURL, cleanup, err := l.Launch(ctx)
	if err != nil {
		return err
	}
	b.launchCleanup = cleanup

	return b.Connect(ctx, wsURL)
}

// WithVersion 设置下载版本。
func WithVersion(v string) func(*launcher.Launcher) {
	return func(l *launcher.Launcher) { l.Version = v }
}

// WithPort 设置端口。
func WithPort(p int) func(*launcher.Launcher) {
	return func(l *launcher.Launcher) { l.Port = p }
}

// WithStealth 启用反检测模式。
func WithStealth() func(*launcher.Launcher) {
	return func(l *launcher.Launcher) { l.Stealth = true }
}

// WithProxy 设置代理。
func WithProxy(proxy string) func(*launcher.Launcher) {
	return func(l *launcher.Launcher) { l.Proxy = proxy }
}

// NewPage 创建新页面。
func (b *Browser) NewPage(ctx context.Context) (*Page, error) {
	var res proto.TargetCreateTargetResult
	if err := b.callResult(ctx, "", proto.TargetCreateTarget{URL: "about:blank"}, &res); err != nil {
		return nil, err
	}
	return b.pageFromTarget(ctx, res.TargetID)
}

// NewIncognito 创建隔离的浏览上下文。
func (b *Browser) NewIncognito(ctx context.Context) (*Browser, error) {
	var res proto.TargetCreateBrowserContextResult
	if err := b.callResult(ctx, "", proto.TargetCreateBrowserContext{}, &res); err != nil {
		return nil, err
	}

	incog := *b
	incog.BrowserContextID = res.BrowserContextID
	incog.pages = make(map[string]*Page)
	return &incog, nil
}

// Pages 返回所有活跃页面。
func (b *Browser) Pages() ([]*Page, error) {
	var res proto.TargetGetTargetsResult
	if err := b.callResult(b.ctx, "", proto.TargetGetTargets{}, &res); err != nil {
		return nil, err
	}

	var pages []*Page
	for _, info := range res.TargetInfos {
		if info.Type != "page" {
			continue
		}
		p, err := b.pageFromTarget(b.ctx, info.TargetID)
		if err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, nil
}

// Close 关闭浏览器。
func (b *Browser) Close() error {
	if b.BrowserContextID != "" {
		_ = b.call(context.Background(), "", proto.TargetDisposeBrowserContext{
			BrowserContextID: b.BrowserContextID,
		})
	} else {
		_ = b.call(context.Background(), "", proto.TargetCloseTarget{})
	}
	b.cancel()
	if b.launchCleanup != nil {
		b.launchCleanup()
	}
	if b.client != nil {
		return b.client.Close()
	}
	return nil
}

// call 发送 CDP 调用（忽略结果）。
func (b *Browser) call(ctx context.Context, sessionID string, req proto.Request) error {
	return b.client.Call(ctx, req.Method(), req, nil)
}

// callResult 发送 CDP 调用并解码结果。
func (b *Browser) callResult(ctx context.Context, sessionID string, req proto.Request, result any) error {
	return b.client.Call(ctx, req.Method(), req, result)
}

// pageFromTarget 从 targetID 创建 Page 实例。
func (b *Browser) pageFromTarget(ctx context.Context, targetID string) (*Page, error) {
	b.pagesMu.Lock()
	defer b.pagesMu.Unlock()

	if p, ok := b.pages[targetID]; ok {
		return p, nil
	}

	var res proto.TargetAttachToTargetResult
	if err := b.callResult(ctx, "", proto.TargetAttachToTarget{TargetID: targetID, Flatten: true}, &res); err != nil {
		return nil, err
	}

	sessionCtx, sessionCancel := context.WithCancel(b.ctx)

	p := &Page{
		browser:   b,
		sessionID: res.SessionID,
		targetID:  targetID,
		ctx:       sessionCtx,
		cancel:    sessionCancel,
		timeout:   b.timeout,
	}

	_ = b.call(ctx, p.sessionID, proto.PageEnable{})

	b.pages[targetID] = p
	return p, nil
}
```

- [ ] **Step 2: 验证编译**

Run: `go build .`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add obscura.go
git commit -m "feat: 添加 Browser 类型（连接/启动/页面管理/Incognito）"
```

---

### Task 20: Page 类型 (`page.go`)

**Files:**
- Create: `page.go`

- [ ] **Step 1: 编写 page.go**

```go
package obscura

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/8763232/obscura-go/cdp"
	"github.com/8763232/obscura-go/proto"
)

// Page 是浏览器页面的控制句柄。
type Page struct {
	browser   *Browser
	sessionID string
	targetID  string
	frameID   string
	ctx       context.Context
	cancel    context.CancelFunc
	timeout   time.Duration
}

// Navigate 导航到指定 URL，等待 loadEventFired。
func (p *Page) Navigate(ctx context.Context, url string) error {
	p.frameID = ""

	var navRes proto.PageNavigateResult
	if err := p.browser.callResult(ctx, p.sessionID, proto.PageNavigate{URL: url}, &navRes); err != nil {
		return err
	}
	p.frameID = navRes.FrameID

	return p.waitLoadEvent(ctx)
}

// WaitUntil 等待指定事件。
func (p *Page) WaitUntil(ctx context.Context, condition string) error {
	switch condition {
	case "load":
		return p.waitLoadEvent(ctx)
	case "domcontentloaded":
		return p.waitDOMContentEvent(ctx)
	default:
		return p.waitLoadEvent(ctx)
	}
}

func (p *Page) waitLoadEvent(ctx context.Context) error {
	ch := p.browser.filteredEvents("Page.loadEventFired", p.sessionID)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	}
}

func (p *Page) waitDOMContentEvent(ctx context.Context) error {
	ch := p.browser.filteredEvents("Page.domContentEventFired", p.sessionID)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	}
}

// Evaluate 执行 JavaScript 表达式。
func (p *Page) Evaluate(ctx context.Context, expression string, result any) error {
	var res proto.RuntimeEvaluateResult
	if err := p.browser.callResult(ctx, p.sessionID, proto.RuntimeEvaluate{
		Expression:    expression,
		ReturnByValue: true,
	}, &res); err != nil {
		return err
	}
	if res.ExceptionDetails != nil {
		return &Error{Message: "JS 执行异常"}
	}
	if res.Result != nil && res.Result.Value != nil && result != nil {
		return decodeValue(res.Result.Value, result)
	}
	return nil
}

// Element 通过 CSS 选择器查找单个元素。
func (p *Page) Element(ctx context.Context, selector string) (*Element, error) {
	var docRes proto.DOMGetDocumentResult
	if err := p.browser.callResult(ctx, p.sessionID, proto.DOMGetDocument{}, &docRes); err != nil {
		return nil, err
	}

	var qsRes proto.DOMQuerySelectorResult
	if err := p.browser.callResult(ctx, p.sessionID, proto.DOMQuerySelector{
		NodeID:   docRes.Root.NodeID,
		Selector: selector,
	}, &qsRes); err != nil {
		return nil, err
	}

	return &Element{
		page:     p,
		nodeID:   qsRes.NodeID,
		selector: selector,
		ctx:      p.ctx,
	}, nil
}

// Elements 通过 CSS 选择器查找多个元素。
func (p *Page) Elements(ctx context.Context, selector string) ([]*Element, error) {
	var docRes proto.DOMGetDocumentResult
	if err := p.browser.callResult(ctx, p.sessionID, proto.DOMGetDocument{}, &docRes); err != nil {
		return nil, err
	}

	var qsRes proto.DOMQuerySelectorAllResult
	if err := p.browser.callResult(ctx, p.sessionID, proto.DOMQuerySelectorAll{
		NodeID:   docRes.Root.NodeID,
		Selector: selector,
	}, &qsRes); err != nil {
		return nil, err
	}

	elements := make([]*Element, len(qsRes.NodeIDs))
	for i, id := range qsRes.NodeIDs {
		elements[i] = &Element{
			page:     p,
			nodeID:   id,
			selector: selector,
			ctx:      p.ctx,
		}
	}
	return elements, nil
}

// HTML 获取页面根节点 outerHTML。
func (p *Page) HTML(ctx context.Context) (string, error) {
	var docRes proto.DOMGetDocumentResult
	if err := p.browser.callResult(ctx, p.sessionID, proto.DOMGetDocument{}, &docRes); err != nil {
		return "", err
	}

	var htmlRes proto.DOMGetOuterHTMLResult
	if err := p.browser.callResult(ctx, p.sessionID, proto.DOMGetOuterHTML{NodeID: docRes.Root.NodeID}, &htmlRes); err != nil {
		return "", err
	}
	return htmlRes.OuterHTML, nil
}

// Markdown 获取页面的 Markdown 转换（Obscura 私有 API）。
func (p *Page) Markdown(ctx context.Context) (string, error) {
	var res proto.LPGetMarkdownResult
	if err := p.browser.callResult(ctx, p.sessionID, proto.LPGetMarkdown{}, &res); err != nil {
		return "", err
	}
	return res.Markdown, nil
}

// Screenshot 返回页面截图（PNG 格式，base64 编码）。
func (p *Page) Screenshot(ctx context.Context) ([]byte, error) {
	var res proto.PageCaptureScreenshotResult
	if err := p.browser.callResult(ctx, p.sessionID, proto.PageCaptureScreenshot{Format: "png"}, &res); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(res.Data)
}

// SetUserAgent 设置 User-Agent。
func (p *Page) SetUserAgent(ctx context.Context, ua string) error {
	return p.browser.call(ctx, p.sessionID, proto.NetworkSetUserAgentOverride{UserAgent: ua})
}

// SetViewport 设置视口大小。
func (p *Page) SetViewport(ctx context.Context, width, height int) error {
	return p.browser.call(ctx, p.sessionID, proto.PageSetDeviceMetricsOverride{
		Width:             width,
		Height:            height,
		DeviceScaleFactor: 1.0,
		Mobile:            false,
	})
}

// HijackRequests 创建网络拦截路由器（绑定到此页面 session）。
func (p *Page) HijackRequests() *HijackRouter {
	return newHijackRouter(p.browser, p.sessionID)
}

// Close 关闭页面。
func (p *Page) Close() error {
	p.cancel()
	return p.browser.call(context.Background(), "", proto.TargetCloseTarget{TargetID: p.targetID})
}

// filteredEvents 过滤出指定 method 和 sessionID 的事件。
func (b *Browser) filteredEvents(method, sessionID string) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range b.eventCh {
			if e.Method == method && (sessionID == "" || e.SessionID == sessionID) {
				return
			}
		}
	}()
	return done
}
```

- [ ] **Step 2: 验证编译**

Run: `go build .`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add page.go
git commit -m "feat: 添加 Page 类型（导航/JS执行/DOM/截图/拦截）"
```

---

### Task 21: 修复编译 — 添加 decodeValue 辅助函数

**Files:**
- Modify: `page.go`

> 注：`Evaluate` 中的 `decodeValue` 函数需要实现。

- [ ] **Step 1: 在 page.go 底部添加 decodeValue**

```go
import "encoding/json"

func decodeValue(src any, dst any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
```

- [ ] **Step 2: 验证编译**

Run: `go build .`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add page.go
git commit -m "fix: 添加 decodeValue 辅助函数"
```

---

### Task 22: Element 类型 (`element.go`)

**Files:**
- Create: `element.go`

- [ ] **Step 1: 编写 element.go**

```go
package obscura

import (
	"context"
	"strings"

	"github.com/8763232/obscura-go/proto"
)

// Element 是 DOM 元素的操作句柄。
// 不支持 iframe 和 shadow DOM 操作。
type Element struct {
	page     *Page
	nodeID   int
	selector string
	ctx      context.Context
}

// Click 点击元素。
func (el *Element) Click(ctx context.Context) error {
	// 获取元素的坐标（简化：使用 DOM.getBoxModel 计算中心点）
	// 此处简化为固定的点击位置
	return el.page.browser.call(ctx, el.page.sessionID, proto.InputDispatchMouseEvent{
		Type:       "mousePressed",
		X:          0,
		Y:          0,
		Button:     "left",
		ClickCount: 1,
	})
}

// TODO: 需要先获取元素的 boxModel 计算坐标，此处先定义简化版。
// 完整实现见 element_full.go
```

上面代码不完整。需要正确获取元素的坐标再点击。完整实现：

- [ ] **Step 1 (重写): 编写完整的 element.go**

```go
package obscura

import (
	"context"
	"encoding/json"

	"github.com/8763232/obscura-go/proto"
)

// Element 是 DOM 元素的操作句柄。
// 不支持 iframe 和 shadow DOM 操作。
type Element struct {
	page     *Page
	nodeID   int
	selector string
	ctx      context.Context
}

// Click 点击元素（先获取 boxModel 计算中心坐标）。
func (el *Element) Click(ctx context.Context) error {
	box := &domBoxModel{}
	if err := el.page.browser.callResult(ctx, el.page.sessionID,
		domGetBoxModel{NodeID: el.nodeID}, box); err != nil {
		return err
	}

	if len(box.Model.Content) < 8 {
		return ErrInvalidSelector
	}

	// Content quad: [x1,y1, x2,y2, x3,y3, x4,y4]
	cx := (box.Model.Content[0] + box.Model.Content[4]) / 2
	cy := (box.Model.Content[1] + box.Model.Content[5]) / 2

	for _, evt := range []proto.InputDispatchMouseEvent{
		{Type: "mouseMoved", X: cx, Y: cy},
		{Type: "mousePressed", X: cx, Y: cy, Button: "left", ClickCount: 1},
		{Type: "mouseReleased", X: cx, Y: cy, Button: "left", ClickCount: 1},
	} {
		if err := el.page.browser.call(ctx, el.page.sessionID, evt); err != nil {
			return err
		}
	}
	return nil
}

// Input 在元素中输入文本。
func (el *Element) Input(ctx context.Context, text string) error {
	for _, ch := range text {
		key := string(ch)
		if err := el.page.browser.call(ctx, el.page.sessionID, proto.InputDispatchKeyEvent{
			Type: "keyDown",
			Text: key,
		}); err != nil {
			return err
		}
		if err := el.page.browser.call(ctx, el.page.sessionID, proto.InputDispatchKeyEvent{
			Type: "keyUp",
			Text: key,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Text 获取元素的文本内容。
func (el *Element) Text(ctx context.Context) (string, error) {
	var resolveRes proto.DOMResolveNodeResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID,
		proto.DOMResolveNode{NodeID: el.nodeID}, &resolveRes); err != nil {
		return "", err
	}

	var propRes proto.RuntimeGetPropertiesResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID,
		proto.RuntimeGetProperties{ObjectID: resolveRes.Object.ObjectID, OwnOnly: true},
		&propRes); err != nil {
		return "", err
	}

	// 查找 innerText 属性
	for _, prop := range propRes.Result {
		if prop.Name == "innerText" && prop.Value != nil {
			if s, ok := prop.Value.Value.(string); ok {
				return s, nil
			}
		}
	}
	return "", nil
}

// HTML 获取元素的 outerHTML。
func (el *Element) HTML(ctx context.Context) (string, error) {
	var res proto.DOMGetOuterHTMLResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID,
		proto.DOMGetOuterHTML{NodeID: el.nodeID}, &res); err != nil {
		return "", err
	}
	return res.OuterHTML, nil
}

// Attribute 获取元素的属性值。
func (el *Element) Attribute(ctx context.Context, name string) (string, error) {
	var resolveRes proto.DOMResolveNodeResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID,
		proto.DOMResolveNode{NodeID: el.nodeID}, &resolveRes); err != nil {
		return "", err
	}

	// 通过 JS 获取属性
	js := fmt.Sprintf("function() { return this.getAttribute('%s'); }", name)
	var callRes proto.RuntimeCallFunctionOnResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID, proto.RuntimeCallFunctionOn{
		FunctionDeclaration: js,
		ObjectID:            resolveRes.Object.ObjectID,
		ReturnByValue:       true,
	}, &callRes); err != nil {
		return "", err
	}

	if callRes.Result != nil && callRes.Result.Value != nil {
		if s, ok := callRes.Result.Value.(string); ok {
			return s, nil
		}
	}
	return "", nil
}

// 获取 DOM boxModel（内联类型，不暴露）
type domBoxModel struct {
	Model struct {
		Content []float64 `json:"content"`
	} `json:"model"`
}

type domGetBoxModel struct {
	NodeID int `json:"nodeId"`
}

func (r domGetBoxModel) Method() string { return "DOM.getBoxModel" }
```

> 注：`domGetBoxModel` 使用了之前 proto 中未定义的类型。需要在 proto/dom.go 中补充 `DOMGetBoxModel`。

- [ ] **Step 2: 在 proto/dom.go 中补充 DOMGetBoxModel**

在 `proto/dom.go` 末尾添加：

```go
// DOM.getBoxModel
type DOMGetBoxModel struct {
	NodeID int `json:"nodeId"`
}

func (r DOMGetBoxModel) Method() string { return "DOM.getBoxModel" }

type DOMGetBoxModelResult struct {
	Model *DOMBoxModel `json:"model"`
}

type DOMBoxModel struct {
	Content []float64 `json:"content"`
}
```

然后修改 `element.go` 中的 `domGetBoxModel` 为使用 proto 包的类型：

将 element.go 底部的自定义类型删除，在 Click 方法中使用 proto 类型：

```go
func (el *Element) Click(ctx context.Context) error {
	var boxRes proto.DOMGetBoxModelResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID,
		proto.DOMGetBoxModel{NodeID: el.nodeID}, &boxRes); err != nil {
		return err
	}

	if boxRes.Model == nil || len(boxRes.Model.Content) < 8 {
		return ErrInvalidSelector
	}

	cx := (boxRes.Model.Content[0] + boxRes.Model.Content[4]) / 2
	cy := (boxRes.Model.Content[1] + boxRes.Model.Content[5]) / 2

	for _, evt := range []proto.InputDispatchMouseEvent{
		{Type: "mouseMoved", X: cx, Y: cy},
		{Type: "mousePressed", X: cx, Y: cy, Button: "left", ClickCount: 1},
		{Type: "mouseReleased", X: cx, Y: cy, Button: "left", ClickCount: 1},
	} {
		if err := el.page.browser.call(ctx, el.page.sessionID, evt); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 3: 验证编译**

Run: `go build .`
Expected: 编译成功

- [ ] **Step 4: Commit**

```bash
git add element.go proto/dom.go
git commit -m "feat: 添加 Element 类型（Click/Input/Text/HTML/Attribute）"
```

---

### Task 23: HijackRouter (`hijack.go`)

**Files:**
- Create: `hijack.go`

- [ ] **Step 1: 编写 hijack.go**

```go
package obscura

import (
	"context"
	"encoding/json"
	"regexp"
	"sync"

	"github.com/8763232/obscura-go/cdp"
	"github.com/8763232/obscura-go/proto"
)

// HijackRouter 是网络请求拦截路由器。
type HijackRouter struct {
	browser  *Browser
	sessionID string
	patterns []*proto.FetchRequestPattern
	handlers []*hijackHandlerItem
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	running  bool
}

type hijackHandlerItem struct {
	pattern string
	regexp  *regexp.Regexp
	handler func(ctx context.Context, req *HijackRequest, res *HijackResponse)
}

func newHijackRouter(b *Browser, sessionID string) *HijackRouter {
	ctx, cancel := context.WithCancel(b.ctx)
	return &HijackRouter{
		browser:   b,
		sessionID: sessionID,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Add 添加拦截规则。pattern 使用 glob 模式（如 "*/api/*"）。
// resourceType 可为 "Document"、"XHR"、"Script" 等，空字符串匹配所有。
func (r *HijackRouter) Add(pattern, resourceType string, handler func(ctx context.Context, req *HijackRequest, res *HijackResponse)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	re := globToRegex(pattern)

	r.patterns = append(r.patterns, &proto.FetchRequestPattern{
		URLPattern:   pattern,
		ResourceType: resourceType,
	})

	r.handlers = append(r.handlers, &hijackHandlerItem{
		pattern: pattern,
		regexp:  re,
		handler: handler,
	})

	// 更新 Fetch.enable 的拦截模式
	return r.browser.call(r.ctx, r.sessionID, &proto.FetchEnable{
		Patterns: r.patterns,
	})
}

// Run 启动拦截监听。
func (r *HijackRouter) Run() {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()

	go r.eventLoop()
}

func (r *HijackRouter) eventLoop() {
	ch := r.browser.filteredEventChannel("Fetch.requestPaused", r.sessionID)
	for {
		select {
		case <-r.ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			var paused proto.FetchRequestPaused
			if err := json.Unmarshal(e.Params, &paused); err != nil {
				continue
			}
			go r.handlePaused(&paused)
		}
	}
}

func (r *HijackRouter) handlePaused(e *proto.FetchRequestPaused) {
	req := &HijackRequest{
		URL:             e.Request.URL,
		Method:          e.Request.Method,
		Headers:         e.Request.Headers,
		Body:            e.Request.PostData,
		Type:            e.ResourceType,
		StatusCode:      e.ResponseStatusCode,
		ResponseHeaders: make(map[string]string),
	}

	for _, h := range e.ResponseHeaders {
		req.ResponseHeaders[h.Name] = h.Value
	}

	res := &HijackResponse{
		requestID: e.RequestID,
		client:    r.browser,
		sessionID: r.sessionID,
	}

	matched := false
	r.mu.Lock()
	for _, h := range r.handlers {
		if h.regexp.MatchString(e.Request.URL) {
			matched = true
			r.mu.Unlock()
			h.handler(r.ctx, req, res)
			r.mu.Lock()
		}
	}
	r.mu.Unlock()

	// 无匹配 handler，或 handler 没有明确决策 → 继续请求
	if !matched || (!res.fulfilled && !res.failed && !res.modified) {
		r.browser.call(context.Background(), r.sessionID, proto.FetchContinueRequest{
			RequestID: e.RequestID,
		})
		return
	}

	if res.fulfilled {
		r.browser.call(context.Background(), r.sessionID, proto.FetchFulfillRequest{
			RequestID:       e.RequestID,
			ResponseCode:    res.StatusCode,
			ResponseHeaders: headerMapToEntries(res.Headers),
			Body:            res.Body,
		})
		return
	}

	if res.failed {
		r.browser.call(context.Background(), r.sessionID, proto.FetchFailRequest{
			RequestID:   e.RequestID,
			ErrorReason: res.FailReason,
		})
		return
	}

	if res.modified && res.FollowURL != "" {
		r.browser.call(context.Background(), r.sessionID, proto.FetchFulfillRequest{
			RequestID:    e.RequestID,
			ResponseCode: 302,
			ResponseHeaders: []proto.FetchHeaderEntry{
				{Name: "Location", Value: res.FollowURL},
			},
		})
	}
}

// Stop 停止拦截。
func (r *HijackRouter) Stop() error {
	r.cancel()
	return r.browser.call(context.Background(), r.sessionID, proto.FetchDisable{})
}

// filteredEventChannel 返回过滤后的事件通道。
func (b *Browser) filteredEventChannel(method, sessionID string) <-chan *cdp.Event {
	ch := make(chan *cdp.Event, 64)
	go func() {
		defer close(ch)
		for e := range b.eventCh {
			if e.Method == method && (sessionID == "" || e.SessionID == sessionID) {
				select {
				case <-b.ctx.Done():
					return
				case ch <- e:
				}
			}
		}
	}()
	return ch
}

// globToRegex 将 glob 模式转为正则表达式。
func globToRegex(pattern string) *regexp.Regexp {
	reStr := regexp.QuoteMeta(pattern)
	reStr = strings.ReplaceAll(reStr, `\*`, ".*")
	reStr = strings.ReplaceAll(reStr, `\?`, ".")
	reStr = "^" + reStr + "$"
	return regexp.MustCompile(reStr)
}

func headerMapToEntries(headers map[string]string) []proto.FetchHeaderEntry {
	var entries []proto.FetchHeaderEntry
	for k, v := range headers {
		entries = append(entries, proto.FetchHeaderEntry{Name: k, Value: v})
	}
	return entries
}
```

需要在文件头部添加 `strings` 和 `encoding/json` 的 import（已在 `handlePaused` 中使用 `json.Unmarshal`）。

- [ ] **Step 2: 验证编译**

Run: `go build .`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add hijack.go
git commit -m "feat: 添加 HijackRouter 网络拦截系统"
```

---

### Task 24: HijackRequest / HijackResponse 类型 (`hijack_types.go`)

**Files:**
- Create: `hijack_types.go`

- [ ] **Step 1: 编写 hijack_types.go**

```go
package obscura

import (
	"context"
	"net/http"

	"github.com/8763232/obscura-go/proto"
)

// HijackRequest 是拦截到的网络请求。
type HijackRequest struct {
	URL    string
	Method string
	Headers map[string]string
	Body   string
	Type   string // "Document" | "XHR" | "Script" | ...

	// 响应阶段字段（StatusCode != 0 表示已收到响应，handler 在响应阶段被调用）
	StatusCode      int
	ResponseHeaders map[string]string
	RedirectChain   []string

	modified   bool
	newURL     string
	newMethod  string
	newHeaders map[string]string
	newBody    string
}

// Continue 标记此请求继续（修改后或原样）。
func (r *HijackRequest) Continue() {
	r.modified = true
}

// SetURL 修改请求 URL。
func (r *HijackRequest) SetURL(url string) {
	r.newURL = url
	r.modified = true
}

// SetMethod 修改请求方法。
func (r *HijackRequest) SetMethod(method string) {
	r.newMethod = method
	r.modified = true
}

// SetHeader 设置请求头。
func (r *HijackRequest) SetHeader(key, value string) {
	if r.newHeaders == nil {
		r.newHeaders = make(map[string]string)
	}
	r.newHeaders[key] = value
	r.modified = true
}

// SetBody 设置请求体。
func (r *HijackRequest) SetBody(body string) {
	r.newBody = body
	r.modified = true
}

// HijackResponse 控制对拦截请求的响应。
type HijackResponse struct {
	requestID  string
	client     *Browser
	sessionID  string
	fulfilled  bool
	failed     bool
	modified   bool
	StatusCode int
	Headers    map[string]string
	Body       string
	FailReason string
	FollowURL  string
}

// Fulfill 返回自定义响应，终止请求。
func (r *HijackResponse) Fulfill(code int, headers map[string]string, body string) {
	r.fulfilled = true
	r.StatusCode = code
	r.Headers = headers
	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}
	r.Body = body
}

// Fail 使请求失败。
func (r *HijackResponse) Fail(reason string) {
	r.failed = true
	r.FailReason = reason
}

// Follow 在响应阶段（301/302）跟随重定向。
func (r *HijackResponse) Follow() {
	r.modified = true
	r.FollowURL = "" // 空表示跟随原始 Location header，由事件循环处理
}

// FollowTo 修改重定向目标地址。
func (r *HijackResponse) FollowTo(newURL string) {
	r.modified = true
	r.FollowURL = newURL
}
```

- [ ] **Step 2: 验证编译**

Run: `go build .`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add hijack_types.go
git commit -m "feat: 添加 HijackRequest/HijackResponse 类型及重定向控制"
```

---

### Task 25: 修复 HijackRouter — Follow 逻辑

> 注：Task 23 中的 `handlePaused` 对 Follow 的处理不完整。需要修正 `hijack.go` 中 handlePaused 的逻辑。

- [ ] **Step 1: 修正 handlePaused 中的路由决策**

修改 `hijack.go` 的 `handlePaused` 方法，替换结尾部分：

```go
func (r *HijackRouter) handlePaused(e *proto.FetchRequestPaused) {
	req := &HijackRequest{
		URL:             e.Request.URL,
		Method:          e.Request.Method,
		Headers:         e.Request.Headers,
		Body:            e.Request.PostData,
		Type:            e.ResourceType,
		StatusCode:      e.ResponseStatusCode,
		ResponseHeaders: make(map[string]string),
	}

	for _, h := range e.ResponseHeaders {
		req.ResponseHeaders[h.Name] = h.Value
	}

	res := &HijackResponse{
		requestID: e.RequestID,
		client:    r.browser,
		sessionID: r.sessionID,
	}

	matched := false
	r.mu.Lock()
	for _, h := range r.handlers {
		if h.regexp.MatchString(e.Request.URL) {
			matched = true
			r.mu.Unlock()
			h.handler(r.ctx, req, res)
			r.mu.Lock()
			if res.fulfilled || res.failed {
				break
			}
		}
	}
	r.mu.Unlock()

	// 决策路由
	switch {
	case res.fulfilled:
		r.browser.call(context.Background(), r.sessionID, proto.FetchFulfillRequest{
			RequestID:       e.RequestID,
			ResponseCode:    res.StatusCode,
			ResponseHeaders: headerMapToEntries(res.Headers),
			Body:            res.Body,
		})

	case res.failed:
		r.browser.call(context.Background(), r.sessionID, proto.FetchFailRequest{
			RequestID:   e.RequestID,
			ErrorReason: res.FailReason,
		})

	case res.modified && res.FollowURL != "":
		// 修改重定向目标
		r.browser.call(context.Background(), r.sessionID, proto.FetchFulfillRequest{
			RequestID:    e.RequestID,
			ResponseCode: 302,
			ResponseHeaders: []proto.FetchHeaderEntry{
				{Name: "Location", Value: res.FollowURL},
			},
		})

	case res.modified && req.StatusCode != 0:
		// Follow(): 响应阶段跟随原始 Location
		r.browser.call(context.Background(), r.sessionID, proto.FetchContinueRequest{
			RequestID: e.RequestID,
		})

	case req.modified:
		// 请求阶段修改
		r.browser.call(context.Background(), r.sessionID, proto.FetchContinueRequest{
			RequestID: e.RequestID,
			URL:       req.newURL,
			Method:    req.newMethod,
			Headers:   headerMapToEntries(req.newHeaders),
			PostData:  req.newBody,
		})

	default:
		// 默认继续
		r.browser.call(context.Background(), r.sessionID, proto.FetchContinueRequest{
			RequestID: e.RequestID,
		})
	}
}
```

- [ ] **Step 2: 验证编译**

Run: `go build .`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add hijack.go
git commit -m "fix: 完善 HijackRouter 决策路由（Fulfill/Fail/Follow/修改/Continue）"
```

---

### Task 26: 示例 — basic (`examples/basic/main.go`)

**Files:**
- Create: `examples/basic/main.go`

- [ ] **Step 1: 编写 examples/basic/main.go**

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	obscura "github.com/8763232/obscura-go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	browser := obscura.New()
	if err := browser.Launch(ctx, obscura.WithStealth()); err != nil {
		log.Fatalf("启动 obscura 失败: %v", err)
	}
	defer browser.Close()

	page, err := browser.NewPage(ctx)
	if err != nil {
		log.Fatalf("创建页面失败: %v", err)
	}

	if err := page.Navigate(ctx, "https://news.ycombinator.com"); err != nil {
		log.Fatalf("导航失败: %v", err)
	}
	fmt.Println("页面加载完成")

	// 获取页面标题
	var title string
	if err := page.Evaluate(ctx, "document.title", &title); err != nil {
		log.Fatalf("获取标题失败: %v", err)
	}
	fmt.Printf("标题: %s\n", title)

	// 获取 Markdown 转换
	md, err := page.Markdown(ctx)
	if err != nil {
		log.Fatalf("Markdown 转换失败: %v", err)
	}
	fmt.Printf("Markdown 长度: %d 字符\n", len(md))

	// 截图
	png, err := page.Screenshot(ctx)
	if err != nil {
		log.Fatalf("截图失败: %v", err)
	}
	if err := os.WriteFile("screenshot.png", png, 0644); err != nil {
		log.Fatalf("保存截图失败: %v", err)
	}
	fmt.Println("截图已保存到 screenshot.png")
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./examples/basic`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add examples/basic/main.go
git commit -m "feat: 添加基础操作示例"
```

---

### Task 27: 示例 — hijack (`examples/hijack/main.go`)

**Files:**
- Create: `examples/hijack/main.go`

- [ ] **Step 1: 编写 examples/hijack/main.go**

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	obscura "github.com/8763232/obscura-go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	browser := obscura.New()
	if err := browser.Launch(ctx, obscura.WithStealth()); err != nil {
		log.Fatalf("启动 obscura 失败: %v", err)
	}
	defer browser.Close()

	page, err := browser.NewPage(ctx)
	if err != nil {
		log.Fatalf("创建页面失败: %v", err)
	}

	// 设置网络拦截
	router := page.HijackRequests()

	// 拦截特定 API 返回 mock 数据
	router.Add("*/api/*", "XHR", func(ctx context.Context, req *obscura.HijackRequest, res *obscura.HijackResponse) {
		fmt.Printf("[Mock] 拦截 API 请求: %s %s\n", req.Method, req.URL)
		res.Fulfill(200,
			map[string]string{"Content-Type": "application/json"},
			`{"message": "mock data from obscura-go"}`)
	})

	// 控制重定向
	router.Add("*", "", func(ctx context.Context, req *obscura.HijackRequest, res *obscura.HijackResponse) {
		// 响应阶段：检查重定向
		if req.StatusCode == 301 || req.StatusCode == 302 {
			location := req.ResponseHeaders["Location"]
			fmt.Printf("[Redirect] %d → %s\n", req.StatusCode, location)

			if location != "" && len(location) > 100 {
				fmt.Println("[Redirect] 阻止可疑的长 URL 重定向")
				res.Fail("BlockedByClient")
				return
			}
			res.Follow()
			return
		}

		// 请求阶段：打印所有请求
		if req.StatusCode == 0 {
			fmt.Printf("[Request] %s %s (%s)\n", req.Method, req.URL, req.Type)
		}
	})

	router.Run()
	defer router.Stop()

	fmt.Println("导航到目标页面...")
	if err := page.Navigate(ctx, "https://httpbin.org/redirect/1"); err != nil {
		log.Fatalf("导航失败: %v", err)
	}

	time.Sleep(2 * time.Second)
	fmt.Println("完成")
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./examples/hijack`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add examples/hijack/main.go
git commit -m "feat: 添加网络拦截示例（mock API + 重定向控制）"
```

---

### Task 28: 示例 — concurrent (`examples/concurrent/main.go`)

**Files:**
- Create: `examples/concurrent/main.go`

- [ ] **Step 1: 编写 examples/concurrent/main.go**

```go
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	obscura "github.com/8763232/obscura-go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	browser := obscura.New()
	if err := browser.Launch(ctx); err != nil {
		log.Fatalf("启动 obscura 失败: %v", err)
	}
	defer browser.Close()

	urls := []string{
		"https://example.com",
		"https://httpbin.org",
		"https://news.ycombinator.com",
	}

	var wg sync.WaitGroup
	results := make(chan string, len(urls))

	for _, url := range urls {
		wg.Add(1)
		go func(targetURL string) {
			defer wg.Done()

			incog, err := browser.NewIncognito(ctx)
			if err != nil {
				log.Printf("创建 incognito 失败: %v", err)
				return
			}
			defer incog.Close()

			page, err := incog.NewPage(ctx)
			if err != nil {
				log.Printf("创建页面失败: %v", err)
				return
			}

			if err := page.Navigate(ctx, targetURL); err != nil {
				log.Printf("导航 %s 失败: %v", targetURL, err)
				return
			}

			var title string
			if err := page.Evaluate(ctx, "document.title", &title); err != nil {
				log.Printf("获取 %s 标题失败: %v", targetURL, err)
				return
			}

			results <- fmt.Sprintf("[%s] %s", targetURL, title)
		}(url)
	}

	wg.Wait()
	close(results)

	fmt.Println("并发抓取结果:")
	for r := range results {
		fmt.Println(r)
	}
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./examples/concurrent`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add examples/concurrent/main.go
git commit -m "feat: 添加并发多页面示例"
```

---

### Task 29: 全局编译验证

- [ ] **Step 1: 编译所有包**

Run: `go build ./...`
Expected: 所有包编译成功，无错误

- [ ] **Step 2: 运行 go vet**

Run: `go vet ./...`
Expected: 无警告

- [ ] **Step 3: Commit（如有修改）**

```bash
git add -A
git commit -m "chore: 全局编译修复与验证"
```

---

## 实现顺序总结

```
Task 1:  项目初始化（目录结构 + go.mod）
Task 2:  cdp/conn.go — WebSocket 连接
Task 3:  cdp/client.go + cdp/event.go — JSON-RPC 客户端
Task 4:  cdp — 修复 SessionID 传递
Task 5:  proto/common.go — 公共接口
Task 6:  proto/target.go
Task 7:  proto/page.go
Task 8:  proto/runtime.go
Task 9:  proto/dom.go
Task 10: proto/network.go
Task 11: proto/fetch.go
Task 12: proto/storage.go
Task 13: proto/input.go
Task 14: proto/lp.go
Task 15: launcher/browser.go — 下载
Task 16: launcher/launcher.go — 进程
Task 17: error.go
Task 18: context.go
Task 19: obscura.go — Browser
Task 20: page.go — Page
Task 21: page.go — decodeValue 修复
Task 22: element.go + proto/dom.go 补充
Task 23: hijack.go — HijackRouter 事件循环
Task 24: hijack_types.go — HijackRequest/Response
Task 25: hijack.go — Follow 逻辑修复
Task 26: examples/basic/
Task 27: examples/hijack/
Task 28: examples/concurrent/
Task 29: 全局编译验证
```
