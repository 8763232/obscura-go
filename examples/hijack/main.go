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

	// === 启动 Go 本地代理 ===
	proxy := &obscura.ProxyServer{
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // 不自动跟随，可见每步 302
			},
			Timeout: 30 * time.Second,
		},
		OnRequest: func(req *http.Request) {
			fmt.Printf("  [Proxy Request] %s %s\n", req.Method, req.URL)
		},
		OnResponse: func(req *http.Request, resp *http.Response) {
			status := resp.StatusCode
			fmt.Printf("  [Proxy Response] %d ← %s %s", status, req.Method, req.URL)
			if status == 301 || status == 302 {
				fmt.Printf(" → Location: %s", resp.Header.Get("Location"))
			}
			fmt.Println()
			for _, sc := range resp.Header["Set-Cookie"] {
				fmt.Printf("    Set-Cookie: %s\n", sc)
			}
		},
	}

	proxyAddr, err := proxy.Start()
	if err != nil {
		log.Fatalf("启动代理失败: %v", err)
	}
	defer proxy.Stop()
	fmt.Printf("代理启动: %s\n", proxyAddr)

	// === 启动 obscura，配置使用 Go 代理 ===
	browser := obscura.New()
	if err := browser.Serve(ctx, obscura.WithProxy(proxyAddr), obscura.WithStealth()); err != nil {
		log.Fatalf("启动 obscura 失败: %v", err)
	}
	defer browser.Close()

	// 忽略证书错误（MITM 自签证书）
	browser.IgnoreCertErrors(true)

	page, _ := browser.NewPage(ctx)
	fmt.Println("\n导航到目标页面...")
	page.Navigate(ctx, "https://login.teamviewer.com/Cmd/ActivateAccount?lng=zhcn&token=f865bfb8-99c9-4dbe-9c30-5b1b109a9bd4")

	var title string
	page.Evaluate(ctx, "document.title", &title)
	fmt.Printf("\n标题: %s\n", title)
}
