package tests

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-template/internal/apperror"
	"go-template/internal/health"
	"go-template/internal/httpserver"
)

// TestHealthRoute 验证健康检查接口
func TestHealthRoute(t *testing.T) {
	// 创建关闭日志输出的测试服务
	server := newTestServer(httpserver.Config{AppName: "test"}, databaseReady, dependenciesReady)

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response, err := server.App().Test(request)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", response.StatusCode, http.StatusOK)
	}

	var body responseEnvelope[healthResponse]
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != apperror.CodeSuccess {
		t.Errorf("code = %d, want %d", body.Code, apperror.CodeSuccess)
	}
	if body.Data.Status != "ok" {
		t.Errorf("status = %q, want %q", body.Data.Status, "ok")
	}
}

// TestPingRouteReflectsDatabaseFailure 验证数据库检查失败时返回服务不可用
func TestPingRouteReflectsDatabaseFailure(t *testing.T) {
	server := newTestServer(
		httpserver.Config{AppName: "test"},
		func(context.Context) error {
			return errors.New("database unavailable")
		},
		dependenciesReady,
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	response, err := server.App().Test(request)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want %d", response.StatusCode, http.StatusServiceUnavailable)
	}

	var body responseEnvelope[any]
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != health.CodeDatabaseUnavailable {
		t.Errorf("code = %d, want %d", body.Code, health.CodeDatabaseUnavailable)
	}
	if body.Message != "数据库服务不可用" {
		t.Errorf("message = %q, want %q", body.Message, "数据库服务不可用")
	}
	if body.Data != nil {
		t.Errorf("data = %#v, want nil", body.Data)
	}
}

// TestReadyRouteReflectsDependencyState 验证就绪接口反映依赖状态
func TestReadyRouteReflectsDependencyState(t *testing.T) {
	tests := []struct {
		name       string
		check      func(context.Context) error
		wantStatus int
		wantCode   apperror.Code
		wantBody   string
	}{
		{
			name: "ready",
			check: func(context.Context) error {
				return nil
			},
			wantStatus: http.StatusOK,
			wantCode:   apperror.CodeSuccess,
			wantBody:   "ok",
		},
		{
			name: "unavailable",
			check: func(context.Context) error {
				return errors.New("dependency unavailable")
			},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   health.CodeDependencyUnavailable,
			wantBody:   "unavailable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// 注入依赖状态并调用就绪接口
			server := newTestServer(
				httpserver.Config{AppName: "test"},
				databaseReady,
				test.check,
			)
			request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			response, err := server.App().Test(request)
			if err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
			defer response.Body.Close()

			if response.StatusCode != test.wantStatus {
				t.Fatalf("status code = %d, want %d", response.StatusCode, test.wantStatus)
			}

			var body responseEnvelope[healthResponse]
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Code != test.wantCode {
				t.Errorf("code = %d, want %d", body.Code, test.wantCode)
			}
			if body.Data.Status != test.wantBody {
				t.Errorf("status = %q, want %q", body.Data.Status, test.wantBody)
			}
		})
	}
}

// TestNotFoundUsesJSONError 验证未找到路由时返回 JSON 错误
func TestNotFoundUsesJSONError(t *testing.T) {
	// 创建关闭日志输出的测试服务
	server := newTestServer(httpserver.Config{AppName: "test"}, databaseReady, dependenciesReady)

	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	response, err := server.App().Test(request)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", response.StatusCode, http.StatusNotFound)
	}

	var body responseEnvelope[any]
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != apperror.CodeNotFound {
		t.Errorf("code = %d, want %d", body.Code, apperror.CodeNotFound)
	}
	if body.Message != "资源不存在" {
		t.Errorf("message = %q, want %q", body.Message, "资源不存在")
	}
}

// TestServerRunStopsWhenContextCanceled 验证上下文取消后服务会优雅停止
func TestServerRunStopsWhenContextCanceled(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate test address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close test listener: %v", err)
	}

	// 使用临时地址创建真实监听服务
	server := newTestServer(
		httpserver.Config{
			AppName:         "test",
			Address:         address,
			ShutdownTimeout: time.Second,
		},
		databaseReady,
		dependenciesReady,
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- server.Run(ctx)
	}()

	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, requestErr := client.Get("http://" + address + "/healthz")
		if requestErr == nil {
			response.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("server did not become ready: %v", requestErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()

	select {
	case runErr := <-done:
		if runErr != nil {
			t.Fatalf("Server.Run() error = %v", runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after context cancellation")
	}
}

type responseEnvelope[T any] struct {
	Code    apperror.Code `json:"code"`
	Message string        `json:"message"`
	Data    T             `json:"data"`
}

type healthResponse struct {
	Status string `json:"status"`
}

func databaseReady(context.Context) error {
	return nil
}

func dependenciesReady(context.Context) error {
	return nil
}

// newTestServer 使用显式模块依赖创建关闭日志输出的测试服务
func newTestServer(
	cfg httpserver.Config,
	databaseCheck func(context.Context) error,
	readinessCheck func(context.Context) error,
) *httpserver.Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httpserver.New(cfg, logger)

	// 注入检查函数并注册轻量系统探针
	health.RegisterHandlers(server.App(), readinessCheck, databaseCheck, time.Second)
	return server
}
