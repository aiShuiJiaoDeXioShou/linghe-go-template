package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go-template/internal/auth"
	"go-template/internal/config"
	"go-template/internal/data"
	"go-template/internal/httpserver"
)

// Run 使用指定配置构建应用并阻塞运行直至 HTTP 服务停止
func Run(ctx context.Context, configPath string) error {
	// 加载并校验 YAML 配置
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// 根据运行环境创建日志组件
	logger := newLogger(cfg.App.Env)

	// 创建并验证全部数据资源
	resources, err := data.Open(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize data resources: %w", err)
	}
	defer func() {
		if err := resources.Close(); err != nil {
			logger.Warn("close data resources failed", "error", err)
		}
	}()

	// 手动装配 HTTP 服务和业务模块
	server, err := newHTTPServer(cfg, logger, resources)
	if err != nil {
		return fmt.Errorf("initialize HTTP server: %w", err)
	}

	logger.Info("PostgreSQL and Redis connections ready")
	logger.Info("starting HTTP server",
		"app", cfg.App.Name,
		"environment", cfg.App.Env,
		"address", cfg.HTTP.Address(),
	)

	// 运行服务并等待进程上下文结束
	if err := server.Run(ctx); err != nil {
		return fmt.Errorf("run HTTP server: %w", err)
	}

	logger.Info("HTTP server stopped")
	return nil
}

// newHTTPServer 组装 HTTP 服务及全部业务模块
func newHTTPServer(cfg config.Config, logger *slog.Logger, resources *data.Data) (*httpserver.Server, error) {
	server := httpserver.New(httpserver.Config{
		AppName:         cfg.App.Name,
		Address:         cfg.HTTP.Address(),
		ReadTimeout:     cfg.HTTP.ReadTimeout,
		WriteTimeout:    cfg.HTTP.WriteTimeout,
		IdleTimeout:     cfg.HTTP.IdleTimeout,
		ShutdownTimeout: cfg.HTTP.ShutdownTimeout,
	}, logger)
	if cfg.App.Env != "production" {
		// 在本地和预发布环境开放 API 文档供调试与同步
		server.RegisterAPIDocs()
	}

	// 创建 App 和 Admin 相互隔离的认证门面
	realms, err := auth.NewRealms(resources.Redis(), auth.Config{
		SessionLifetime: cfg.Auth.SessionLifetime,
		IdleTimeout:     cfg.Auth.IdleTimeout,
		App:             auth.RealmConfig{KeyPrefix: cfg.Auth.App.KeyPrefix},
		Admin:           auth.RealmConfig{KeyPrefix: cfg.Auth.Admin.KeyPrefix},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize authentication realms: %w", err)
	}

	// 集中装配认证和全部业务模块
	registerModules(server.App(), resources, realms, cfg.HTTP.ReadinessTimeout)
	return server, nil
}

// newLogger 根据运行环境创建文本或 JSON 日志组件
func newLogger(environment string) *slog.Logger {
	options := &slog.HandlerOptions{Level: slog.LevelInfo}
	if strings.EqualFold(environment, "local") {
		return slog.New(slog.NewTextHandler(os.Stdout, options))
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, options))
}
