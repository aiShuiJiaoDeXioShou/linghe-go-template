package adminuser

import "time"

// Status 表示管理员用户状态
type Status string

const (
	// StatusEnabled 表示管理员用户可以正常使用
	StatusEnabled Status = "enabled"
	// StatusDisabled 表示管理员用户已被禁用
	StatusDisabled Status = "disabled"
)

// AdminUser 表示不包含认证凭据的管理员用户
type AdminUser struct {
	ID          string
	Username    string
	DisplayName string
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Credential 表示仅在登录流程内部使用的管理员凭据
type Credential struct {
	AdminUser    AdminUser
	PasswordHash string
}

// CreateAdminUser 表示持久层创建管理员用户所需的数据
type CreateAdminUser struct {
	Username     string
	DisplayName  string
	PasswordHash string
	Status       Status
}

// CreateCommand 表示创建管理员用户的命令
type CreateCommand struct {
	Username    string
	Password    string
	DisplayName string
}

// LoginCommand 表示管理员用户登录命令
type LoginCommand struct {
	Username string
	Password string
	Device   string
}
