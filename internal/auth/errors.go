package auth

import "go-template/internal/apperror"

const (
	// CodeAuthenticationRequired 表示缺少有效登录会话
	CodeAuthenticationRequired apperror.Code = 40101
	// CodeRecentAuthenticationRequired 表示敏感操作需要近期认证
	CodeRecentAuthenticationRequired apperror.Code = 40102
	// CodePermissionDenied 表示当前登录主体缺少指定权限
	CodePermissionDenied apperror.Code = 40301
	// CodeSessionUnavailable 表示认证会话存储暂不可用
	CodeSessionUnavailable apperror.Code = 50310
	// CodeAuthorizationUnavailable 表示权限数据暂不可用
	CodeAuthorizationUnavailable apperror.Code = 50311
)

func authenticationRequired() *apperror.Error {
	return apperror.New(CodeAuthenticationRequired, "未登录或登录状态已失效")
}

func sessionUnavailable(cause error) *apperror.Error {
	return apperror.Wrap(CodeSessionUnavailable, "认证服务暂不可用", cause)
}
