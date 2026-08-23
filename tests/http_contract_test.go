package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-template/internal/apperror"
	"go-template/internal/httpserver"
	"go-template/internal/httpx"

	"github.com/gofiber/fiber/v3"
)

type validationRequest struct {
	Name  string `json:"name" validate:"required,min=2,max=32"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=0,lte=120"`
}

// TestRequestValidationFailure 验证字段错误使用统一业务码和请求字段名
func TestRequestValidationFailure(t *testing.T) {
	server := newContractServer()
	registerValidationRoute(server.App())

	response := performJSONRequest(t, server.App(), "/validation", `{"name":"","email":"invalid","age":121}`)
	defer response.Body.Close()

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}

	var body responseEnvelope[httpx.ValidationDetails]
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != apperror.CodeValidationFailed {
		t.Fatalf("code = %d, want %d", body.Code, apperror.CodeValidationFailed)
	}
	if body.Message != "请求参数校验失败" {
		t.Errorf("message = %q, want %q", body.Message, "请求参数校验失败")
	}

	wantRules := map[string]string{
		"name":  "required",
		"email": "email",
		"age":   "lte",
	}
	if len(body.Data.Fields) != len(wantRules) {
		t.Fatalf("field count = %d, want %d", len(body.Data.Fields), len(wantRules))
	}
	for _, field := range body.Data.Fields {
		if wantRules[field.Field] != field.Rule {
			t.Errorf("field %q rule = %q, want %q", field.Field, field.Rule, wantRules[field.Field])
		}
		if field.Message == "" {
			t.Errorf("field %q message is empty", field.Field)
		}
	}
}

// TestMalformedJSONUsesInvalidArgumentCode 验证非法 JSON 返回参数格式业务码
func TestMalformedJSONUsesInvalidArgumentCode(t *testing.T) {
	server := newContractServer()
	registerValidationRoute(server.App())

	response := performJSONRequest(t, server.App(), "/validation", `{"name":`)
	defer response.Body.Close()

	assertFailureResponse(t, response, http.StatusBadRequest, apperror.CodeInvalidArgument, "请求参数格式错误")
}

// TestMultipleJSONValuesAreRejected 验证请求体只允许包含一个 JSON 值
func TestMultipleJSONValuesAreRejected(t *testing.T) {
	server := newContractServer()
	registerValidationRoute(server.App())

	response := performJSONRequest(t, server.App(), "/validation", `{"name":"张三","email":"user@example.com","age":20} {}`)
	defer response.Body.Close()

	assertFailureResponse(t, response, http.StatusBadRequest, apperror.CodeInvalidArgument, "请求参数格式错误")
}

// TestUnknownJSONFieldIsRejected 验证请求体中的未知字段会被拒绝
func TestUnknownJSONFieldIsRejected(t *testing.T) {
	server := newContractServer()
	registerValidationRoute(server.App())

	response := performJSONRequest(t, server.App(), "/validation", `{"name":"张三","email":"user@example.com","age":20,"admin":true}`)
	defer response.Body.Close()

	assertFailureResponse(t, response, http.StatusBadRequest, apperror.CodeInvalidArgument, "请求参数格式错误")
}

// TestValidRequestUsesUnifiedCreatedResponse 验证合法请求返回统一成功结构
func TestValidRequestUsesUnifiedCreatedResponse(t *testing.T) {
	server := newContractServer()
	registerValidationRoute(server.App())

	response := performJSONRequest(t, server.App(), "/validation", `{"name":"张三","email":"user@example.com","age":20}`)
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status code = %d, want %d", response.StatusCode, http.StatusCreated)
	}

	var body responseEnvelope[validationRequest]
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != apperror.CodeSuccess {
		t.Errorf("code = %d, want %d", body.Code, apperror.CodeSuccess)
	}
	if body.Message != "成功" {
		t.Errorf("message = %q, want %q", body.Message, "成功")
	}
	if body.Data.Name != "张三" {
		t.Errorf("name = %q, want %q", body.Data.Name, "张三")
	}
}

// TestBusinessErrorDoesNotExposeCause 验证业务错误不会向客户端暴露底层原因
func TestBusinessErrorDoesNotExposeCause(t *testing.T) {
	server := newContractServer()
	server.App().Get("/conflict", func(fiber.Ctx) error {
		return apperror.Wrap(40901, "用户名已存在", errors.New("duplicate key value violates unique constraint"))
	})

	request := httptest.NewRequest(http.MethodGet, "/conflict", nil)
	response, err := server.App().Test(request)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if strings.Contains(string(body), "duplicate key") {
		t.Fatalf("response exposes internal cause: %s", body)
	}

	var envelope responseEnvelope[any]
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.StatusCode != http.StatusConflict {
		t.Errorf("status code = %d, want %d", response.StatusCode, http.StatusConflict)
	}
	if envelope.Code != 40901 {
		t.Errorf("code = %d, want %d", envelope.Code, 40901)
	}
	if envelope.Message != "用户名已存在" {
		t.Errorf("message = %q, want %q", envelope.Message, "用户名已存在")
	}
}

// TestBusinessCodeDerivesHTTPStatus 验证业务码前缀可以稳定映射 HTTP 状态
func TestBusinessCodeDerivesHTTPStatus(t *testing.T) {
	tests := []struct {
		code       apperror.Code
		wantStatus int
	}{
		{code: apperror.CodeSuccess, wantStatus: http.StatusOK},
		{code: apperror.CodeValidationFailed, wantStatus: http.StatusBadRequest},
		{code: 40901, wantStatus: http.StatusConflict},
		{code: 50399, wantStatus: http.StatusServiceUnavailable},
		{code: 123, wantStatus: http.StatusInternalServerError},
	}

	for _, test := range tests {
		if status := test.code.HTTPStatus(); status != test.wantStatus {
			t.Errorf("code %d status = %d, want %d", test.code, status, test.wantStatus)
		}
	}
}

func newContractServer() *httpserver.Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httpserver.New(httpserver.Config{AppName: "test"}, logger)
}

func registerValidationRoute(app *fiber.App) {
	app.Post("/validation", func(c fiber.Ctx) error {
		var request validationRequest
		// 请求绑定会自动执行结构体校验
		if err := c.Bind().Body(&request); err != nil {
			return err
		}
		return httpx.Created(c, request)
	})
}

func performJSONRequest(t *testing.T, app *fiber.App, path string, body string) *http.Response {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	return response
}

func assertFailureResponse(t *testing.T, response *http.Response, status int, code apperror.Code, message string) {
	t.Helper()
	if response.StatusCode != status {
		t.Fatalf("status code = %d, want %d", response.StatusCode, status)
	}

	var body responseEnvelope[any]
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != code {
		t.Errorf("code = %d, want %d", body.Code, code)
	}
	if body.Message != message {
		t.Errorf("message = %q, want %q", body.Message, message)
	}
	if body.Data != nil {
		t.Errorf("data = %#v, want nil", body.Data)
	}
}
