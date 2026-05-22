package obscura

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
	pagesMu          *sync.Mutex
	pages            map[string]*Page
	BrowserContextID string
	timeout          time.Duration
	eventCh          <-chan *cdp.Event

	subMu       sync.Mutex
	subscribers map[int]chan *cdp.Event
	nextSubID   int
}

// New 创建 Browser 实例。
func New() *Browser {
	ctx, cancel := context.WithCancel(context.Background())
	return &Browser{
		ctx:         ctx,
		cancel:      cancel,
		pages:       make(map[string]*Page),
		pagesMu:     &sync.Mutex{},
		timeout:     30 * time.Second,
		subscribers: make(map[int]chan *cdp.Event),
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

	// 启动事件广播 goroutine
	go b.broadcast()

	return b.call(ctx, "", proto.TargetSetDiscoverTargets{Discover: true})
}

// broadcast 将 CDP 事件广播给所有订阅者。
func (b *Browser) broadcast() {
	for {
		select {
		case <-b.ctx.Done():
			return
		case e, ok := <-b.eventCh:
			if !ok {
				return
			}
			b.subMu.Lock()
			for _, ch := range b.subscribers {
				select {
				case ch <- e:
				default:
					// 订阅者通道满了，跳过
				}
			}
			b.subMu.Unlock()
		}
	}
}

// subscribe 订阅 CDP 事件，返回唯一 ID 和专用通道。
func (b *Browser) subscribe() (int, <-chan *cdp.Event) {
	b.subMu.Lock()
	defer b.subMu.Unlock()

	id := b.nextSubID
	b.nextSubID++
	ch := make(chan *cdp.Event, 128)
	b.subscribers[id] = ch
	return id, ch
}

// unsubscribe 取消订阅。
func (b *Browser) unsubscribe(id int) {
	b.subMu.Lock()
	defer b.subMu.Unlock()

	delete(b.subscribers, id)
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

// Serve 直接启动本地的 obscura 二进制（跳过下载），并连接 CDP。
// 默认从 .cache/obscura-go/latest/obscura 查找，可用 WithBinPath 指定路径。
func (b *Browser) Serve(ctx context.Context, opts ...func(*launcher.Launcher)) error {
	// 默认使用缓存目录中的二进制
	binName := "obscura"
	if runtime.GOOS == "windows" {
		binName = "obscura.exe"
	}
	defaultBin := filepath.Join(".cache", "obscura-go", "latest", binName)
	if _, err := os.Stat(defaultBin); err != nil {
		// 回退：从 launcher 目录查找
		defaultBin = filepath.Join("launcher", runtime.GOOS+"_"+runtime.GOARCH, "latest", binName)
	}

	l := launcher.New()
	l.BinPath = defaultBin
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

// WithBinPath 直接指定 obscura 二进制路径，跳过下载。
func WithBinPath(p string) func(*launcher.Launcher) {
	return func(l *launcher.Launcher) { l.BinPath = p }
}

// IgnoreCertErrors 忽略 HTTPS 证书错误。
func (b *Browser) IgnoreCertErrors(ignore bool) error {
	return b.call(b.ctx, "", proto.SecuritySetIgnoreCertificateErrors{Ignore: ignore})
}

// NewPage 创建新页面。
func (b *Browser) NewPage(ctx context.Context) (*Page, error) {
	var res proto.TargetCreateTargetResult
	if err := b.callResult(ctx, "", proto.TargetCreateTarget{
		URL:              "about:blank",
		BrowserContextID: b.BrowserContextID,
	}, &res); err != nil {
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

	incog := &Browser{
		client:           b.client,
		ctx:              b.ctx,
			cancel:           func() {},
		eventCh:          b.eventCh,
		pages:            make(map[string]*Page),
		pagesMu:          &sync.Mutex{},
		BrowserContextID: res.BrowserContextID,
		timeout:          b.timeout,
		subscribers:      b.subscribers,
	}
	return incog, nil
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if b.BrowserContextID != "" {
		_ = b.call(ctx, "", proto.TargetDisposeBrowserContext{
			BrowserContextID: b.BrowserContextID,
		})
	}
	b.cancel()
	if b.launchCleanup != nil {
		b.launchCleanup()
	}
	if b.client != nil && b.BrowserContextID == "" {
		return b.client.Close()
	}
	return nil
}

// call 发送 CDP 调用（忽略结果）。
func (b *Browser) call(ctx context.Context, sessionID string, req proto.Request) error {
	return b.client.Call(ctx, sessionID, req.Method(), req, nil)
}

// callResult 发送 CDP 调用并解码结果。
func (b *Browser) callResult(ctx context.Context, sessionID string, req proto.Request, result any) error {
	return b.client.Call(ctx, sessionID, req.Method(), req, result)
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
