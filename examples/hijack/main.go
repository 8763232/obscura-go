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
	//if err := browser.Connect(ctx, "ws://127.0.0.1:9222"); err != nil {
	//	log.Fatalf("连接 obscura 失败: %v", err)
	//}

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
	// 使用自定义 HTTP 客户端：跳过 TLS 验证。默认自动跟随重定向，
	// 如需手动控制重定向，设置 CheckRedirect: http.ErrUseLastResponse
	// 并在 handler 的响应阶段（StatusCode==301/302）处理 res.Follow()/FollowTo()
	router.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 30 * time.Second,
	}

	router.Add("*", "", func(ctx context.Context, req *obscura.HijackRequest, res *obscura.HijackResponse) {
		// 响应阶段：处理重定向
		if req.StatusCode == 301 || req.StatusCode == 302 {
			location := req.ResponseHeaders["Location"]
			fmt.Printf("  [Redirect] %d → %s\n", req.StatusCode, location)
			res.Follow() // 让 obscura 跟随重定向
			return
		}
		// 请求阶段：Go 端代理请求
		if req.StatusCode == 0 {
			fmt.Printf("  [Proxy] %s %s\n", req.Method, req.URL)

			if err := req.LoadResponse(router.HTTPClient, res); err != nil {
				fmt.Printf("  [Proxy] 请求失败: %v\n", err)
				res.Fail("Failed")
				return
			}
			fmt.Printf("  [Proxy] → %d (%d字节)\n", res.StatusCode, len(res.Body))
		}
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
