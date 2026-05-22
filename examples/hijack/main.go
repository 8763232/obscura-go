package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"

	obscura "github.com/8763232/obscura-go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	browser := obscura.New()
	if err := browser.Connect(ctx, "ws://127.0.0.1:9222/devtools/browser"); err != nil {
		log.Fatalf("连接 obscura 失败: %v", err)
	}
	defer browser.Close()

	// === 示例 1：LoadResponse 代理模式 ===
	fmt.Println("=== 示例 1: LoadResponse 代理模式 ===")
	demoLoadResponse(ctx, browser)

	// === 示例 2：mock 响应（不发起网络请求） ===
	fmt.Println("\n=== 示例 2: Mock 响应 ===")
	demoMock(ctx, browser)
}

// demoLoadResponse: 对所有请求用 Go HTTP 客户端代理
func demoLoadResponse(ctx context.Context, browser *obscura.Browser) {
	page, _ := browser.NewPage(ctx)

	router := page.HijackRequests()
	router.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 30 * time.Second,
	}

	router.Add("*", "", func(ctx context.Context, req *obscura.HijackRequest, res *obscura.HijackResponse) {
		if req.StatusCode != 0 {
			return // 跳过响应阶段
		}
		fmt.Printf("  [Proxy] %s %s\n", req.Method, req.URL)

		if err := req.LoadResponse(router.HTTPClient, res); err != nil {
			fmt.Printf("  [Proxy] 请求失败: %v\n", err)
			res.Fail("Failed")
			return
		}
		fmt.Printf("  [Proxy] → %d (%d字节)\n", res.StatusCode, len(res.Body))
	})

	router.Run()
	defer router.Stop()

	page.Navigate(ctx, "https://httpbin.org/get")
}

// demoMock: 直接返回 mock 数据，不发起真实网络请求
func demoMock(ctx context.Context, browser *obscura.Browser) {
	page, _ := browser.NewPage(ctx)

	router := page.HijackRequests()

	router.Add("*/api/*", "XHR", func(ctx context.Context, req *obscura.HijackRequest, res *obscura.HijackResponse) {
		fmt.Printf("  [Mock] %s %s → 返回 mock JSON\n", req.Method, req.URL)
		res.Fulfill(200,
			map[string]string{"Content-Type": "application/json"},
			`{"users": [{"id": 1, "name": "test"}]}`)
	})

	router.Add("*", "", func(ctx context.Context, req *obscura.HijackRequest, res *obscura.HijackResponse) {
		if req.StatusCode != 0 {
			// 响应阶段：处理重定向
			if req.StatusCode == 301 || req.StatusCode == 302 {
				fmt.Printf("  [Redirect] %d → %s\n", req.StatusCode,
					req.ResponseHeaders["Location"])
				res.Follow()
			}
			return
		}
		fmt.Printf("  [Request] %s %s (%s)\n", req.Method, req.URL, req.Type)
	})

	router.Run()
	defer router.Stop()

	page.Navigate(ctx, "https://httpbin.org/get")
}
