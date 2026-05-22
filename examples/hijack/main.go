package main

import (
	"context"
	"fmt"
	"log"
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

	// 设置网络拦截
	router := page.HijackRequests()

	// 拦截特定 API 返回 mock 数据
	router.Add("*", "XHR", func(ctx context.Context, req *obscura.HijackRequest, res *obscura.HijackResponse) {
		fmt.Printf("[Mock] 拦截 API 请求: %s %s\n", req.Method, req.URL)
		//res.Fulfill(200,
		//	map[string]string{"Content-Type": "application/json"},
		//	`{"message": "mock data from obscura-go"}`)
	})

	// 控制重定向
	router.Add("*", "", func(ctx context.Context, req *obscura.HijackRequest, res *obscura.HijackResponse) {
		// 响应阶段：检查重定向
		if req.StatusCode == 301 || req.StatusCode == 302 {
			location := req.ResponseHeaders["Location"]
			fmt.Printf("[Redirect] %d → %s\n", req.StatusCode, location)

			if location != "" && len(location) > 100 {
				fmt.Println("[Redirect] 阻止可疑的长 URL 重定向")
				res.Fail("BlockedByClient")
				return
			}
			res.Follow()
			return
		}

		// 请求阶段：打印所有请求
		if req.StatusCode == 0 {
			fmt.Printf("[Request] %s %s (%s)\n", req.Method, req.URL, req.Type)
		}
	})

	router.Run()
	defer router.Stop()

	// 忽略 HTTPS 证书错误（某些网站证书可能有问题）
	if err := browser.IgnoreCertErrors(true); err != nil {
		log.Fatalf("设置忽略证书错误失败: %v", err)
	}

	fmt.Println("导航到目标页面...")
	if err := page.Navigate(ctx, "https://login.teamviewer.com/Cmd/ActivateAccount?lng=zhcn&token=f865bfb8-99c9-4dbe-9c30-5b1b109a9bd4"); err != nil {
		log.Fatalf("导航失败: %v", err)
	}

	time.Sleep(2 * time.Second)
	fmt.Println("完成")
}
