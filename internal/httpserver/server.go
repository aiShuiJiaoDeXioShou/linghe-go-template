package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go-template/internal/httpx"

	"github.com/gofiber/fiber/v3"
	fiberlogger "github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// Config 表示应用层传入的 HTTP 服务配置
type Config struct {
	AppName         string
	Address         string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// Server 管理 Fiber 应用及网络生命周期
type Server struct {
	app    *fiber.App
	config Config
}

// New 创建完成基础配置的 Fiber HTTP 服务
func New(cfg Config, logger *slog.Logger) *Server {
	// 创建 Fiber 实例并注入统一错误处理
	web := fiber.New(fiber.Config{
		AppName:         cfg.AppName,
		Immutable:       true,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		IdleTimeout:     cfg.IdleTimeout,
		JSONDecoder:     httpx.DecodeJSON,
		StructValidator: httpx.NewStructValidator(),
		ErrorHandler:    newErrorHandler(logger),
	})

	// 按请求标识、访问日志和异常恢复的顺序注册中间件
	web.Use(requestid.New())
	web.Use(fiberlogger.New(fiberlogger.Config{
		Format:     "${time} request_id=${requestid} status=${status} business_code=${locals:business_code} latency=${latency} method=${method} path=${path}\n",
		TimeFormat: time.RFC3339,
		TimeZone:   "UTC",
	}))
	web.Use(recover.New())

	return &Server{app: web, config: cfg}
}

// App 返回底层 Fiber 应用
func (s *Server) App() *fiber.App {
	return s.app
}

// Run 监听请求并在上下文取消后优雅停止
func (s *Server) Run(ctx context.Context) error {
	err := s.app.Listen(s.config.Address, fiber.ListenConfig{
		DisableStartupMessage: true,
		GracefulContext:       ctx,
		ShutdownTimeout:       s.config.ShutdownTimeout,
	})
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.config.Address, err)
	}
	return nil
}

// newErrorHandler 创建统一 JSON 错误处理函数
func newErrorHandler(logger *slog.Logger) func(fiber.Ctx, error) error {
	return func(c fiber.Ctx, err error) error {
		// 将错误链转换为稳定业务码和安全响应内容
		failure := httpx.ResolveError(err)

		if failure.Status >= fiber.StatusInternalServerError {
			logger.Error("HTTP request failed",
				"request_id", requestid.FromContext(c),
				"method", c.Method(),
				"path", c.Path(),
				"error", err,
			)
		}

		return httpx.WriteFailure(c, failure)
	}
}
