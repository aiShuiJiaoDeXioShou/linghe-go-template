package tests

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-template/internal/apperror"
	"go-template/internal/auth"
	"go-template/internal/httpserver"
	"go-template/internal/httpx"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

const (
	testAppSessionPrefix   = "go-template:test:auth:app:"
	testAdminSessionPrefix = "go-template:test:auth:admin:"
)

// TestAuthRealmsIsolateTokens 验证 App 和 Admin 令牌不能跨登录域使用
func TestAuthRealmsIsolateTokens(t *testing.T) {
	realms, _ := newTestAuthRealms(t)
	ctx := context.Background()

	appToken, err := realms.App.Login(ctx, "user-1", "ios")
	if err != nil {
		t.Fatalf("App.Login() error = %v", err)
	}
	appContext, principal, err := realms.App.Authenticate(ctx, appToken.Value)
	if err != nil {
		t.Fatalf("App.Authenticate() error = %v", err)
	}
	if principal.UserID != "user-1" || principal.Realm != auth.RealmApp || principal.Device != "ios" {
		t.Errorf("principal = %#v, want App user-1 on ios", principal)
	}
	if contextPrincipal, ok := auth.PrincipalFromContext(appContext); !ok || contextPrincipal != principal {
		t.Errorf("PrincipalFromContext() = %#v, %t, want %#v, true", contextPrincipal, ok, principal)
	}

	_, _, err = realms.Admin.Authenticate(ctx, appToken.Value)
	assertApplicationErrorCode(t, err, auth.CodeAuthenticationRequired)

	adminToken, err := realms.Admin.Login(ctx, "user-1", "web")
	if err != nil {
		t.Fatalf("Admin.Login() error = %v", err)
	}
	_, _, err = realms.App.Authenticate(ctx, adminToken.Value)
	assertApplicationErrorCode(t, err, auth.CodeAuthenticationRequired)
}

// TestAuthLoginRotatesCurrentToken 验证已认证上下文重新登录会轮换令牌
func TestAuthLoginRotatesCurrentToken(t *testing.T) {
	realms, _ := newTestAuthRealms(t)
	oldToken := loginTestUser(t, realms.App, "user-rotate", "web")
	ctx, _, err := realms.App.Authenticate(context.Background(), oldToken)
	if err != nil {
		t.Fatalf("App.Authenticate() error = %v", err)
	}

	newToken, err := realms.App.Login(ctx, "user-rotate", "web")
	if err != nil {
		t.Fatalf("App.Login() error = %v", err)
	}
	if newToken.Value == oldToken {
		t.Fatal("App.Login() reused the current token")
	}
	assertTokenRejected(t, realms.App, oldToken)
	assertTokenAccepted(t, realms.App, newToken.Value)
}

// TestAuthRealmRevokesDeviceAndUserSessions 验证按设备和按用户注销会话
func TestAuthRealmRevokesDeviceAndUserSessions(t *testing.T) {
	realms, _ := newTestAuthRealms(t)
	ctx := context.Background()

	mobileTokenOne := loginTestUser(t, realms.App, "user-2", "mobile")
	mobileTokenTwo := loginTestUser(t, realms.App, "user-2", "mobile")
	webToken := loginTestUser(t, realms.App, "user-2", "web")
	adminToken := loginTestUser(t, realms.Admin, "user-2", "web")

	if err := realms.App.LogoutDevice(ctx, "user-2", "mobile"); err != nil {
		t.Fatalf("App.LogoutDevice() error = %v", err)
	}
	assertTokenRejected(t, realms.App, mobileTokenOne)
	assertTokenRejected(t, realms.App, mobileTokenTwo)
	assertTokenAccepted(t, realms.App, webToken)
	assertTokenAccepted(t, realms.Admin, adminToken)

	if err := realms.LogoutUser(ctx, "user-2"); err != nil {
		t.Fatalf("Realms.LogoutUser() error = %v", err)
	}
	assertTokenRejected(t, realms.App, webToken)
	assertTokenRejected(t, realms.Admin, adminToken)
}

