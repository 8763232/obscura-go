package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
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
		log.Fatalf("启动 obscura 失败: %v", err)
	}
	defer browser.Close()

	// === 示例 1：正常证书 URL，LoadResponse 代理主文档 ===
	fmt.Println("=== 示例 1: LoadResponse 代理模式 ===")
	demoLoadResponse(ctx, browser)

	// === 示例 2：自签/过期证书 URL，Go 端获取后注入 ===
	fmt.Println("\n=== 示例 2: Go 端获取 + 注入页面内容 ===")
	demoInjectContent(ctx, browser)
}

// demoLoadResponse 演示对正常证书 URL 的代理拦截
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
			fmt.Printf("  [Proxy] 失败: %v\n", err)
			res.Fail("Failed")
			return
		}
		fmt.Printf("  [Proxy] → %d (%d字节)\n", res.StatusCode, len(res.Body))
	})

	router.Run()
	defer router.Stop()

	page.Navigate(ctx, "https://httpbin.org/get")
	var title string
	page.Evaluate(ctx, "document.title", &title)
	fmt.Printf("  标题: %s\n", title)
}

// demoInjectContent 演示绕过 obscura TLS：Go 端获取 HTML，注入到页面
func demoInjectContent(ctx context.Context, browser *obscura.Browser) {
	// 1. Go 端用自定义 HTTP 客户端获取内容
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get("https://login.teamviewer.com/Cmd/ActivateAccount?lng=zhcn&token=f865bfb8-99c9-4dbe-9c30-5b1b109a9bd4")
	if err != nil {
		log.Printf("  Go 端请求失败: %v", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	fmt.Printf("  Go 端获取: %d, %d字节\n", resp.StatusCode, len(html))

	// 2. 导航到 about:blank
	page, _ := browser.NewPage(ctx)
	page.Navigate(ctx, "about:blank")

	// 3. 注入 HTML 内容
	page.Evaluate(ctx, fmt.Sprintf("document.write(%q); document.close();", html), nil)

	var title string
	page.Evaluate(ctx, "document.title", &title)
	fmt.Printf("  标题: %s\n", title)
}
