package adminuser

import (
	"errors"

	"go-template/internal/apperror"
)

const (
	// CodeInvalidInput 表示管理员用户输入不符合领域约束
	CodeInvalidInput apperror.Code = 40021
	// CodeInvalidCredentials 表示管理员用户名或密码错误
	CodeInvalidCredentials apperror.Code = 40121
	// CodeDisabled 表示管理员用户已被禁用
	CodeDisabled apperror.Code = 40321
	// CodeNotFound 表示管理员用户不存在
	CodeNotFound apperror.Code = 40421
	// CodeUsernameExists 表示管理员用户名已存在
	CodeUsernameExists apperror.Code = 40921
)

// NewInvalidInputError 创建管理员用户输入错误
func NewInvalidInputError() error {
	return apperror.New(CodeInvalidInput, "管理员用户信息不合法")
}

// NewInvalidCredentialsError 创建管理员登录凭据错误
func NewInvalidCredentialsError() error {
	return apperror.New(CodeInvalidCredentials, "管理员用户名或密码错误")
}

// NewDisabledError 创建管理员用户禁用错误
func NewDisabledError() error {
	return apperror.New(CodeDisabled, "管理员用户已被禁用")
}

// NewNotFoundError 创建管理员用户不存在错误
func NewNotFoundError() error {
	return apperror.New(CodeNotFound, "管理员用户不存在")
}

// NewUsernameExistsError 创建管理员用户名冲突错误
func NewUsernameExistsError() error {
	return apperror.New(CodeUsernameExists, "管理员用户名已存在")
}

// IsNotFound 判断错误链是否表示管理员用户不存在
func IsNotFound(err error) bool {
	var applicationError *apperror.Error
	return errors.As(err, &applicationError) && applicationError.Code() == CodeNotFound
}
