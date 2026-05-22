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
		log.Fatalf("启动 obscura 失败: %v", err)
	}
	defer browser.Close()

	page, err := browser.NewPage(ctx)
	if err != nil {
		log.Fatalf("创建页面失败: %v", err)
	}

	// 设置网络拦截 — 在 Navigate 之前
	router := page.HijackRequests()

	// 自定义 HTTP 客户端：跳过 TLS 验证（解决自签名/过期证书问题）
	router.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 30 * time.Second,
	}

	// 对所有请求：Go 端代理发起真实 HTTP 请求，注入响应回 obscura
	router.Add("*", "", func(ctx context.Context, req *obscura.HijackRequest, res *obscura.HijackResponse) {
		if req.StatusCode != 0 {
			return // 响应阶段不处理
		}
		fmt.Printf("[Proxy] %s %s\n", req.Method, req.URL)

		if err := req.LoadResponse(router.HTTPClient, res); err != nil {
			fmt.Printf("[Proxy] 请求失败: %v\n", err)
			res.Fail("Failed")
			return
		}
		fmt.Printf("[Proxy] 响应: %d, body=%d字节\n", res.StatusCode, len(res.Body))
	})

	router.Run()
	defer router.Stop()

	fmt.Println("导航到目标页面...")
	err = page.Navigate(ctx, "https://login.teamviewer.com/Cmd/ActivateAccount?lng=zhcn&token=f865bfb8-99c9-4dbe-9c30-5b1b109a9bd4")
	if err != nil {
		log.Fatalf("导航失败: %v", err)
	}

	var title string
	page.Evaluate(ctx, "document.title", &title)
	fmt.Printf("页面标题: %s\n", title)
	fmt.Println("完成")
}
