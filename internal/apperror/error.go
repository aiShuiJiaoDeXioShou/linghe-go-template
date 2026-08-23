package apperror

import "fmt"

// Code 表示稳定且可供客户端判断的业务码
type Code int

const (
	// CodeSuccess 表示请求成功
	CodeSuccess Code = 0
	// CodeInvalidArgument 表示请求参数无法解析
	CodeInvalidArgument Code = 40000
	// CodeValidationFailed 表示请求参数未通过业务规则校验
	CodeValidationFailed Code = 40001
	// CodeUnauthorized 表示请求缺少有效身份
	CodeUnauthorized Code = 40100
	// CodeForbidden 表示当前身份无权访问
	CodeForbidden Code = 40300
	// CodeNotFound 表示请求资源不存在
	CodeNotFound Code = 40400
	// CodeMethodNotAllowed 表示请求方法不受支持
	CodeMethodNotAllowed Code = 40500
	// CodeConflict 表示资源状态冲突
	CodeConflict Code = 40900
	// CodePayloadTooLarge 表示请求体超过限制
	CodePayloadTooLarge Code = 41300
	// CodeUnsupportedMediaType 表示请求内容类型不受支持
	CodeUnsupportedMediaType Code = 41500
	// CodeTooManyRequests 表示请求频率超过限制
	CodeTooManyRequests Code = 42900
	// CodeInternal 表示服务器内部错误
	CodeInternal Code = 50000
	// CodeServiceUnavailable 表示依赖或服务暂不可用
	CodeServiceUnavailable Code = 50300
)

// HTTPStatus 根据业务码前缀返回对应 HTTP 状态码
func (c Code) HTTPStatus() int {
	if c == CodeSuccess {
		return 200
	}

	status := int(c) / 100
	if status < 400 || status > 599 {
		return 500
	}
	return status
}

// Error 表示可以安全转换为 HTTP 响应的应用错误
type Error struct {
	code    Code
	message string
	details any
	cause   error
}

// New 创建不包含底层原因的应用错误
func New(code Code, message string) *Error {
	return &Error{code: code, message: message}
}

// Wrap 创建保留底层原因的应用错误
func Wrap(code Code, message string, cause error) *Error {
	return &Error{code: code, message: message, cause: cause}
}

// WithDetails 设置允许返回客户端的结构化错误详情
func (e *Error) WithDetails(details any) *Error {
	e.details = details
	return e
}

// Error 返回用于日志和错误链的完整错误文本
func (e *Error) Error() string {
	if e.cause == nil {
		return e.message
	}
	return fmt.Sprintf("%s: %v", e.message, e.cause)
}

// Unwrap 返回底层错误
func (e *Error) Unwrap() error {
	return e.cause
}

// Code 返回应用错误的业务码
func (e *Error) Code() Code {
	return e.code
}

// Message 返回可以安全展示给客户端的错误消息
func (e *Error) Message() string {
	return e.message
}

// Details 返回可以安全展示给客户端的错误详情
func (e *Error) Details() any {
	return e.details
}
