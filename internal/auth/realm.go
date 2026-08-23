package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-template/internal/apperror"

	"github.com/alexedwards/scs/goredisstore"
	"github.com/alexedwards/scs/v2"
	"github.com/redis/go-redis/v9"
)

const (
	sessionUserIDKey          = "user_id"
	sessionRealmKey           = "realm"
	sessionDeviceKey          = "device"
	sessionAuthenticatedAtKey = "authenticated_at"
	tokenTypeBearer           = "Bearer"
)

// Config 表示 App 和 Admin 双登录域的会话配置
type Config struct {
	SessionLifetime time.Duration
	IdleTimeout     time.Duration
	App             RealmConfig
	Admin           RealmConfig
}

// RealmConfig 表示单个登录域的 Redis 键空间配置
type RealmConfig struct {
	KeyPrefix string
}

// Realms 聚合相互隔离的 App 和 Admin 认证门面
type Realms struct {
	App   *Realm
	Admin *Realm
}

// Realm 管理单个登录域的会话生命周期
type Realm struct {
	name     RealmName
	sessions *scs.SessionManager
	index    *redisSessionIndex
}

// NewRealms 使用共享 Redis 客户端创建相互隔离的双登录域
func NewRealms(client *redis.Client, cfg Config) (*Realms, error) {
	if client == nil {
		return nil, fmt.Errorf("Redis 客户端不能为空")
	}
	if cfg.SessionLifetime <= 0 {
		return nil, fmt.Errorf("认证会话有效期必须大于零")
	}
	if cfg.IdleTimeout <= 0 || cfg.IdleTimeout > cfg.SessionLifetime {
		return nil, fmt.Errorf("认证空闲时限必须大于零且不能超过会话有效期")
	}

	appPrefix, err := normalizeRealmPrefix(cfg.App.KeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("App 登录域配置: %w", err)
	}
	adminPrefix, err := normalizeRealmPrefix(cfg.Admin.KeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("Admin 登录域配置: %w", err)
	}
	if appPrefix == adminPrefix {
		return nil, fmt.Errorf("App 和 Admin 登录域必须使用不同的 Redis 键前缀")
	}

	return &Realms{
		App:   newRealm(client, RealmApp, appPrefix, cfg.SessionLifetime, cfg.IdleTimeout),
		Admin: newRealm(client, RealmAdmin, adminPrefix, cfg.SessionLifetime, cfg.IdleTimeout),
	}, nil
}

// Name 返回当前登录域名称
func (r *Realm) Name() RealmName {
	return r.name
}

// Login 创建指定用户和设备的不透明 Redis 会话
func (r *Realm) Login(ctx context.Context, userID string, device string) (Token, error) {
	userID = strings.TrimSpace(userID)
	device = strings.TrimSpace(device)
	if userID == "" {
		return Token{}, fmt.Errorf("登录用户 ID 不能为空")
	}
	if device == "" {
		return Token{}, fmt.Errorf("登录设备不能为空")
	}

	// 加载空会话并写入服务端可信身份
	sessionContext, err := r.sessions.Load(ctx, "")
	if err != nil {
		return Token{}, sessionUnavailable(err)
	}
	oldToken := r.sessions.Token(sessionContext)
	oldUserID := r.sessions.GetString(sessionContext, sessionUserIDKey)
	oldDevice := r.sessions.GetString(sessionContext, sessionDeviceKey)
	if oldToken != "" {
		// 已认证上下文再次登录时轮换令牌并重置绝对有效期
		if err := r.sessions.RenewToken(sessionContext); err != nil {
			return Token{}, sessionUnavailable(err)
		}
	}
	authenticatedAt := time.Now().UTC()
	r.sessions.Put(sessionContext, sessionUserIDKey, userID)
	r.sessions.Put(sessionContext, sessionRealmKey, string(r.name))
	r.sessions.Put(sessionContext, sessionDeviceKey, device)
	r.sessions.Put(sessionContext, sessionAuthenticatedAtKey, authenticatedAt.Unix())

	// 提交会话后建立用户和设备反向索引
	tokenValue, expiresAt, err := r.sessions.Commit(sessionContext)
	if err != nil {
		return Token{}, sessionUnavailable(err)
	}
	if err := r.index.add(sessionContext, userID, device, tokenDigest(tokenValue)); err != nil {
		_ = r.sessions.Destroy(sessionContext)
		return Token{}, sessionUnavailable(err)
	}
	if oldToken != "" && oldUserID != "" && oldDevice != "" {
		_ = r.index.remove(sessionContext, oldUserID, oldDevice, tokenDigest(oldToken))
	}
	return Token{
		Value:     tokenValue,
		Type:      tokenTypeBearer,
		ExpiresAt: expiresAt,
		Realm:     r.name,
	}, nil
}