// TestAuthStoresOnlyTokenDigest 验证 Redis 键和索引不保存原始令牌
func TestAuthStoresOnlyTokenDigest(t *testing.T) {
	realms, redisServer := newTestAuthRealms(t)
	token := loginTestUser(t, realms.App, "user-3", "android")

	hash := sha256.Sum256([]byte(token))
	tokenHash := base64.RawURLEncoding.EncodeToString(hash[:])
	wantSessionKey := testAppSessionPrefix + "session:" + tokenHash
	if !redisServer.Exists(wantSessionKey) {
		t.Errorf("Redis session key %q does not exist", wantSessionKey)
	}
	storedSession, err := redisServer.Get(wantSessionKey)
	if err != nil {
		t.Fatalf("read Redis session: %v", err)
	}
	if strings.Contains(storedSession, token) {
		t.Error("Redis session contains the raw token")
	}

	for _, key := range redisServer.Keys() {
		if strings.Contains(key, token) {
			t.Errorf("Redis key %q contains the raw token", key)
		}
	}
	values, err := redisServer.SMembers(testAppSessionPrefix + "user:dXNlci0z:sessions")
	if err != nil {
		t.Fatalf("read user session index: %v", err)
	}
	if len(values) != 1 || values[0] != tokenHash {
		t.Errorf("session index = %#v, want token digest %q", values, tokenHash)
	}
}

// TestAuthMiddlewareAndLogoutEndpoint 验证 Bearer 认证和双端注销接口
func TestAuthMiddlewareAndLogoutEndpoint(t *testing.T) {
	realms, _ := newTestAuthRealms(t)
	server := newAuthHTTPServer(realms)
	appToken := loginTestUser(t, realms.App, "user-4", "ios")
	adminToken := loginTestUser(t, realms.Admin, "admin-1", "web")

	response := performAuthRequest(t, server, http.MethodGet, "/api/private?token="+appToken, "")
	assertAuthResponseCode(t, response, http.StatusUnauthorized, auth.CodeAuthenticationRequired)

	response = performAuthRequest(t, server, http.MethodGet, "/api/private", "Basic "+appToken)
	assertAuthResponseCode(t, response, http.StatusUnauthorized, auth.CodeAuthenticationRequired)

	response = performAuthRequest(t, server, http.MethodGet, "/api/private", "Bearer "+adminToken)
	assertAuthResponseCode(t, response, http.StatusUnauthorized, auth.CodeAuthenticationRequired)

	response = performAuthRequest(t, server, http.MethodGet, "/api/private", "Bearer "+appToken)
	assertAuthResponseCode(t, response, http.StatusOK, apperror.CodeSuccess)

	response = performAuthRequest(t, server, http.MethodPost, "/api/auth/logout", "Bearer "+appToken)
	assertAuthResponseCode(t, response, http.StatusOK, apperror.CodeSuccess)

	response = performAuthRequest(t, server, http.MethodGet, "/api/private", "Bearer "+appToken)
	assertAuthResponseCode(t, response, http.StatusUnauthorized, auth.CodeAuthenticationRequired)

	response = performAuthRequest(t, server, http.MethodPost, "/admin/auth/logout", "Bearer "+adminToken)
	assertAuthResponseCode(t, response, http.StatusOK, apperror.CodeSuccess)
}

// TestAuthIdleTimeoutRenewsOnActivity 验证活动请求会刷新空闲过期时间
func TestAuthIdleTimeoutRenewsOnActivity(t *testing.T) {
	realms, redisServer := newTestAuthRealms(t)
	token := loginTestUser(t, realms.App, "user-5", "web")

	redisServer.FastForward(20 * time.Minute)
	assertTokenAccepted(t, realms.App, token)
	redisServer.FastForward(20 * time.Minute)
	assertTokenAccepted(t, realms.App, token)
	redisServer.FastForward(31 * time.Minute)
	assertTokenRejected(t, realms.App, token)
}

// TestAuthRequiresRecentAuthentication 验证敏感操作使用独立业务码要求重新认证
func TestAuthRequiresRecentAuthentication(t *testing.T) {
	realms, _ := newTestAuthRealms(t)
	token := loginTestUser(t, realms.App, "user-6", "web")
	ctx, _, err := realms.App.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("App.Authenticate() error = %v", err)
	}

	if err := realms.App.RequireRecentAuthentication(ctx, 5*time.Minute); err != nil {
		t.Errorf("RequireRecentAuthentication() error = %v, want nil", err)
	}
	err = realms.App.RequireRecentAuthentication(ctx, time.Nanosecond)
	assertApplicationErrorCode(t, err, auth.CodeRecentAuthenticationRequired)
}

