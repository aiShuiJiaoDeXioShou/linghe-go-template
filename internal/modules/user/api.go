package user

import (
	"time"

	"go-template/internal/auth"
	"go-template/internal/httpx"

	"github.com/gofiber/fiber/v3"
)

// RegisterHandlers 注册业务用户 App HTTP 路由
func RegisterHandlers(router fiber.Router, service *Service, realm *auth.Realm) {
	resource := resource{service: service}

	router.Post("/api/auth/register", resource.register)
	router.Post("/api/auth/login", resource.login)
	router.Get("/api/users/me", realm.AuthenticateMiddleware(), resource.me)
}

type resource struct {
	service *Service
}

type registerRequest struct {
	Username string `json:"username" validate:"required,min=3,max=32"`
	Password string `json:"password" validate:"required,min=8,max=72"`
	Nickname string `json:"nickname" validate:"max=64"`
}

type loginRequest struct {
	Username string `json:"username" validate:"required,max=32"`
	Password string `json:"password" validate:"required,max=72"`
	Device   string `json:"device" validate:"required,max=64"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r resource) register(c fiber.Ctx) error {
	var request registerRequest
	// 绑定并校验业务用户注册参数
	if err := c.Bind().Body(&request); err != nil {
		return err
	}
	createdUser, err := r.service.Register(c.Context(), RegisterCommand{
		Username: request.Username,
		Password: request.Password,
		Nickname: request.Nickname,
	})
	if err != nil {
		return err
	}
	return httpx.Created(c, newUserResponse(createdUser))
}

func (r resource) login(c fiber.Ctx) error {
	var request loginRequest
	// 绑定并校验业务用户登录参数
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

func (r resource) me(c fiber.Ctx) error {
	// 从认证上下文读取服务端可信用户 ID
	principal, err := auth.RequirePrincipal(c.Context())
	if err != nil {
		return err
	}
	currentUser, err := r.service.GetProfile(c.Context(), principal.UserID)
	if err != nil {
		return err
	}
	return httpx.OK(c, newUserResponse(currentUser))
}

func newUserResponse(currentUser User) userResponse {
	return userResponse{
		ID:        currentUser.ID,
		Username:  currentUser.Username,
		Nickname:  currentUser.Nickname,
		Status:    currentUser.Status,
		CreatedAt: currentUser.CreatedAt,
		UpdatedAt: currentUser.UpdatedAt,
	}
}
