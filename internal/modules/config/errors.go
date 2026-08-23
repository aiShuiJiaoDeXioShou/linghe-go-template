package config

import "go-template/internal/apperror"

const (
	// CodeInvalidInput 表示系统配置不符合领域约束
	CodeInvalidInput apperror.Code = 40031
	// CodeNotFound 表示系统配置不存在或不可公开访问
	CodeNotFound apperror.Code = 40431
)

// NewInvalidInputError 创建系统配置输入错误
func NewInvalidInputError() error {
	return apperror.New(CodeInvalidInput, "系统配置不合法")
}

// NewNotFoundError 创建系统配置不存在错误
func NewNotFoundError() error {
	return apperror.New(CodeNotFound, "系统配置不存在")
}
