package auth

import (
	"go-template/internal/httpx"

	"github.com/gofiber/fiber/v3"
)

// RegisterHandlers 注册 App 和 Admin 两个登录域的会话接口
func RegisterHandlers(router fiber.Router, realms *Realms) {
	resource := resource{realms: realms}

	router.Post("/api/auth/logout", realms.App.AuthenticateMiddleware(), resource.logoutApp)
	router.Post("/admin/auth/logout", realms.Admin.AuthenticateMiddleware(), resource.logoutAdmin)
}

type resource struct {
	realms *Realms
}

func (r resource) logoutApp(c fiber.Ctx) error {
	// 注销当前 App 登录域会话
	if err := r.realms.App.LogoutCurrent(c.Context()); err != nil {
		return err
	}
	return httpx.OK(c, nil)
}

func (r resource) logoutAdmin(c fiber.Ctx) error {
	// 注销当前 Admin 登录域会话
	if err := r.realms.Admin.LogoutCurrent(c.Context()); err != nil {
		return err
	}
	return httpx.OK(c, nil)
}
