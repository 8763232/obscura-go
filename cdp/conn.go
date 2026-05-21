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
	frame := []byte{0x81}

	length := len(msg)
	switch {
	case length <= 125:
		frame = append(frame, byte(length|0x80))
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

// Read 读取下一帧的 payload。
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
