package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	obscura "github.com/8763232/obscura-go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	browser := obscura.New()
	if err := browser.Serve(ctx); err != nil {
		log.Fatalf("启动 obscura 失败: %v", err)
	}
	defer browser.Close()

	urls := []string{
		"https://example.com",
		"https://httpbin.org",
		"https://news.ycombinator.com",
	}

	var wg sync.WaitGroup
	results := make(chan string, len(urls))

	for _, url := range urls {
		wg.Add(1)
		go func(targetURL string) {
			defer wg.Done()

			incog, err := browser.NewIncognito(ctx)
			if err != nil {
				log.Printf("创建 incognito 失败: %v", err)
				return
			}
			defer incog.Close()

			page, err := incog.NewPage(ctx)
			if err != nil {
				log.Printf("创建页面失败: %v", err)
				return
			}

			if err := page.Navigate(ctx, targetURL); err != nil {
				log.Printf("导航 %s 失败: %v", targetURL, err)
				return
			}

			var title string
			if err := page.Evaluate(ctx, "document.title", &title); err != nil {
				log.Printf("获取 %s 标题失败: %v", targetURL, err)
				return
			}

			results <- fmt.Sprintf("[%s] %s", targetURL, title)
		}(url)
	}

	wg.Wait()
	close(results)

	fmt.Println("并发抓取结果:")
	for r := range results {
		fmt.Println(r)
	}
}
