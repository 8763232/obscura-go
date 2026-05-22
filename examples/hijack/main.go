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
	if err := browser.Serve(ctx, obscura.WithStealth()); err != nil {
		log.Fatalf("连接 obscura 失败: %v", err)
	}
	defer browser.Close()

	// === 示例 1：LoadResponse 代理模式 ===
	fmt.Println("=== 示例 1: LoadResponse 代理模式 ===")
	demoLoadResponse(ctx, browser)

	// === 示例 2：Mock 响应 ===
	fmt.Println("\n=== 示例 2: Mock 响应 ===")
	demoMock(ctx, browser)
}

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
			return
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

	// 忽略证书错误，通过 Go 端代理发起请求
	browser.IgnoreCertErrors(true)
	page.Navigate(ctx, "https://login.teamviewer.com/Cmd/ActivateAccount?lng=zhcn&token=f865bfb8-99c9-4dbe-9c30-5b1b109a9bd4")

	var title string
	page.Evaluate(ctx, "document.title", &title)
	fmt.Printf("  标题: %s\n", title)
}

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
			if req.StatusCode == 301 || req.StatusCode == 302 {
				fmt.Printf("  [Redirect] %d → %s\n", req.StatusCode, req.ResponseHeaders["Location"])
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
