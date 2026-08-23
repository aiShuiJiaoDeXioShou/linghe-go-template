package auth

import (
	"context"
	"time"
)

// RealmName 表示相互隔离的登录域名称
type RealmName string

const (
	// RealmApp 表示业务端登录域
	RealmApp RealmName = "app"
	// RealmAdmin 表示管理端登录域
	RealmAdmin RealmName = "admin"
)

// Principal 表示服务端认证得到的可信登录主体
type Principal struct {
	UserID          string
	Realm           RealmName
	Device          string
	AuthenticatedAt time.Time
}

// Token 表示登录成功后返回给客户端的不透明会话令牌
type Token struct {
	Value     string    `json:"access_token"`
	Type      string    `json:"token_type"`
	ExpiresAt time.Time `json:"expires_at"`
	Realm     RealmName `json:"realm"`
}

type principalContextKey struct{}

type requestSessionContextKey struct{}

type requestSession struct {
	token     string
	principal Principal
}

// PrincipalFromContext 读取认证中间件写入的可信登录主体
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

// RequirePrincipal 读取可信登录主体并在会话缺失时返回认证错误
func RequirePrincipal(ctx context.Context) (Principal, error) {
	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		return Principal{}, authenticationRequired()
	}
	return principal, nil
}

func withPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func withRequestSession(ctx context.Context, session requestSession) context.Context {
	return context.WithValue(ctx, requestSessionContextKey{}, session)
}

func requestSessionFromContext(ctx context.Context) (requestSession, bool) {
	session, ok := ctx.Value(requestSessionContextKey{}).(requestSession)
	return session, ok
}
