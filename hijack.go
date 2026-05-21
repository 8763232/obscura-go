package obscura

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"

	"github.com/8763232/obscura-go/cdp"
	"github.com/8763232/obscura-go/proto"
)

// HijackRouter 是网络请求拦截路由器。
type HijackRouter struct {
	browser   *Browser
	sessionID string
	patterns  []*proto.FetchRequestPattern
	handlers  []*hijackHandlerItem
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	running   bool
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
	ch := make(chan *cdp.Event, 64)
	go func() {
		defer close(ch)
		for e := range r.browser.eventCh {
			if e.Method == "Fetch.requestPaused" && (r.sessionID == "" || e.SessionID == r.sessionID) {
				select {
				case <-r.ctx.Done():
					return
				case ch <- e:
				}
			}
		}
	}()

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

	// 匹配并执行 handler
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
		// FollowTo: 修改重定向目标
		r.browser.call(context.Background(), r.sessionID, proto.FetchFulfillRequest{
			RequestID:    e.RequestID,
			ResponseCode: 302,
			ResponseHeaders: []proto.FetchHeaderEntry{
				{Name: "Location", Value: res.FollowURL},
			},
		})

	case res.modified && req.StatusCode != 0:
		// Follow(): 响应阶段跟随原始 Location header
		r.browser.call(context.Background(), r.sessionID, proto.FetchContinueRequest{
			RequestID: e.RequestID,
		})

	case req.modified:
		// 请求阶段修改（SetURL/SetMethod/SetHeader/SetBody）
		r.browser.call(context.Background(), r.sessionID, proto.FetchContinueRequest{
			RequestID: e.RequestID,
			URL:       req.newURL,
			HTTPMethod: req.newMethod,
			Headers:   headerMapToEntries(req.newHeaders),
			PostData:  req.newBody,
		})

	default:
		// 默认：原样继续
		if matched {
			r.browser.call(context.Background(), r.sessionID, proto.FetchContinueRequest{
				RequestID: e.RequestID,
			})
		}
	}
}

// Stop 停止拦截。
func (r *HijackRouter) Stop() error {
	r.cancel()
	return r.browser.call(context.Background(), r.sessionID, proto.FetchDisable{})
}

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
