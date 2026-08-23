// utils 包提供无业务语义的通用工具函数
package utils

import "github.com/google/uuid"

// NewUUID 生成项目统一使用的 UUID
func NewUUID() string {
	return uuid.NewString()
}
