package adminuser

import (
	"time"

	"go-template/internal/auth"
	"go-template/internal/httpx"

	"github.com/gofiber/fiber/v3"
)

// RegisterHandlers 注册管理员用户 Admin HTTP 路由
func RegisterHandlers(router fiber.Router, service *Service, realm *auth.Realm) {
	resource := resource{service: service}

	router.Post("/admin/auth/login", resource.login)
	router.Get("/admin/users/me", realm.AuthenticateMiddleware(), resource.me)
	router.Post("/admin/users", realm.AuthenticateMiddleware(), resource.create)
}

type resource struct {
	service *Service
}

type createRequest struct {
	Username    string `json:"username" validate:"required,min=3,max=32"`
	Password    string `json:"password" validate:"required,min=8,max=72"`
	DisplayName string `json:"display_name" validate:"max=64"`
}

type loginRequest struct {
	Username string `json:"username" validate:"required,max=32"`
	Password string `json:"password" validate:"required,max=72"`
	Device   string `json:"device" validate:"required,max=64"`
}

type adminUserResponse struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// create 创建管理员用户
//
// @Summary 创建管理员用户
// @Tags 管理员用户
// @ID createAdminUser
// @Security BearerAuth
// @Param request body createRequest true "管理员用户参数"
// @Success 201 {object} httpx.Response{data=adminUserResponse}
// @Router /admin/users [post]
func (r resource) create(c fiber.Ctx) error {
	var request createRequest
	// 绑定并校验管理员用户创建参数
	if err := c.Bind().Body(&request); err != nil {
		return err
	}
	createdUser, err := r.service.Create(c.Context(), CreateCommand{
		Username:    request.Username,
		Password:    request.Password,
		DisplayName: request.DisplayName,
	})
	if err != nil {
		return err
	}
	return httpx.Created(c, newAdminUserResponse(createdUser))
}

// login 登录管理员用户
//
// @Summary 登录管理员用户
// @Tags 管理员用户
// @ID loginAdminUser
// @Param request body loginRequest true "登录参数"
// @Success 200 {object} httpx.Response{data=auth.Token}
// @Router /admin/auth/login [post]
func (r resource) login(c fiber.Ctx) error {
	var request loginRequest
	// 绑定并校验管理员用户登录参数
	if err := c.Bind().Body(&request); err != nil {
		return err
	}
	token, err := r.service.Login(c.Context(), LoginCommand{
		Username: request.Username,
		Password: request.Password,
		Device:   request.Device,
	})
	if err != nil {
		return err
	}
	return httpx.OK(c, token)
}

// me 获取当前管理员用户
//
// @Summary 获取当前管理员用户
// @Tags 管理员用户
// @ID getCurrentAdminUser
// @Security BearerAuth
// @Success 200 {object} httpx.Response{data=adminUserResponse}
// @Router /admin/users/me [get]
func (r resource) me(c fiber.Ctx) error {
	// 从认证上下文读取服务端可信管理员 ID
	principal, err := auth.RequirePrincipal(c.Context())
	if err != nil {
		return err
	}
	currentUser, err := r.service.GetProfile(c.Context(), principal.UserID)
	if err != nil {
		return err
	}
	return httpx.OK(c, newAdminUserResponse(currentUser))
}

func newAdminUserResponse(currentUser AdminUser) adminUserResponse {
	return adminUserResponse{
		ID:          currentUser.ID,
		Username:    currentUser.Username,
		DisplayName: currentUser.DisplayName,
		Status:      currentUser.Status,
		CreatedAt:   currentUser.CreatedAt,
		UpdatedAt:   currentUser.UpdatedAt,
	}
}
