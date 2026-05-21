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

// 注意：已包含 SessionID 字段
type rpcResponse struct {
	ID        int64            `json:"id,omitempty"`
	Result    json.RawMessage  `json:"result,omitempty"`
	Error     *json.RawMessage `json:"error,omitempty"`
	Method    string           `json:"method,omitempty"`
	Params    json.RawMessage  `json:"params,omitempty"`
	SessionID string           `json:"sessionId,omitempty"`
}

// Client 是 JSON-RPC 客户端。
type Client struct {
	conn    *Conn
	reqID   int64
	mu      sync.Mutex
	pending map[int64]chan *rpcResponse
	events  chan *Event
	done    chan struct{}
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
			// 事件：无 id，有 method → 发送到事件通道（包含 SessionID）
			c.events <- &Event{
				Method:    resp.Method,
				SessionID: resp.SessionID,
				Params:    resp.Params,
			}
		} else {
			// 响应：有 id → 路由到对应的 pending channel
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
	return c.conn.Close()
}
