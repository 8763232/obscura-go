package main

import (
	"context"
	"fmt"
	"log"
	"os"
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

	if err := page.Navigate(ctx, "https://ip111.cn"); err != nil {
		log.Fatalf("导航失败: %v", err)
	}
	fmt.Println("页面加载完成")

	// 获取页面标题
	var title string
	if err := page.Evaluate(ctx, "document.title", &title); err != nil {
		log.Fatalf("获取标题失败: %v", err)
	}
	fmt.Printf("标题: %s\n", title)

	// 获取 Markdown 转换
	md, err := page.Markdown(ctx)
	if err != nil {
		log.Fatalf("Markdown 转换失败: %v", err)
	}
	fmt.Printf("Markdown 长度: %d 字符\n", len(md))

	// 截图
	png, err := page.Screenshot(ctx)
	if err != nil {
		log.Fatalf("截图失败: %v", err)
	}
	if err := os.WriteFile("screenshot.png", png, 0644); err != nil {
		log.Fatalf("保存截图失败: %v", err)
	}
	fmt.Println("截图已保存到 screenshot.png")
}
