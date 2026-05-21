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
