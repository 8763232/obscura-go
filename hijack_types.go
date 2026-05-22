package obscura

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

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

	req *http.Request // 从 CDP 事件构建，供 LoadResponse 使用
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

	// SetCookieHeaders 收集 LoadResponse 跟随重定向时遇到的所有 Set-Cookie。
	SetCookieHeaders []string
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
	r.FollowURL = ""
}

// FollowTo 修改重定向目标地址。
func (r *HijackResponse) FollowTo(newURL string) {
	r.modified = true
	r.FollowURL = newURL
}

// LoadResponse 使用 HTTP 客户端发起真实网络请求，自动跟随重定向（最多 20 次），
// 重定向链中使用 cookiejar 自动管理 Cookie（Set-Cookie → 后续请求的 Cookie 头）。
// 收集所有 Set-Cookie 到 SetCookieHeaders，通过 handlePaused 自动注入 obscura。
// 最终非重定向响应通过 res.Fulfill 返回。
func (req *HijackRequest) LoadResponse(client *http.Client, res *HijackResponse) error {
	if client == nil {
		client = http.DefaultClient
	}

	noRedirect := new(http.Client)
	*noRedirect = *client
	noRedirect.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if noRedirect.Timeout == 0 {
		noRedirect.Timeout = 30 * time.Second
	}

	// 使用 cookiejar 在重定向链中自动管理 Cookie
	jar, _ := cookiejar.New(nil)
	noRedirect.Jar = jar

	currentReq := req.req
	var allCookies []string

	for i := 0; i < 20; i++ {
		httpResp, err := noRedirect.Do(currentReq)
		if err != nil {
			return err
		}

		// 收集 Set-Cookie
		for _, sc := range httpResp.Header["Set-Cookie"] {
			allCookies = append(allCookies, sc)
		}

		// 非重定向 → 返回最终响应
		if httpResp.StatusCode < 300 || httpResp.StatusCode >= 400 {
			body, _ := io.ReadAll(httpResp.Body)
			httpResp.Body.Close()

			headers := make(map[string]string)
			for k, vs := range httpResp.Header {
				if len(vs) > 0 {
					headers[k] = vs[0]
				}
			}

			res.SetCookieHeaders = allCookies
			res.Fulfill(httpResp.StatusCode, headers, string(body))
			return nil
		}

		// 重定向：提取 Location，构造新请求（cookiejar 自动携带 Cookie）
		location := httpResp.Header.Get("Location")
		httpResp.Body.Close()
		if location == "" {
			break
		}

		newURL, err := currentReq.URL.Parse(location)
		if err != nil {
			return err
		}

		newReq, _ := http.NewRequest("GET", newURL.String(), nil)
		for k, vs := range currentReq.Header {
			for _, v := range vs {
				if k != "Cookie" && k != "Host" {
					newReq.Header.Add(k, v)
				}
			}
		}
		currentReq = newReq
	}

	return fmt.Errorf("obscura: 太多重定向")
}

// InjectCookies 将 SetCookieHeaders 中的 Cookie 注入到 obscura 浏览器。
// 在 LoadResponse 之后调用。
func (res *HijackResponse) InjectCookies(ctx context.Context, browser *Browser) error {
	for _, raw := range res.SetCookieHeaders {
		cookie := parseSetCookie(raw)
		if cookie == nil {
			continue
		}
		_ = browser.SetCookies(ctx, []*proto.CookieParam{cookie})
	}
	return nil
}

// parseSetCookie 解析 Set-Cookie 头字符串为 proto.CookieParam。
func parseSetCookie(raw string) *proto.CookieParam {
	parts := strings.Split(raw, ";")
	if len(parts) == 0 {
		return nil
	}

	nameVal := strings.SplitN(strings.TrimSpace(parts[0]), "=", 2)
	if len(nameVal) < 2 {
		return nil
	}

	cookie := &proto.CookieParam{
		Name:  strings.TrimSpace(nameVal[0]),
		Value: strings.TrimSpace(nameVal[1]),
	}

	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		lower := strings.ToLower(part)

		switch {
		case lower == "secure":
			cookie.Secure = true
		case lower == "httponly":
			cookie.HTTPOnly = true
		case strings.HasPrefix(lower, "domain="):
			cookie.Domain = part[7:]
		case strings.HasPrefix(lower, "path="):
			cookie.Path = part[5:]
		case strings.HasPrefix(lower, "samesite="):
			cookie.SameSite = part[9:]
		}
	}

	return cookie
}