// Authenticate 校验令牌并返回携带可信登录主体的上下文
func (r *Realm) Authenticate(ctx context.Context, token string) (context.Context, Principal, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return ctx, Principal{}, authenticationRequired()
	}
	if session, ok := requestSessionFromContext(ctx); ok && session.principal.Realm == r.name {
		if session.token != token {
			return ctx, Principal{}, authenticationRequired()
		}
		return ctx, session.principal, nil
	}

	// 从当前登录域的 Redis 键空间加载会话
	sessionContext, err := r.sessions.Load(ctx, token)
	if err != nil {
		return ctx, Principal{}, sessionUnavailable(err)
	}
	userID := r.sessions.GetString(sessionContext, sessionUserIDKey)
	device := r.sessions.GetString(sessionContext, sessionDeviceKey)
	realmName := RealmName(r.sessions.GetString(sessionContext, sessionRealmKey))
	authenticatedAtUnix := r.sessions.GetInt64(sessionContext, sessionAuthenticatedAtKey)
	if userID == "" || device == "" || realmName != r.name || authenticatedAtUnix <= 0 {
		return ctx, Principal{}, authenticationRequired()
	}

	if r.sessions.Status(sessionContext) == scs.Modified {
		// 提交 SCS 更新后的空闲过期时间
		if _, _, err := r.sessions.Commit(sessionContext); err != nil {
			return ctx, Principal{}, sessionUnavailable(err)
		}
	}

	principal := Principal{
		UserID:          userID,
		Realm:           r.name,
		Device:          device,
		AuthenticatedAt: time.Unix(authenticatedAtUnix, 0).UTC(),
	}
	sessionContext = withPrincipal(sessionContext, principal)
	sessionContext = withRequestSession(sessionContext, requestSession{
		token:     token,
		principal: principal,
	})
	return sessionContext, principal, nil
}

// LogoutCurrent 使认证中间件加载的当前会话失效
func (r *Realm) LogoutCurrent(ctx context.Context) error {
	session, ok := requestSessionFromContext(ctx)
	if !ok || session.principal.Realm != r.name {
		return authenticationRequired()
	}

	// 删除当前 SCS 会话并同步清理反向索引
	if err := r.sessions.Destroy(ctx); err != nil {
		return sessionUnavailable(err)
	}
	if err := r.index.remove(
		ctx,
		session.principal.UserID,
		session.principal.Device,
		tokenDigest(session.token),
	); err != nil {
		return sessionUnavailable(err)
	}
	return nil
}

// LogoutDevice 使指定用户在当前登录域和设备上的全部会话失效
func (r *Realm) LogoutDevice(ctx context.Context, userID string, device string) error {
	userID = strings.TrimSpace(userID)
	device = strings.TrimSpace(device)
	if userID == "" || device == "" {
		return fmt.Errorf("用户 ID 和设备不能为空")
	}
	if err := r.index.deleteDevice(ctx, userID, device); err != nil {
		return sessionUnavailable(err)
	}
	return nil
}

// LogoutUser 使指定用户在当前登录域中的全部会话失效
func (r *Realm) LogoutUser(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("用户 ID 不能为空")
	}
	if err := r.index.deleteUser(ctx, userID); err != nil {
		return sessionUnavailable(err)
	}
	return nil
}

// RequireRecentAuthentication 校验当前登录主体是否在指定时间内完成认证
func (r *Realm) RequireRecentAuthentication(ctx context.Context, maxAge time.Duration) error {
	if maxAge <= 0 {
		return fmt.Errorf("近期认证时限必须大于零")
	}
	principal, err := RequirePrincipal(ctx)
	if err != nil {
		return err
	}
	if principal.Realm != r.name {
		return authenticationRequired()
	}
	if time.Since(principal.AuthenticatedAt) > maxAge {
		return newRecentAuthenticationRequiredError()
	}
	return nil
}

// LogoutUser 使指定用户的 App 和 Admin 全部会话失效
func (r *Realms) LogoutUser(ctx context.Context, userID string) error {
	appError := r.App.LogoutUser(ctx, userID)
	adminError := r.Admin.LogoutUser(ctx, userID)
	return errors.Join(appError, adminError)
}

// newRealm 创建绑定独立 Redis 键空间的登录域
func newRealm(
	client *redis.Client,
	name RealmName,
	prefix string,
	lifetime time.Duration,
	idleTimeout time.Duration,
) *Realm {
	sessionPrefix := prefix + "session:"
	sessionManager := scs.New()
	sessionManager.Store = goredisstore.NewWithPrefix(client, sessionPrefix)
	sessionManager.Lifetime = lifetime
	sessionManager.IdleTimeout = idleTimeout
	sessionManager.HashTokenInStore = true

	return &Realm{
		name:     name,
		sessions: sessionManager,
		index: newRedisSessionIndex(
			client,
			prefix,
			sessionPrefix,
			2*lifetime,
		),
	}
}

// normalizeRealmPrefix 规范化并校验登录域 Redis 键前缀
func normalizeRealmPrefix(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", fmt.Errorf("Redis 键前缀不能为空")
	}
	if strings.ContainsAny(prefix, " \t\r\n") {
		return "", fmt.Errorf("Redis 键前缀不能包含空白字符")
	}
	if !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return prefix, nil
}

func newRecentAuthenticationRequiredError() error {
	return apperror.New(CodeRecentAuthenticationRequired, "需要重新验证身份").WithDetails(map[string]any{
		"reason": "recent_authentication_required",
	})
}
