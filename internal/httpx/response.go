package httpx

import (
	"errors"
	"net/http"

	"go-template/internal/apperror"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

const (
	successMessage       = "成功"
	businessCodeLocalKey = "business_code"
)

// Response 表示所有 JSON 接口使用的统一响应结构
type Response struct {
	Code    apperror.Code `json:"code"`
	Message string        `json:"message"`
	Data    any           `json:"data"`
}

// Failure 表示由全局错误处理器写入的失败响应
type Failure struct {
	Status  int
	Code    apperror.Code
	Message string
	Data    any
}

// OK 返回 HTTP 200 成功响应
func OK(c fiber.Ctx, data any) error {
	return write(c, fiber.StatusOK, apperror.CodeSuccess, successMessage, data)
}

// Created 返回 HTTP 201 成功响应
func Created(c fiber.Ctx, data any) error {
	return write(c, fiber.StatusCreated, apperror.CodeSuccess, successMessage, data)
}

// ResolveError 将应用错误和 Fiber 错误转换为统一失败信息
func ResolveError(err error) Failure {
	var applicationError *apperror.Error
	if errors.As(err, &applicationError) {
		return Failure{
			Status:  applicationError.Code().HTTPStatus(),
			Code:    applicationError.Code(),
			Message: applicationError.Message(),
			Data:    applicationError.Details(),
		}
	}

	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		return Failure{
			Status:  fiber.StatusBadRequest,
			Code:    apperror.CodeValidationFailed,
			Message: "请求参数校验失败",
			Data:    newValidationDetails(validationErrors),
		}
	}

	var bindError *fiber.BindError
	if errors.As(err, &bindError) {
		return Failure{
			Status:  fiber.StatusBadRequest,
			Code:    apperror.CodeInvalidArgument,
			Message: "请求参数格式错误",
		}
	}

	var fiberError *fiber.Error
	if errors.As(err, &fiberError) {
		return Failure{
			Status:  fiberError.Code,
			Code:    codeFromHTTPStatus(fiberError.Code),
			Message: messageFromHTTPStatus(fiberError.Code),
		}
	}

	return Failure{
		Status:  fiber.StatusInternalServerError,
		Code:    apperror.CodeInternal,
		Message: "服务器内部错误",
	}
}

// WriteFailure 写入统一失败响应
func WriteFailure(c fiber.Ctx, failure Failure) error {
	return write(c, failure.Status, failure.Code, failure.Message, failure.Data)
}

func write(c fiber.Ctx, status int, code apperror.Code, message string, data any) error {
	// 写入请求上下文供访问日志记录业务码
	c.Locals(businessCodeLocalKey, int(code))
	return c.Status(status).JSON(Response{
		Code:    code,
		Message: message,
		Data:    data,
	})
}

func codeFromHTTPStatus(status int) apperror.Code {
	switch status {
	case fiber.StatusBadRequest:
		return apperror.CodeInvalidArgument
	case fiber.StatusUnauthorized:
		return apperror.CodeUnauthorized
	case fiber.StatusForbidden:
		return apperror.CodeForbidden
	case fiber.StatusNotFound:
		return apperror.CodeNotFound
	case fiber.StatusMethodNotAllowed:
		return apperror.CodeMethodNotAllowed
	case fiber.StatusConflict:
		return apperror.CodeConflict
	case fiber.StatusRequestEntityTooLarge:
		return apperror.CodePayloadTooLarge
	case fiber.StatusUnsupportedMediaType:
		return apperror.CodeUnsupportedMediaType
	case fiber.StatusTooManyRequests:
		return apperror.CodeTooManyRequests
	case fiber.StatusServiceUnavailable:
		return apperror.CodeServiceUnavailable
	case fiber.StatusInternalServerError:
		return apperror.CodeInternal
	default:
		if status >= fiber.StatusBadRequest && status <= 599 {
			return apperror.Code(status * 100)
		}
		return apperror.CodeInternal
	}
}

func messageFromHTTPStatus(status int) string {
	switch status {
	case fiber.StatusBadRequest:
		return "请求参数错误"
	case fiber.StatusUnauthorized:
		return "未登录或登录状态已失效"
	case fiber.StatusForbidden:
		return "无权访问"
	case fiber.StatusNotFound:
		return "资源不存在"
	case fiber.StatusMethodNotAllowed:
		return "请求方法不受支持"
	case fiber.StatusRequestTimeout:
		return "请求超时"
	case fiber.StatusConflict:
		return "资源状态冲突"
	case fiber.StatusRequestEntityTooLarge:
		return "请求体超过限制"
	case fiber.StatusUnsupportedMediaType:
		return "请求内容类型不受支持"
	case fiber.StatusTooManyRequests:
		return "请求过于频繁"
	case fiber.StatusBadGateway:
		return "上游服务响应异常"
	case fiber.StatusServiceUnavailable:
		return "服务暂不可用"
	case fiber.StatusGatewayTimeout:
		return "上游服务响应超时"
	}

	if status >= fiber.StatusInternalServerError {
		return "服务器内部错误"
	}
	if message := http.StatusText(status); message != "" {
		return message
	}
	return "请求失败"
}
