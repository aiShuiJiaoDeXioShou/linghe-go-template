package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-template/internal/apperror"
	"go-template/internal/auth"
	"go-template/internal/httpserver"
	adminusermodule "go-template/internal/modules/adminuser"
	configmodule "go-template/internal/modules/config"
	"go-template/internal/modules/identity"
	usermodule "go-template/internal/modules/user"

	"github.com/gofiber/fiber/v3"
)

// TestBusinessHTTPRoutes 验证用户 管理员和系统配置接口调用链
func TestBusinessHTTPRoutes(t *testing.T) {
	realms, _ := newTestAuthRealms(t)
	server := newBusinessHTTPServer(t, realms)

	response := performBusinessRequest(
		t,
		server,
		http.MethodPost,
		"/api/auth/register",
		`{"username":"alice","password":"password-123","nickname":"Alice"}`,
		"",
	)
	assertBusinessResponseCode(t, response, http.StatusCreated, apperror.CodeSuccess)

	appToken := loginBusinessUser(t, server, "/api/auth/login", `{
		"username":"alice",
		"password":"password-123",
		"device":"ios"
	}`)
	response = performBusinessRequest(t, server, http.MethodGet, "/api/users/me", "", appToken)
	assertBusinessResponseCode(t, response, http.StatusOK, apperror.CodeSuccess)

	adminToken := loginBusinessUser(t, server, "/admin/auth/login", `{
		"username":"root",
		"password":"password-456",
		"device":"web"
	}`)
	response = performBusinessRequest(t, server, http.MethodGet, "/admin/users/me", "", adminToken)
	assertBusinessResponseCode(t, response, http.StatusOK, apperror.CodeSuccess)

	response = performBusinessRequest(
		t,
		server,
		http.MethodPut,
		"/admin/configs/app.banner",
		`{"value":{"enabled":true},"description":"首页横幅","public":true}`,
		adminToken,
	)
	assertPublicConfigResponse(t, response)

	response = performBusinessRequest(t, server, http.MethodGet, "/api/configs/app.banner", "", "")
	assertBusinessResponseCode(t, response, http.StatusOK, apperror.CodeSuccess)
}

func assertPublicConfigResponse(t *testing.T, response *http.Response) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("config status code = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var envelope responseEnvelope[struct {
		Public bool `json:"public"`
	}]
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode config response: %v", err)
	}
	if !envelope.Data.Public {
		t.Fatal("saved config is not public")
	}
}

func newBusinessHTTPServer(t *testing.T, realms *auth.Realms) *httpserver.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httpserver.New(httpserver.Config{AppName: "business-test"}, logger)
	passwords := fakePasswordHasher{}

	userService := usermodule.NewService(
		newFakeUserRepository(),
		passwords,
		auth.NewSessionIssuer(realms.App),
	)
	usermodule.RegisterHandlers(server.App(), userService, realms.App)

	adminRepository := newFakeAdminUserRepository()
	adminService := adminusermodule.NewService(
		adminRepository,
		passwords,
		auth.NewSessionIssuer(realms.Admin),
	)
	if _, err := adminService.Create(context.Background(), adminusermodule.CreateCommand{
		Username: "root",
		Password: "password-456",
	}); err != nil {
		t.Fatalf("create bootstrap admin user: %v", err)
	}
	adminusermodule.RegisterHandlers(server.App(), adminService, realms.Admin)

	configService := configmodule.NewService(&fakeConfigRepository{items: make(map[string]configmodule.Item)})
	configmodule.RegisterHandlers(server.App(), configService, realms.Admin)
	auth.RegisterHandlers(server.App(), realms)
	return server
}

func loginBusinessUser(t *testing.T, server *httpserver.Server, path string, body string) string {
	t.Helper()
	response := performBusinessRequest(t, server, http.MethodPost, path, body, "")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status code = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var envelope responseEnvelope[identity.Token]
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if envelope.Code != apperror.CodeSuccess || envelope.Data.AccessToken == "" {
		t.Fatalf("login response = %#v, want a successful token", envelope)
	}
	return envelope.Data.AccessToken
}

func performBusinessRequest(
	t *testing.T,
	server *httpserver.Server,
	method string,
	path string,
	body string,
	token string,
) *http.Response {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	if token != "" {
		request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	}
	response, err := server.App().Test(request)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	return response
}

func assertBusinessResponseCode(
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
	var envelope responseEnvelope[json.RawMessage]
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != wantCode {
		t.Errorf("response code = %d, want %d", envelope.Code, wantCode)
	}
}
