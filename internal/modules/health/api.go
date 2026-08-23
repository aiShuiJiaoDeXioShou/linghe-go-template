package health

import (
	"context"
	"time"

	"go-template/internal/apperror"
	"go-template/internal/httpx"

	"github.com/gofiber/fiber/v3"
)

const (
	// CodeDependencyUnavailable 表示就绪检查依赖不可用
	CodeDependencyUnavailable apperror.Code = 50301
	// CodeDatabaseUnavailable 表示业务数据库检查不可用
	CodeDatabaseUnavailable apperror.Code = 50302
)

type check func(context.Context) error

type handler struct {
	readinessCheck check
	databaseCheck  check
	readinessLimit time.Duration
}

type healthResponse struct {
	Status string `json:"status"`
}

type pingResponse struct {
	Message  string `json:"message"`
	Database string `json:"database"`
}

// RegisterHandlers 注册存活 就绪和数据库探针路由
func RegisterHandlers(
	router fiber.Router,
	readinessCheck func(context.Context) error,
	databaseCheck func(context.Context) error,
	readinessLimit time.Duration,
) {
	h := handler{
		readinessCheck: readinessCheck,
		databaseCheck:  databaseCheck,
		readinessLimit: readinessLimit,
	}

	router.Get("/healthz", h.health)
	router.Get("/readyz", h.ready)
	router.Get("/api/v1/ping", h.ping)
}

// health 检查 HTTP 进程存活状态
//
// @Summary 检查进程存活状态
// @Tags 系统探针
// @ID getLiveness
// @Success 200 {object} httpx.Response{data=healthResponse}
// @Router /healthz [get]
func (h handler) health(c fiber.Ctx) error {
	return httpx.OK(c, healthResponse{Status: "ok"})
}

// ready 检查应用依赖就绪状态
//
// @Summary 检查应用就绪状态
// @Tags 系统探针
// @ID getReadiness
// @Success 200 {object} httpx.Response{data=healthResponse}
// @Router /readyz [get]
func (h handler) ready(c fiber.Ctx) error {
	// 限制依赖检查时间避免探针长时间阻塞
	checkContext, cancel := context.WithTimeout(c.Context(), h.readinessLimit)
	defer cancel()
	if err := h.readinessCheck(checkContext); err != nil {
		response := healthResponse{Status: "unavailable"}
		return apperror.Wrap(CodeDependencyUnavailable, "依赖服务不可用", err).WithDetails(response)
	}
	return httpx.OK(c, healthResponse{Status: "ok"})
}

// ping 检查业务数据库连接
//
// @Summary 检查业务数据库连接
// @Tags 系统探针
// @ID pingDatabase
// @Success 200 {object} httpx.Response{data=pingResponse}
// @Router /api/v1/ping [get]
func (h handler) ping(c fiber.Ctx) error {
	// 执行独立的 PostgreSQL 轻量查询
	if err := h.databaseCheck(c.Context()); err != nil {
		return apperror.Wrap(CodeDatabaseUnavailable, "数据库服务不可用", err)
	}
	return httpx.OK(c, pingResponse{Message: "pong", Database: "ok"})
}
