package launcher

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// Launcher 管理 obscura 进程的启动。
type Launcher struct {
	Version string // "latest" 或具体版本号
	BinPath string // 直接指定 obscura 二进制路径，跳过下载
	Port    int    // 0 = 随机端口
	Proxy   string // HTTP/SOCKS5 代理
	Stealth bool   // 反检测模式
	Workers int    // worker 进程数

	browser *Browser
	cmd     *exec.Cmd
	mu      sync.Mutex
}

// New 创建默认 Launcher。
func New() *Launcher {
	return &Launcher{
		browser: NewBrowser(),
		Version: "latest",
		Port:    0,
		Workers: 1,
	}
}

// Launch 下载（如需要）并启动 obscura serve 进程。
// 如果 BinPath 已设置，跳过下载直接启动。
// 返回 CDP WebSocket URL 和 cleanup 函数。
func (l *Launcher) Launch(ctx context.Context) (wsURL string, cleanup func(), err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var binPath string

	if l.BinPath != "" {
		binPath = l.BinPath
	} else {
		// 传递 Version 到 browser（仅当设置时覆盖默认值）
		if l.Version != "" {
			l.browser.Version = l.Version
		}
		binPath, err = l.browser.Get(ctx)
		if err != nil {
			return "", nil, err
		}
	}

	wsURL, cleanup, err = l.startAndWait(ctx, binPath)
	return
}

// startAndWait 启动 obscura serve 并等待就绪。
func (l *Launcher) startAndWait(ctx context.Context, binPath string) (wsURL string, cleanup func(), err error) {
	port := l.Port
	if port == 0 {
		p, err := randomPort()
		if err != nil {
			return "", nil, fmt.Errorf("launcher: 无可用端口: %w", err)
		}
		port = p
	}

	args := []string{"serve", "--port", strconv.Itoa(port)}
	if l.Proxy != "" {
		args = append(args, "--proxy", l.Proxy)
	}
	if l.Stealth {
		args = append(args, "--stealth")
	}
	if l.Workers > 1 {
		args = append(args, "--workers", strconv.Itoa(l.Workers))
	}

	l.cmd = exec.CommandContext(ctx, binPath, args...)
	if err := l.cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("launcher: 启动 obscura 失败: %w", err)
	}

	wsURL = fmt.Sprintf("ws://127.0.0.1:%d/devtools/browser", port)

	// 等待 WebSocket 就绪
	if err := l.waitReady(ctx, port); err != nil {
		l.cmd.Process.Kill()
		return "", nil, fmt.Errorf("launcher: obscura 未就绪: %w", err)
	}

	cleanup = func() {
		if l.cmd != nil && l.cmd.Process != nil {
			l.cmd.Process.Kill()
		}
	}

	return wsURL, cleanup, nil
}

func (l *Launcher) waitReady(ctx context.Context, port int) error {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("连接超时")
		}

		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func randomPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
