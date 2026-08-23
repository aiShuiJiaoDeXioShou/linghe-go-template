package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go-template/internal/app"
	"go-template/internal/config"
	"go-template/internal/migration"
)

// main 启动 HTTP 服务或数据库迁移命令
//
// @title go-template API
// @version 1.0
// @description go-template 提供业务端和管理端 HTTP API
// @BasePath /
// @schemes http https
// @accept json
// @produce json
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description 使用 Bearer 加空格和会话令牌
func main() {
	// 运行应用并统一处理启动或退出错误
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

// run 创建进程上下文并启动应用
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 迁移命令与 HTTP 服务共享根进程入口但保持显式执行
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		return runMigration(ctx, os.Args[2:])
	}
	return runServer(ctx, os.Args[1:])
}

func runServer(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	configPath := flags.String("config", config.DefaultPath, "配置文件路径")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("未知参数 %q", flags.Arg(0))
	}
	return app.Run(ctx, *configPath)
}

func runMigration(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("迁移命令必须指定 up down 或 version")
	}
	action := args[0]
	flags := flag.NewFlagSet("migrate "+action, flag.ContinueOnError)
	configPath := flags.String("config", config.DefaultPath, "配置文件路径")
	migrationsPath := flags.String("path", migration.DefaultDirectory, "迁移文件目录")
	steps := flags.Uint("steps", 1, "回滚迁移步数")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("未知参数 %q", flags.Arg(0))
	}

	// 迁移命令只加载 PostgreSQL 配置且不会初始化 Redis 或 HTTP 服务
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("加载迁移配置: %w", err)
	}
	switch action {
	case "up":
		return migration.Up(ctx, cfg.PostgreSQL.URL, *migrationsPath)
	case "down":
		return migration.Down(ctx, cfg.PostgreSQL.URL, *migrationsPath, *steps)
	case "version":
		state, err := migration.Current(cfg.PostgreSQL.URL, *migrationsPath)
		if err != nil {
			return err
		}
		if !state.Exists {
			_, err = fmt.Fprintln(os.Stdout, "version=none dirty=false")
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "version=%d dirty=%t\n", state.Version, state.Dirty)
		return err
	default:
		return fmt.Errorf("未知迁移命令 %q 只支持 up down 或 version", action)
	}
}
