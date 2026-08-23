package auth

import (
	"context"
	"fmt"
	"strings"

	"go-template/internal/apperror"

	"github.com/gofiber/fiber/v3"
)

// PermissionChecker 定义权限数据的最小查询能力
type PermissionChecker interface {
	HasPermission(ctx context.Context, userID string, permission string) (bool, error)
}

// Authorizer 将登录域和权限查询器组合为路由授权门面
type Authorizer struct {
	realm   *Realm
	checker PermissionChecker
}

// NewAuthorizer 创建绑定指定登录域的权限门面
func NewAuthorizer(realm *Realm, checker PermissionChecker) (*Authorizer, error) {
	if realm == nil {
		return nil, fmt.Errorf("登录域不能为空")
	}
	if checker == nil {
		return nil, fmt.Errorf("权限查询器不能为空")
	}
	return &Authorizer{realm: realm, checker: checker}, nil
}

// AuthenticateMiddleware 创建只接受 Authorization Bearer 的 Fiber 认证中间件
func (r *Realm) AuthenticateMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		token, err := parseBearerToken(c.Get(fiber.HeaderAuthorization))
		if err != nil {
			return err
		}

		// 校验会话并替换为携带可信主体的请求上下文
		requestContext, _, err := r.Authenticate(c.Context(), token)
		if err != nil {
			return err
		}
		c.SetContext(requestContext)
		return c.Next()
	}
}

// CheckPermission 校验当前登录主体是否拥有指定权限
func (a *Authorizer) CheckPermission(ctx context.Context, permission string) error {
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return apperror.New(CodePermissionDenied, "无权访问")
	}
	principal, err := RequirePrincipal(ctx)
	if err != nil {
		return err
	}
	if principal.Realm != a.realm.Name() {
		return authenticationRequired()
	}

	allowed, err := a.checker.HasPermission(ctx, principal.UserID, permission)
	if err != nil {
		return apperror.Wrap(CodeAuthorizationUnavailable, "权限服务暂不可用", err)
	}
	if !allowed {
		return apperror.New(CodePermissionDenied, "无权访问")
	}
	return nil
}

// RequirePermission 创建校验指定权限码的 Fiber 中间件
func (a *Authorizer) RequirePermission(permission string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if err := a.CheckPermission(c.Context(), permission); err != nil {
			return err
		}
		return c.Next()
	}
}

// parseBearerToken 从请求头解析并复制 Bearer 令牌
func parseBearerToken(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], tokenTypeBearer) || parts[1] == "" {
		return "", authenticationRequired()
	}
	return strings.Clone(parts[1]), nil
}
