package cdp

import "encoding/json"

// Event 是 CDP 事件。
type Event struct {
	Method    string          `json:"method"`
	SessionID string          `json:"sessionId"`
	Params    json.RawMessage `json:"params"`
}
