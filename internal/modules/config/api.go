package config

import (
	"encoding/json"
	"time"

	"go-template/internal/auth"
	"go-template/internal/httpx"

	"github.com/gofiber/fiber/v3"
)

// RegisterHandlers 注册系统配置 App 和 Admin HTTP 路由
func RegisterHandlers(router fiber.Router, service *Service, adminRealm *auth.Realm) {
	resource := resource{service: service}

	router.Get("/api/configs/:key", resource.getPublic)
	router.Get("/admin/configs/:key", adminRealm.AuthenticateMiddleware(), resource.get)
	router.Put("/admin/configs/:key", adminRealm.AuthenticateMiddleware(), resource.upsert)
}

type resource struct {
	service *Service
}

type keyRequest struct {
	Key string `uri:"key" validate:"required,max=128"`
}

type upsertRequest struct {
	Value       json.RawMessage `json:"value" swaggertype:"object" validate:"required"`
	Description string          `json:"description" validate:"max=256"`
	Public      bool            `json:"public"`
}

type itemResponse struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value" swaggertype:"object"`
	Description string          `json:"description"`
	Public      bool            `json:"public"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// getPublic 获取公开系统配置
//
// @Summary 获取公开系统配置
// @Tags 系统配置
// @ID getPublicConfig
// @Param key path string true "配置键" minlength(1) maxlength(128)
// @Success 200 {object} httpx.Response{data=itemResponse}
// @Router /api/configs/{key} [get]
func (r resource) getPublic(c fiber.Ctx) error {
	key, err := bindKey(c)
	if err != nil {
		return err
	}
	item, err := r.service.GetPublic(c.Context(), key)
	if err != nil {
		return err
	}
	return httpx.OK(c, newItemResponse(item))
}

// get 获取管理端系统配置
//
// @Summary 获取管理端系统配置
// @Tags 系统配置
// @ID getAdminConfig
// @Security BearerAuth
// @Param key path string true "配置键" minlength(1) maxlength(128)
// @Success 200 {object} httpx.Response{data=itemResponse}
// @Router /admin/configs/{key} [get]
func (r resource) get(c fiber.Ctx) error {
	key, err := bindKey(c)
	if err != nil {
		return err
	}
	item, err := r.service.Get(c.Context(), key)
	if err != nil {
		return err
	}
	return httpx.OK(c, newItemResponse(item))
}

// upsert 新增或更新系统配置
//
// @Summary 新增或更新系统配置
// @Tags 系统配置
// @ID upsertAdminConfig
// @Security BearerAuth
// @Param key path string true "配置键" minlength(1) maxlength(128)
// @Param request body upsertRequest true "系统配置内容"
// @Success 200 {object} httpx.Response{data=itemResponse}
// @Router /admin/configs/{key} [put]
func (r resource) upsert(c fiber.Ctx) error {
	key, err := bindKey(c)
	if err != nil {
		return err
	}
	var request upsertRequest
	// 绑定并校验系统配置内容
	if err := c.Bind().Body(&request); err != nil {
		return err
	}
	item, err := r.service.Upsert(c.Context(), UpsertCommand{
		Key:         key,
		Value:       request.Value,
		Description: request.Description,
		Public:      request.Public,
	})
	if err != nil {
		return err
	}
	return httpx.OK(c, newItemResponse(item))
}

func bindKey(c fiber.Ctx) (string, error) {
	var request keyRequest
	// 绑定并校验路径中的配置键
	if err := c.Bind().URI(&request); err != nil {
		return "", err
	}
	return request.Key, nil
}

func newItemResponse(item Item) itemResponse {
	return itemResponse{
		Key:         item.Key,
		Value:       item.Value,
		Description: item.Description,
		Public:      item.Public,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}
