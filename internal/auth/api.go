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

// logoutApp 注销当前 App 登录域会话
//
// @Summary 注销业务用户会话
// @Tags 认证会话
// @ID logoutBusinessUser
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /api/auth/logout [post]
func (r resource) logoutApp(c fiber.Ctx) error {
	// 注销当前 App 登录域会话
	if err := r.realms.App.LogoutCurrent(c.Context()); err != nil {
		return err
	}
	return httpx.OK(c, nil)
}

// logoutAdmin 注销当前 Admin 登录域会话
//
// @Summary 注销管理员会话
// @Tags 认证会话
// @ID logoutAdminUser
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /admin/auth/logout [post]
func (r resource) logoutAdmin(c fiber.Ctx) error {
	// 注销当前 Admin 登录域会话
	if err := r.realms.Admin.LogoutCurrent(c.Context()); err != nil {
		return err
	}
	return httpx.OK(c, nil)
}
