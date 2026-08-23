package user

import (
	"errors"

	"go-template/internal/apperror"
)

const (
	// CodeInvalidInput 表示业务用户输入不符合领域约束
	CodeInvalidInput apperror.Code = 40011
	// CodeInvalidCredentials 表示用户名或密码错误
	CodeInvalidCredentials apperror.Code = 40111
	// CodeDisabled 表示业务用户已被禁用
	CodeDisabled apperror.Code = 40311
	// CodeNotFound 表示业务用户不存在
	CodeNotFound apperror.Code = 40411
	// CodeUsernameExists 表示业务用户名已存在
	CodeUsernameExists apperror.Code = 40911
)

// NewInvalidInputError 创建业务用户输入错误
func NewInvalidInputError() error {
	return apperror.New(CodeInvalidInput, "用户信息不合法")
}

// NewInvalidCredentialsError 创建登录凭据错误
func NewInvalidCredentialsError() error {
	return apperror.New(CodeInvalidCredentials, "用户名或密码错误")
}

// NewDisabledError 创建业务用户禁用错误
func NewDisabledError() error {
	return apperror.New(CodeDisabled, "用户已被禁用")
}

// NewNotFoundError 创建业务用户不存在错误
func NewNotFoundError() error {
	return apperror.New(CodeNotFound, "用户不存在")
}

// NewUsernameExistsError 创建业务用户名冲突错误
func NewUsernameExistsError() error {
	return apperror.New(CodeUsernameExists, "用户名已存在")
}

// IsNotFound 判断错误链是否表示业务用户不存在
func IsNotFound(err error) bool {
	var applicationError *apperror.Error
	return errors.As(err, &applicationError) && applicationError.Code() == CodeNotFound
}
