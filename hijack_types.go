package obscura

import (
	"io"
	"net/http"
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

// Follow 在响应阶段（301/302）跟随重定向。仅在 StatusCode != 0 时有效。
func (r *HijackResponse) Follow() {
	r.modified = true
	r.FollowURL = "" // 空表示跟随原始 Location header
}

// FollowTo 修改重定向目标地址。
func (r *HijackResponse) FollowTo(newURL string) {
	r.modified = true
	r.FollowURL = newURL
}

// LoadResponse 使用指定的 HTTP 客户端发起真实网络请求，并将响应注入到 HijackResponse。
// 调用后 handler 中无需再调用 res.Fulfill，响应已自动设置。
// 如果 client 为 nil，使用 http.DefaultClient。
func (req *HijackRequest) LoadResponse(client *http.Client, res *HijackResponse) error {
	if client == nil {
		client = http.DefaultClient
	}

	httpResp, err := client.Do(req.req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}

	headers := make(map[string]string)
	for k, vs := range httpResp.Header {
		if len(vs) > 0 {
			headers[k] = vs[0]
		}
	}

	res.Fulfill(httpResp.StatusCode, headers, string(body))
	return nil
}
