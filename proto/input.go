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
