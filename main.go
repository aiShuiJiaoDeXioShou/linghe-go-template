package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go-template/internal/app"
	"go-template/internal/config"
)

func main() {
	// 运行应用并统一处理启动或退出错误
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

// run 创建进程上下文并启动应用
func run() error {
	// 解析配置文件路径
	configPath := flag.String("config", config.DefaultPath, "配置文件路径")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return app.Run(ctx, *configPath)
}
