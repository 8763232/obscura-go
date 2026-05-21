package obscura_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	obscura "github.com/8763232/obscura-go"
)

const testEndpoint = "ws://127.0.0.1:9223/devtools/browser"

var testBrowser *obscura.Browser

func TestMain(m *testing.M) {
	browser := obscura.New()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := browser.Connect(ctx, testEndpoint); err != nil {
		println("连接 Obscura 失败:", err.Error())
		os.Exit(1)
	}

	testBrowser = browser
	code := m.Run()
	browser.Close()
	os.Exit(code)
}

// 测试连接
func TestConnect(t *testing.T) {
	if testBrowser == nil {
		t.Fatal("浏览器未连接")
	}
	t.Log("浏览器连接正常")
}

// 测试页面导航与 JS 执行
func TestNavigateAndEvaluate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	page, err := testBrowser.NewPage(ctx)
	if err != nil {
		t.Fatalf("创建页面失败: %v", err)
	}

	if err := page.Navigate(ctx, "https://example.com"); err != nil {
		t.Fatalf("导航失败: %v", err)
	}

	var title string
	if err := page.Evaluate(ctx, "document.title", &title); err != nil {
		t.Fatalf("执行 JS 失败: %v", err)
	}

	if title == "" {
		t.Fatal("标题不应为空")
	}
	t.Logf("页面标题: %s", title)
	if title != "Example Domain" {
		t.Errorf("期望标题 'Example Domain'，得到 '%s'", title)
	}
}

// 测试 HTML 和 Markdown
func TestContent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	page, err := testBrowser.NewPage(ctx)
	if err != nil {
		t.Fatalf("创建页面失败: %v", err)
	}

	if err := page.Navigate(ctx, "https://example.com"); err != nil {
		t.Fatalf("导航失败: %v", err)
	}

	html, err := page.HTML(ctx)
	if err != nil {
		t.Fatalf("获取 HTML 失败: %v", err)
	}
	if !strings.Contains(html, "<html") {
		t.Fatal("HTML 应包含 <html> 标签")
	}
	t.Logf("HTML 长度: %d", len(html))

	md, err := page.Markdown(ctx)
	if err != nil {
		t.Fatalf("获取 Markdown 失败: %v", err)
	}
	if md == "" {
		t.Fatal("Markdown 不应为空")
	}
	t.Logf("Markdown 长度: %d", len(md))
	t.Logf("Markdown: %s", strings.TrimSpace(md))
}

// 测试网络拦截
func TestHijack(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	page, err := testBrowser.NewPage(ctx)
	if err != nil {
		t.Fatalf("创建页面失败: %v", err)
	}

	router := page.HijackRequests()

	mockCalled := false
	requestCount := 0

	router.Add("*/api/*", "XHR", func(ctx context.Context, req *obscura.HijackRequest, res *obscura.HijackResponse) {
		mockCalled = true
		res.Fulfill(200,
			map[string]string{"Content-Type": "application/json"},
			`{"mocked": true}`)
	})

	router.Add("*", "", func(ctx context.Context, req *obscura.HijackRequest, res *obscura.HijackResponse) {
		requestCount++
		t.Logf("[请求] %s %s (类型=%s)", req.Method, req.URL, req.Type)
	})

	router.Run()
	defer router.Stop()

	if err := page.Navigate(ctx, "https://httpbin.org/get"); err != nil {
		t.Fatalf("导航失败: %v", err)
	}

	time.Sleep(1 * time.Second) // 等待所有请求完成

	t.Logf("请求数量: %d, mock handler 被调用: %v", requestCount, mockCalled)
	if requestCount == 0 {
		t.Error("应该拦截到至少一个请求")
	}
}

// 测试并发多页面（使用主 Browser 创建多个页面）
func TestConcurrency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	urls := []string{
		"https://example.com",
		"https://httpbin.org",
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(urls))
	results := make([]string, len(urls))

	for i, url := range urls {
		wg.Add(1)
		go func(idx int, targetURL string) {
			defer wg.Done()

			page, err := testBrowser.NewPage(ctx)
			if err != nil {
				errCh <- err
				return
			}

			if err := page.Navigate(ctx, targetURL); err != nil {
				errCh <- err
				return
			}

			var title string
			if err := page.Evaluate(ctx, "document.title", &title); err != nil {
				errCh <- err
				return
			}
			results[idx] = title
			t.Logf("[%s] %s", targetURL, title)
		}(i, url)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("并发测试错误: %v", err)
	}

	if results[0] != "Example Domain" {
		t.Errorf("example.com 标题错误: %s", results[0])
	}
	if results[1] != "httpbin.org" {
		t.Errorf("httpbin.org 标题错误: %s", results[1])
	}
}
