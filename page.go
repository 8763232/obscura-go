package obscura

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

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

	// 提前订阅事件，避免竞态
	id, ch := p.browser.subscribe()
	defer p.browser.unsubscribe(id)

	var navRes proto.PageNavigateResult
	if err := p.browser.callResult(ctx, p.sessionID, proto.PageNavigate{URL: url}, &navRes); err != nil {
		return err
	}
	p.frameID = navRes.FrameID

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e, ok := <-ch:
			if !ok {
				return ctx.Err()
			}
			if e.Method == "Page.loadEventFired" && (p.sessionID == "" || e.SessionID == p.sessionID) {
				return nil
			}
		}
	}
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
	id, ch := p.browser.subscribe()
	defer p.browser.unsubscribe(id)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e, ok := <-ch:
			if !ok {
				return ctx.Err()
			}
			if e.Method == "Page.loadEventFired" && (p.sessionID == "" || e.SessionID == p.sessionID) {
				return nil
			}
		}
	}
}

func (p *Page) waitDOMContentEvent(ctx context.Context) error {
	id, ch := p.browser.subscribe()
	defer p.browser.unsubscribe(id)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e, ok := <-ch:
			if !ok {
				return ctx.Err()
			}
			if e.Method == "Page.domContentEventFired" && (p.sessionID == "" || e.SessionID == p.sessionID) {
				return nil
			}
		}
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

func decodeValue(src any, dst any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