// TestAuthorizerChecksDatabasePermissions 验证权限门面使用可信主体查询权限
func TestAuthorizerChecksDatabasePermissions(t *testing.T) {
	realms, _ := newTestAuthRealms(t)
	token := loginTestUser(t, realms.Admin, "admin-2", "web")
	ctx, _, err := realms.Admin.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("Admin.Authenticate() error = %v", err)
	}

	checker := &fakePermissionChecker{permissions: map[string]bool{"system:user:read": true}}
	authorizer, err := auth.NewAuthorizer(realms.Admin, checker)
	if err != nil {
		t.Fatalf("NewAuthorizer() error = %v", err)
	}
	if err := authorizer.CheckPermission(ctx, "system:user:read"); err != nil {
		t.Errorf("CheckPermission() error = %v", err)
	}
	if checker.lastUserID != "admin-2" {
		t.Errorf("permission user ID = %q, want %q", checker.lastUserID, "admin-2")
	}
	if err := authorizer.CheckPermission(ctx, "system:user:write"); err == nil {
		t.Fatal("CheckPermission() error = nil, want permission denied")
	} else {
		assertApplicationErrorCode(t, err, auth.CodePermissionDenied)
	}
	assertApplicationErrorCode(t, authorizer.CheckPermission(ctx, "  "), auth.CodePermissionDenied)

	checker.checkError = errors.New("permission database unavailable")
	assertApplicationErrorCode(
		t,
		authorizer.CheckPermission(ctx, "system:user:read"),
		auth.CodeAuthorizationUnavailable,
	)
}

type fakePermissionChecker struct {
	permissions map[string]bool
	lastUserID  string
	checkError  error
}

// HasPermission 返回测试预设的权限结果
func (c *fakePermissionChecker) HasPermission(_ context.Context, userID string, permission string) (bool, error) {
	c.lastUserID = userID
	return c.permissions[permission], c.checkError
}

// newTestAuthRealms 使用内存 Redis 创建双登录域
func newTestAuthRealms(t *testing.T) (*auth.Realms, *miniredis.Miniredis) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	realms, err := auth.NewRealms(client, auth.Config{
		SessionLifetime: 24 * time.Hour,
		IdleTimeout:     30 * time.Minute,
		App:             auth.RealmConfig{KeyPrefix: testAppSessionPrefix},
		Admin:           auth.RealmConfig{KeyPrefix: testAdminSessionPrefix},
	})
	if err != nil {
		t.Fatalf("NewRealms() error = %v", err)
	}
	return realms, redisServer
}

// newAuthHTTPServer 创建注册认证路由的测试服务
func newAuthHTTPServer(realms *auth.Realms) *httpserver.Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httpserver.New(httpserver.Config{AppName: "auth-test"}, logger)
	auth.RegisterHandlers(server.App(), realms)
	server.App().Get("/api/private", realms.App.AuthenticateMiddleware(), func(c fiber.Ctx) error {
		principal, err := auth.RequirePrincipal(c.Context())
		if err != nil {
			return err
		}
		return httpx.OK(c, map[string]string{
			"user_id": principal.UserID,
			"realm":   string(principal.Realm),
		})
	})
	return server
}

// loginTestUser 创建测试会话并返回原始令牌
func loginTestUser(t *testing.T, realm *auth.Realm, userID string, device string) string {
	t.Helper()
	token, err := realm.Login(context.Background(), userID, device)
	if err != nil {
		t.Fatalf("Realm.Login() error = %v", err)
	}
	return token.Value
}

// assertTokenAccepted 断言令牌可在指定登录域使用
func assertTokenAccepted(t *testing.T, realm *auth.Realm, token string) {
	t.Helper()
	if _, _, err := realm.Authenticate(context.Background(), token); err != nil {
		t.Errorf("Realm.Authenticate() error = %v, want nil", err)
	}
}

// assertTokenRejected 断言令牌被指定登录域拒绝
func assertTokenRejected(t *testing.T, realm *auth.Realm, token string) {
	t.Helper()
	_, _, err := realm.Authenticate(context.Background(), token)
	assertApplicationErrorCode(t, err, auth.CodeAuthenticationRequired)
}

// assertApplicationErrorCode 断言应用错误包含指定业务码
func assertApplicationErrorCode(t *testing.T, err error, want apperror.Code) {
	t.Helper()
	var applicationError *apperror.Error
	if !errors.As(err, &applicationError) {
		t.Fatalf("error = %v, want *apperror.Error", err)
	}
	if applicationError.Code() != want {
		t.Errorf("error code = %d, want %d", applicationError.Code(), want)
	}
}

// performAuthRequest 调用测试服务并返回认证响应
func performAuthRequest(
	t *testing.T,
	server *httpserver.Server,
	method string,
	path string,
	authorization string,
) *http.Response {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	if authorization != "" {
		request.Header.Set(fiber.HeaderAuthorization, authorization)
	}
	response, err := server.App().Test(request)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	return response
}

// assertAuthResponseCode 断言认证接口的 HTTP 状态和业务码
func assertAuthResponseCode(
	t *testing.T,
	response *http.Response,
	wantStatus int,
	wantCode apperror.Code,
) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("status code = %d, want %d", response.StatusCode, wantStatus)
	}
	var body responseEnvelope[json.RawMessage]
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != wantCode {
		t.Errorf("response code = %d, want %d", body.Code, wantCode)
	}
}
